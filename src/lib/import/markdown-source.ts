import * as crypto from 'node:crypto';
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { parseFrontmatter } from './frontmatter.js';
import { emptyParsedImport, ROOT_NOTEBOOK_REF, type ParsedImport, type ParsedNote, type ParsedNotebook, type ParsedResource } from './types.js';

export { ROOT_NOTEBOOK_REF };
export const MD_EXTENSIONS = ['.md', '.markdown', '.mdown', '.mkd'];

const MD_LINK_RE = /\[([^\]]*)\]\(([^)]+)\)/g;

const MIME_BY_EXT: Record<string, string> = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.svg': 'image/svg+xml',
  '.webp': 'image/webp',
  '.pdf': 'application/pdf',
  '.txt': 'text/plain',
  '.zip': 'application/zip',
  '.mp3': 'audio/mpeg',
  '.mp4': 'video/mp4',
};

function guessMime(filename: string): string {
  return MIME_BY_EXT[path.extname(filename).toLowerCase()] ?? 'application/octet-stream';
}

/**
 * Rewrites relative links pointing at a real local file that isn't another
 * imported note into a `:/<synthetic-id>` token — the same shape JEX bodies
 * already contain — registering the file in `resources` (deduped by resolved
 * path, so a logo referenced from three notes is only read from disk once).
 * The synthetic ID is an MD5 hash of the resolved path: 32 hex characters,
 * matching Joplin's own ID shape, which is what lets importer.ts's existing
 * `:/id`-token resource-upload pass (built for JEX) handle these too with no
 * changes of its own — a link to another imported .md note is deliberately
 * left as-is here for that same importer.ts pass to rewrite once note IDs
 * exist. A relative link to a file that doesn't actually exist on disk is
 * left untouched — not ours to guess about.
 */
async function rewriteAssetLinks(body: string, fileDir: string, mdFileSet: Set<string>, resources: Map<string, ParsedResource>): Promise<string> {
  let result = '';
  let lastIndex = 0;

  for (const m of body.matchAll(MD_LINK_RE)) {
    const [full, text, target] = m;
    const matchStart = m.index ?? 0;
    if (/^https?:\/\//.test(target) || target.startsWith(':/')) continue;

    const resolved = path.resolve(fileDir, target);
    if (mdFileSet.has(resolved)) continue; // note-to-note link — importer.ts's second pass handles this.

    let resource = resources.get(resolved);
    if (!resource) {
      let data: Buffer;
      try {
        data = await fs.readFile(resolved);
      } catch {
        continue; // doesn't exist / unreadable — leave the link exactly as written.
      }
      resource = { id: crypto.createHash('md5').update(resolved).digest('hex'), filename: path.basename(resolved), mime: guessMime(resolved), data };
      resources.set(resolved, resource);
    }

    result += body.slice(lastIndex, matchStart) + `[${text}](:/${resource.id})`;
    lastIndex = matchStart + full.length;
  }

  return result + body.slice(lastIndex);
}

function truthy(value: string | undefined): boolean {
  if (!value) return false;
  return ['true', 'yes', '1'].includes(value.toLowerCase());
}

function parseDate(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? undefined : ms;
}

const HEADING_RE = /^#{1,6}\s+(.+?)\s*$/;

function extractTitle(body: string, fallback: string): { title: string; body: string } {
  const lines = body.split('\n');
  const firstNonBlank = lines.findIndex((l) => l.trim() !== '');
  if (firstNonBlank === -1) return { title: fallback, body };

  const m = HEADING_RE.exec(lines[firstNonBlank]);
  if (m) {
    // Strip the duplicated title heading from the body — Joplin displays the
    // title separately, so leaving it in the body would show it twice.
    const rest = [...lines.slice(0, firstNonBlank), ...lines.slice(firstNonBlank + 1)].join('\n').replace(/^\n+/, '');
    return { title: m[1], body: rest };
  }

  const firstLine = lines[firstNonBlank].trim();
  if (firstLine.length > 0 && firstLine.length <= 100) {
    return { title: firstLine, body };
  }

  return { title: fallback, body };
}

function titleFromFilename(filePath: string): string {
  return path.basename(filePath, path.extname(filePath)).replace(/[_-]+/g, ' ').trim();
}

async function collectMarkdownFiles(root: string): Promise<string[]> {
  const stat = await fs.stat(root);
  if (stat.isFile()) return [root];

  const found: string[] = [];
  async function walk(dir: string): Promise<void> {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.name.startsWith('.')) continue;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        await walk(full);
      } else if (MD_EXTENSIONS.includes(path.extname(entry.name).toLowerCase())) {
        found.push(full);
      }
    }
  }
  await walk(root);
  return found;
}

/** Notebook ref for a file: its directory path relative to the import root,
 * with path separators normalized to `/` — or ROOT_NOTEBOOK_REF if the file
 * sits directly at the root (or the source is a single file). */
function notebookRefForFile(filePath: string, root: string, isSingleFile: boolean): string {
  if (isSingleFile) return ROOT_NOTEBOOK_REF;
  const rel = path.relative(root, path.dirname(filePath));
  return rel === '' ? ROOT_NOTEBOOK_REF : rel.split(path.sep).join('/');
}

function ensureNotebookChain(notebooks: Map<string, ParsedNotebook>, ref: string): void {
  if (ref === ROOT_NOTEBOOK_REF || notebooks.has(ref)) return;
  const segments = ref.split('/');
  for (let i = 0; i < segments.length; i++) {
    const segRef = segments.slice(0, i + 1).join('/');
    if (notebooks.has(segRef)) continue;
    const parentRef = i === 0 ? undefined : segments.slice(0, i).join('/');
    notebooks.set(segRef, { ref: segRef, title: segments[i], parentRef });
  }
}

/**
 * Parses a single markdown file or a directory of them into a ParsedImport.
 * Directory structure becomes notebook structure (nested); frontmatter
 * `notebook:` overrides that for an individual file. No Joplin API calls —
 * see src/lib/import/importer.ts for the apply phase.
 */
export async function parseMarkdownSource(sourcePath: string): Promise<ParsedImport> {
  const stat = await fs.stat(sourcePath);
  const isSingleFile = stat.isFile();

  if (isSingleFile && !MD_EXTENSIONS.includes(path.extname(sourcePath).toLowerCase())) {
    throw new Error(`not a markdown file (expected one of ${MD_EXTENSIONS.join(', ')}): ${sourcePath}`);
  }

  const files = await collectMarkdownFiles(sourcePath);
  const mdFileSet = new Set(files.map((f) => path.resolve(f)));
  const result: ParsedImport = emptyParsedImport();
  const notebooks = new Map<string, ParsedNotebook>();
  const tagRefs = new Set<string>();
  const resources = new Map<string, ParsedResource>();

  for (const filePath of files) {
    const raw = await fs.readFile(filePath, 'utf-8');
    const { fields, lists, body: rawBody } = parseFrontmatter(raw);
    const fileStat = await fs.stat(filePath);

    const { title, body: bodyWithoutTitle } = extractTitle(rawBody, fields.title || titleFromFilename(filePath) || 'Untitled note');
    const body = await rewriteAssetLinks(bodyWithoutTitle, path.dirname(filePath), mdFileSet, resources);
    const resolvedTitle = fields.title || title;

    const notebookRef = fields.notebook
      ? fields.notebook
          .split('/')
          .map((s) => s.trim())
          .filter(Boolean)
          .join('/') || ROOT_NOTEBOOK_REF
      : notebookRefForFile(filePath, sourcePath, isSingleFile);
    ensureNotebookChain(notebooks, notebookRef);

    const tags = lists.tags ?? [];
    for (const t of tags) tagRefs.add(t);

    const note: ParsedNote = {
      title: resolvedTitle,
      body,
      notebookRef,
      tagRefs: tags,
      isTodo: truthy(fields.todo) || truthy(fields.is_todo),
      todoCompleted: truthy(fields.completed) || truthy(fields.todo_completed),
      createdTime: parseDate(fields.created) ?? parseDate(fields.date) ?? fileStat.birthtimeMs,
      updatedTime: parseDate(fields.updated) ?? parseDate(fields.modified) ?? fileStat.mtimeMs,
      sourceFilePath: path.resolve(filePath),
    };
    result.notes.push(note);
  }

  result.notebooks = [...notebooks.values()];
  result.tags = [...tagRefs].map((title) => ({ ref: title, title }));
  result.resources = [...resources.values()];
  return result;
}

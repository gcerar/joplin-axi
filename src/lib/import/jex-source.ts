import { list as tarList } from 'tar';
import type { ReadEntry } from 'tar';
import { emptyParsedImport, ROOT_NOTEBOOK_REF, type ParsedImport, type ParsedNote, type ParsedNotebook, type ParsedResource, type ParsedTag } from './types.js';

// Joplin's RAW export item-type codes (used both standalone and packed into
// a .jex, which is a plain tar archive despite the zip-like extension).
const TYPE_NOTE = '1';
const TYPE_FOLDER = '2';
const TYPE_RESOURCE = '4';
const TYPE_TAG = '5';
const TYPE_NOTE_TAG = '6';

// The fixed field set Joplin's exporter writes in a note/folder/tag/relation/
// resource item's trailing metadata block. Restricting the backward key:value
// scan to this allowlist (rather than accepting any "word: text" line) avoids
// misparsing body prose that happens to look like "Label: something".
const KNOWN_KV_KEYS = new Set([
  'id', 'parent_id', 'created_time', 'updated_time', 'is_conflict', 'latitude', 'longitude', 'altitude',
  'author', 'source_url', 'is_todo', 'todo_due', 'todo_completed', 'source', 'source_application',
  'application_data', 'order', 'user_created_time', 'user_updated_time', 'encryption_cipher_text',
  'encryption_applied', 'markup_language', 'is_shared', 'share_id', 'conflict_original_id', 'master_key_id',
  'note_id', 'tag_id', 'mime', 'filename', 'size', 'file_extension', 'encryption_blob_encrypted', 'icon',
  'type_',
]);

interface KvBlock {
  metadata: Record<string, string>;
  contentBeforeBlock: string;
}

/**
 * Splits a Joplin RAW-export item's raw text into its leading content and
 * trailing `key: value` metadata block. Scans backward from the last line,
 * collecting recognized-key lines, and stops at the first line that either
 * doesn't match `key: value` shape or whose key isn't in KNOWN_KV_KEYS —
 * matching what a real export always has (every field present, including
 * empty ones) so a genuine content/metadata boundary is never ambiguous.
 */
function splitKvBlock(raw: string): KvBlock {
  const lines = raw.split('\n');
  let boundary = lines.length;

  for (let i = lines.length - 1; i >= 0; i--) {
    const m = /^([a-z_]+):(.*)$/.exec(lines[i]);
    if (!m || !KNOWN_KV_KEYS.has(m[1])) break;
    boundary = i;
  }

  const metadata: Record<string, string> = {};
  for (let i = boundary; i < lines.length; i++) {
    const m = /^([a-z_]+):(.*)$/.exec(lines[i]);
    if (m) metadata[m[1]] = m[2].trim();
  }

  // The exporter puts one blank line between content and the metadata block.
  let contentEnd = boundary;
  if (contentEnd > 0 && lines[contentEnd - 1] === '') contentEnd--;

  return { metadata, contentBeforeBlock: lines.slice(0, contentEnd).join('\n') };
}

/** Joplin RAW items store title as a bare first line (no `#` heading marker,
 * unlike the markdown importer's convention) — content is title, blank line,
 * body. Folders/tags have title only (no body). */
function splitTitleAndBody(content: string): { title: string; body: string } {
  const firstBreak = content.indexOf('\n');
  if (firstBreak === -1) return { title: content, body: '' };
  const title = content.slice(0, firstBreak);
  const rest = content.slice(firstBreak + 1).replace(/^\n/, '');
  return { title, body: rest };
}

function toEpochMs(iso: string | undefined): number | undefined {
  if (!iso) return undefined;
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? undefined : ms;
}

/** Reads an entire tar entry into memory via Minipass's concat() — never
 * writes anything to disk, which is the deliberate fix for the reference
 * implementation's bug (it extracts to a temp dir that gets cleaned up
 * before resource upload runs later in the pipeline). */
async function readEntry(entry: ReadEntry): Promise<Buffer> {
  return entry.concat();
}

/**
 * Parses a .jex archive (a tar file containing a Joplin RAW export) entirely
 * in memory into a ParsedImport, reconstructing notebook hierarchy, tags, and
 * note-tag associations from the archive's own item types — the reference
 * implementation this was ported from does none of that for JEX (only a
 * literal inline `tags:` field, which real exports never set).
 */
export async function parseJexSource(jexPath: string): Promise<ParsedImport> {
  const mdEntries: { name: string; data: Buffer }[] = [];
  const resourceEntries: { name: string; data: Buffer }[] = [];
  const pending: Promise<void>[] = [];

  try {
    await tarList({
      file: jexPath,
      onReadEntry: (entry: ReadEntry) => {
        if (entry.type === 'Directory') return;
        // tar normalizes a `.`-cwd add with a `./` prefix on every entry path.
        const entryPath = entry.path.replace(/^\.\//, '');
        pending.push(
          readEntry(entry).then((data) => {
            if (entryPath.startsWith('resources/')) {
              resourceEntries.push({ name: entryPath.slice('resources/'.length), data });
            } else if (entryPath.endsWith('.md')) {
              mdEntries.push({ name: entryPath, data });
            }
          })
        );
      },
    });
    await Promise.all(pending);
  } catch (e) {
    throw new Error(`not a valid JEX archive (expected a tar archive): ${jexPath} — ${(e as Error).message}`);
  }

  if (mdEntries.length === 0 && resourceEntries.length === 0) {
    // Note: tar's parser doesn't throw on non-tar/garbage input — it just
    // yields zero entries — so this also covers "not actually a tar archive",
    // not only a genuinely empty one.
    throw new Error(`empty or invalid JEX archive (no note/resource entries found, or not a valid tar archive): ${jexPath}`);
  }

  const notebooks: ParsedNotebook[] = [];
  const tags: ParsedTag[] = [];
  const noteTagRelations: { noteId: string; tagId: string }[] = [];
  const resourceMeta: { id: string; filename?: string; mime?: string }[] = [];
  const notes: ParsedNote[] = [];

  for (const { data } of mdEntries) {
    const raw = data.toString('utf-8');
    const { metadata, contentBeforeBlock } = splitKvBlock(raw);
    const type = metadata.type_;
    if (!type) continue; // not a recognizable Joplin export item — skip rather than guess.

    const { title, body } = splitTitleAndBody(contentBeforeBlock);

    switch (type) {
      case TYPE_FOLDER:
        notebooks.push({
          ref: metadata.id,
          title,
          parentRef: metadata.parent_id ? metadata.parent_id : undefined,
        });
        break;
      case TYPE_TAG:
        tags.push({ ref: metadata.id, title });
        break;
      case TYPE_NOTE_TAG:
        if (metadata.note_id && metadata.tag_id) noteTagRelations.push({ noteId: metadata.note_id, tagId: metadata.tag_id });
        break;
      case TYPE_RESOURCE:
        resourceMeta.push({ id: metadata.id, filename: metadata.filename || title, mime: metadata.mime });
        break;
      case TYPE_NOTE:
        notes.push({
          title,
          body,
          notebookRef: metadata.parent_id ? metadata.parent_id : ROOT_NOTEBOOK_REF,
          tagRefs: [], // filled in below once all note_tag relations are known
          isTodo: metadata.is_todo === '1',
          todoCompleted: metadata.todo_completed === '1',
          createdTime: toEpochMs(metadata.created_time) ?? toEpochMs(metadata.user_created_time),
          updatedTime: toEpochMs(metadata.updated_time) ?? toEpochMs(metadata.user_updated_time),
          sourceId: metadata.id,
        });
        break;
      default:
        break; // other item types (master keys, etc.) aren't notes/notebooks/tags — nothing to import.
    }
  }

  const tagsByNoteId = new Map<string, string[]>();
  for (const rel of noteTagRelations) {
    const list = tagsByNoteId.get(rel.noteId) ?? [];
    list.push(rel.tagId);
    tagsByNoteId.set(rel.noteId, list);
  }
  for (const note of notes) {
    note.tagRefs = note.sourceId ? tagsByNoteId.get(note.sourceId) ?? [] : [];
  }

  const resourceByStem = new Map<string, { name: string; data: Buffer }>();
  for (const r of resourceEntries) {
    const stem = r.name.replace(/\.[^.]+$/, '');
    resourceByStem.set(stem, r);
  }

  const resources: ParsedResource[] = [];
  for (const meta of resourceMeta) {
    const blob = resourceByStem.get(meta.id);
    if (!blob) continue; // metadata item with no matching binary — nothing to upload, left unresolved downstream.
    resources.push({ id: meta.id, filename: meta.filename || blob.name, mime: meta.mime, data: blob.data });
  }

  const result: ParsedImport = emptyParsedImport();
  result.notebooks = notebooks;
  result.tags = tags;
  result.notes = notes;
  result.resources = resources;
  return result;
}

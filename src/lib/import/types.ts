// Shared shapes for the two-phase import pipeline: a source-specific parse
// phase (markdown-source.ts / jex-source.ts) produces a ParsedImport with zero
// Joplin API calls, then importer.ts applies it. Keeping parsing pure and
// synchronous-with-the-filesystem-only is what makes `--dry-run` (parse, then
// stop) and unit testing the parsers (no mocked client needed) both cheap.

/** Sentinel notebookRef meaning "the --notebook target itself" rather than a
 * notebook that needs creating: for markdown import, notes/directories at the
 * source root; for JEX import, items with no parent_id (Joplin's own concept
 * of a top-level item). Shared by both sources so importer.ts has one rule
 * regardless of which produced the ParsedImport. */
export const ROOT_NOTEBOOK_REF = '.';

export interface ParsedNotebook {
  /** Stable key used elsewhere in the same ParsedImport to reference this
   * notebook — a literal Joplin ID for JEX (from its type_=2 items), or a
   * `/`-joined path for markdown (derived from directory structure). Never
   * sent to Joplin as-is. */
  ref: string;
  title: string;
  /** ref of the parent notebook, or undefined for a top-level one. */
  parentRef?: string;
}

export interface ParsedTag {
  /** Literal Joplin ID (JEX) or the title itself (markdown, which has no
   * separate tag IDs — frontmatter/hashtags are just strings). */
  ref: string;
  title: string;
}

export interface ParsedNote {
  title: string;
  body: string;
  /** ref of the notebook this note belongs in (see ParsedNotebook.ref). */
  notebookRef: string;
  /** Tag refs (see ParsedTag.ref) this note should carry. */
  tagRefs: string[];
  isTodo: boolean;
  todoCompleted: boolean;
  createdTime?: number;
  updatedTime?: number;
  /** Original Joplin note ID, for JEX sources only — lets the second pass
   * rewrite `:/oldId` links to the newly-created note's ID. */
  sourceId?: string;
  /** Absolute source file path, for markdown sources only — lets the second
   * pass rewrite relative `[text](other.md)` links between imported files. */
  sourceFilePath?: string;
}

export interface ParsedResource {
  /** Original Joplin resource ID (JEX only) — what `:/id` tokens reference. */
  id: string;
  filename: string;
  mime?: string;
  data: Buffer;
}

export interface ParsedImport {
  notebooks: ParsedNotebook[];
  tags: ParsedTag[];
  notes: ParsedNote[];
  resources: ParsedResource[];
}

export function emptyParsedImport(): ParsedImport {
  return { notebooks: [], tags: [], notes: [], resources: [] };
}

export interface ImportReport {
  notebooksCreated: number;
  tagsCreated: number;
  notesCreated: number;
  notesSkipped: { title: string; reason: string }[];
  notesFailed: { title: string; error: string }[];
  resourcesUploaded: number;
  unresolvedLinks: number;
  /** Note was created successfully (counted in notesCreated) but rewriting its
   * links/resource tokens afterward failed — its body may still contain stale
   * `:/oldId` references. Distinct from notesFailed, where the note was never
   * created at all. */
  linkRewriteFailed: { title: string; error: string }[];
}

export function emptyImportReport(): ImportReport {
  return {
    notebooksCreated: 0,
    tagsCreated: 0,
    notesCreated: 0,
    notesSkipped: [],
    notesFailed: [],
    resourcesUploaded: 0,
    unresolvedLinks: 0,
    linkRewriteFailed: [],
  };
}

export type OnDuplicate = 'skip' | 'rename';

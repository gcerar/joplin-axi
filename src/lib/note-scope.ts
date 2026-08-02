import type { JoplinClient } from '../client.js';

export interface NoteScopeOptions {
  notebookId?: string;
  tagId?: string;
  query?: string;
  task?: boolean;
  fields: string[];
  /** Cap on the /search-derived candidate set when combined with other filters. */
  searchCap?: number;
}

const DEFAULT_SEARCH_CAP = 500;

/** Shared empty-match hint for any note-scope-filtered command (notes list, tags add/remove). */
export const CHECK_SCOPES_HINT = 'Run `joplin-axi notebooks list` or `joplin-axi tags list` to check available scopes.';

function byRecency(a: Record<string, any>, b: Record<string, any>): number {
  return Number(b.updated_time ?? 0) - Number(a.updated_time ?? 0);
}

/**
 * Resolves the set of notes matching any combination of notebook/tag/query/task
 * filters. --notebook and --tag each contribute a *full* candidate set fetched
 * from their own ID-scoped endpoint (no title ever touches a query string —
 * see notes.ts's `runList` design note for why that matters). --query/--task
 * contribute a capped candidate set from Joplin's real full-text search. When
 * more than one filter is active, the result is their intersection by note ID.
 *
 * With zero filters, returns *everything* (unbounded) — safe for a read-only
 * listing (notes list), but callers doing a mutation should validate that at
 * least one filter (or an explicit ID list) was given before calling this,
 * rather than relying on this function to refuse an unscoped "everything".
 */
export async function resolveNoteScope(client: JoplinClient, opts: NoteScopeOptions): Promise<Record<string, any>[]> {
  const searchCap = opts.searchCap ?? DEFAULT_SEARCH_CAP;
  const sources: Promise<{ items: Record<string, any>[] }>[] = [];

  if (opts.notebookId) sources.push(client.listNotes({ notebookId: opts.notebookId, fields: opts.fields, limit: Infinity }));
  if (opts.tagId) sources.push(client.listNotes({ tagId: opts.tagId, fields: opts.fields, limit: Infinity }));
  if (opts.query || opts.task) {
    const searchQuery = `${opts.query || '*'}${opts.task ? ' type:todo' : ''}`;
    sources.push(client.listNotes({ query: searchQuery, fields: opts.fields, limit: searchCap }));
  }

  if (sources.length === 0) {
    return (await client.listNotes({ fields: opts.fields, limit: Infinity })).items.sort(byRecency);
  }

  if (sources.length === 1) {
    return (await sources[0]).items.sort(byRecency);
  }

  const results = await Promise.all(sources);
  const noteById = new Map<string, Record<string, any>>();
  const idSets = results.map((r) => {
    const ids = new Set<string>();
    for (const n of r.items) {
      ids.add(n.id);
      noteById.set(n.id, n);
    }
    return ids;
  });
  const [first, ...rest] = idSets;
  const commonIds = [...first].filter((id) => rest.every((s) => s.has(id)));
  return commonIds.map((id) => noteById.get(id)!).sort(byRecency);
}

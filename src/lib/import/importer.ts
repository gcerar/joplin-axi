import * as path from 'node:path';
import type { JoplinClient } from '../../client.js';
import { emptyImportReport, ROOT_NOTEBOOK_REF, type ImportReport, type OnDuplicate, type ParsedImport, type ParsedNotebook } from './types.js';

export interface RunImportOptions {
  /** Required for markdown sources; an optional graft point for JEX sources
   * (whose notebook hierarchy is self-contained in the archive). */
  targetNotebookId?: string;
  onDuplicate: OnDuplicate;
}

const RESOURCE_LINK_RE = /:\/([0-9a-f]{32})/g;
const MD_LINK_RE = /\[([^\]]*)\]\(([^)]+)\)/g;

function generateUniqueTitle(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  for (let n = 1; n <= 1000; n++) {
    const candidate = `${base} (${n})`;
    if (!taken.has(candidate)) return candidate;
  }
  // Astronomically unlikely (1000 same-titled notes in one notebook) — fall
  // back to something guaranteed unique rather than looping forever.
  return `${base} (${Date.now()})`;
}

/**
 * Applies a ParsedImport (from markdown-source.ts or jex-source.ts) to a real
 * Joplin instance. Two passes: notes are created first with their original
 * body text, then a second pass re-fetches each created note and rewrites
 * `:/resourceId`/`:/noteId` tokens (and, for markdown sources, relative
 * `.md` links) to the newly-assigned IDs — this has to be a separate pass
 * because a note can link to another note that doesn't exist yet at
 * creation time. Resources are uploaded lazily, once per unique ID, the
 * first time a reference to them is actually encountered during rewriting.
 */
export async function runImport(parsed: ParsedImport, client: JoplinClient, opts: RunImportOptions): Promise<ImportReport> {
  const report = emptyImportReport();

  // --- Notebooks: resolve top-down, memoized by ref, so a nested chain of
  // arbitrary depth only ever creates each notebook once regardless of the
  // order notebooks appear in the array.
  const notebooksByRef = new Map(parsed.notebooks.map((n) => [n.ref, n]));
  const notebookIdByRef = new Map<string, string>();
  // Refs currently being resolved, further up the same call chain — lets a
  // cyclic parentRef (a corrupted/hand-crafted JEX where A's parent is B and
  // B's parent is A) be detected and broken instead of recursing forever:
  // notebookIdByRef is only populated *after* a resolution completes, so
  // without this a cycle would never hit the "already resolved" memo either.
  const resolvingRefs = new Set<string>();

  async function resolveNotebookId(ref: string): Promise<string | undefined> {
    if (ref === ROOT_NOTEBOOK_REF) return opts.targetNotebookId;
    if (notebookIdByRef.has(ref)) return notebookIdByRef.get(ref);

    const nb = notebooksByRef.get(ref) as ParsedNotebook | undefined;
    if (!nb) return undefined; // parser invariant violated — treat as "no notebook" rather than throwing mid-batch.
    if (resolvingRefs.has(ref)) return undefined; // cyclic parentRef — treat the cycle-closing edge as "no parent".

    resolvingRefs.add(ref);
    try {
      const parentId = nb.parentRef ? await resolveNotebookId(nb.parentRef) : opts.targetNotebookId;
      const created = await client.createNotebook(parentId ? { title: nb.title, parent_id: parentId } : { title: nb.title });
      notebookIdByRef.set(ref, created.id);
      report.notebooksCreated++;
      return created.id;
    } finally {
      resolvingRefs.delete(ref);
    }
  }

  for (const nb of parsed.notebooks) await resolveNotebookId(nb.ref);

  // --- Tags: existing tags fetched once (not per tag — avoids both an N+1
  // round trip and a check-then-create race across concurrent lookups of the
  // same not-yet-existing title), then create-if-missing per unique title.
  const existingTagIdByTitle = new Map((await client.listTags()).map((t) => [t.title as string, t.id as string]));
  const tagIdByRef = new Map<string, string>();
  for (const tag of parsed.tags) {
    const existingId = existingTagIdByTitle.get(tag.title);
    if (existingId) {
      tagIdByRef.set(tag.ref, existingId);
    } else {
      const created = await client.createTag(tag.title);
      tagIdByRef.set(tag.ref, created.id);
      existingTagIdByTitle.set(tag.title, created.id);
      report.tagsCreated++;
    }
  }

  // --- Resources: uploaded lazily during the link-rewrite pass, not here —
  // no point uploading one that never actually gets referenced by a created
  // note (e.g. because that note was skipped as a duplicate).
  const resourcesById = new Map(parsed.resources.map((r) => [r.id, r]));
  const resourceIdByOldId = new Map<string, string>();
  const knownSourceIds = new Set(parsed.notes.map((n) => n.sourceId).filter((id): id is string => Boolean(id)));

  // --- Existing-title cache per notebook, for dedup — populated lazily
  // (only for notebooks an import actually targets), fetched by ID never by
  // title-in-a-search-query, consistent with src/lib/note-scope.ts's
  // anti-injection reasoning.
  const existingTitlesByNotebook = new Map<string, Set<string>>();
  async function existingTitles(notebookId: string | undefined): Promise<Set<string>> {
    const key = notebookId ?? '';
    if (existingTitlesByNotebook.has(key)) return existingTitlesByNotebook.get(key)!;
    const items = notebookId
      ? (await client.listNotes({ notebookId, fields: ['id', 'title'], limit: Infinity })).items
      : [];
    const set = new Set(items.map((n) => String(n.title)));
    existingTitlesByNotebook.set(key, set);
    return set;
  }

  // --- Pass 1: create notes with their original body, unrewritten.
  const noteIdBySourceId = new Map<string, string>();
  const noteIdByFilePath = new Map<string, string>();

  for (const note of parsed.notes) {
    try {
      const notebookId = await resolveNotebookId(note.notebookRef);
      const taken = await existingTitles(notebookId);

      let title = note.title;
      if (taken.has(title)) {
        if (opts.onDuplicate === 'skip') {
          report.notesSkipped.push({ title, reason: `duplicate title in target notebook` });
          continue;
        }
        title = generateUniqueTitle(title, taken);
      }

      const created = await client.createNote({
        title,
        body: note.body,
        ...(notebookId ? { parent_id: notebookId } : {}),
        is_todo: note.isTodo ? 1 : 0,
        todo_completed: note.todoCompleted ? 1 : 0,
      });
      // Only mark the title as taken once creation actually succeeded — doing
      // this before the call would make a same-titled note later in the batch
      // look like a real duplicate even if this one failed and doesn't exist.
      taken.add(title);

      const timeFields: Record<string, unknown> = {};
      if (note.createdTime !== undefined) timeFields.created_time = Math.round(note.createdTime);
      if (note.updatedTime !== undefined) timeFields.updated_time = Math.round(note.updatedTime);
      if (Object.keys(timeFields).length) await client.updateNote(created.id, timeFields);

      for (const tagRef of note.tagRefs) {
        const tagId = tagIdByRef.get(tagRef);
        if (tagId) await client.addTagToNote(tagId, created.id);
      }

      report.notesCreated++;
      if (note.sourceId) noteIdBySourceId.set(note.sourceId, created.id);
      if (note.sourceFilePath) noteIdByFilePath.set(note.sourceFilePath, created.id);
    } catch (e) {
      report.notesFailed.push({ title: note.title, error: e instanceof Error ? e.message : String(e) });
    }
  }

  // --- Pass 2: rewrite links now that every note in the batch has an ID.
  async function uploadResourceOnce(oldId: string): Promise<string | undefined> {
    if (resourceIdByOldId.has(oldId)) return resourceIdByOldId.get(oldId);
    const res = resourcesById.get(oldId);
    if (!res) return undefined;
    const created = await client.createResource(res.data, res.filename, res.mime);
    resourceIdByOldId.set(oldId, created.id);
    report.resourcesUploaded++;
    return created.id;
  }

  for (const note of parsed.notes) {
    const newId = note.sourceId ? noteIdBySourceId.get(note.sourceId) : note.sourceFilePath ? noteIdByFilePath.get(note.sourceFilePath) : undefined;
    if (!newId) continue; // skipped as a duplicate, or failed to create — nothing to rewrite.

    // Isolated per note — a failure here (e.g. a transient network error, or
    // the note being deleted concurrently) must not abort the rest of the
    // batch or lose the report entirely: pass 1 already wrote real notes to
    // Joplin by this point, so an uncaught throw here would discard all
    // visibility into what actually happened. The note itself still exists
    // (just with stale :/oldId references), which is why this is tracked
    // separately from notesFailed rather than folded into it.
    try {
      const current = await client.getNote(newId, ['id', 'body']);
      let body = String(current.body ?? '');
      let changed = false;

      // Resource tokens (:/id) can appear in either source's body now: JEX
      // notes have them natively, and markdown-source.ts embeds the same shape
      // for local asset links (see its rewriteAssetLinks doc comment) so this
      // one pass handles both without caring which source produced the note.
      // Note-ID tokens (the knownSourceIds branch) are JEX-only in practice —
      // markdown notes have no sourceId to begin with, so that branch simply
      // never matches for them.
      {
        const replacements: [string, string][] = [];
        for (const m of body.matchAll(RESOURCE_LINK_RE)) {
          const oldId = m[1];
          if (resourcesById.has(oldId)) {
            const newResourceId = await uploadResourceOnce(oldId);
            if (newResourceId) replacements.push([oldId, newResourceId]);
            else report.unresolvedLinks++;
          } else if (knownSourceIds.has(oldId)) {
            const mappedNoteId = noteIdBySourceId.get(oldId);
            if (mappedNoteId) replacements.push([oldId, mappedNoteId]);
            else report.unresolvedLinks++; // linked note existed in the source but was skipped/failed.
          }
          // Anything else (an ID that isn't part of this import batch at all)
          // is left completely alone — it may be a valid reference to a note
          // that already existed in Joplin before this import.
        }
        for (const [oldId, replacementId] of replacements) {
          const before = body;
          body = body.split(`:/${oldId}`).join(`:/${replacementId}`);
          if (body !== before) changed = true;
        }
      }

      if (note.sourceFilePath) {
        const dir = path.dirname(note.sourceFilePath);
        let rewritten = '';
        let lastIndex = 0;
        for (const m of body.matchAll(MD_LINK_RE)) {
          const [full, text, target] = m;
          if (/^https?:\/\//.test(target) || target.startsWith(':/')) continue;
          const resolved = path.resolve(dir, target);
          const targetNoteId = noteIdByFilePath.get(resolved);
          if (targetNoteId) {
            rewritten += body.slice(lastIndex, m.index) + `[${text}](:/${targetNoteId})`;
            lastIndex = (m.index ?? 0) + full.length;
            changed = true;
          } else if (parsed.notes.some((n) => n.sourceFilePath === resolved)) {
            report.unresolvedLinks++; // pointed at another imported file that was skipped/failed.
          }
        }
        if (changed) body = rewritten + body.slice(lastIndex);
      }

      if (changed) await client.updateNote(newId, { body });
    } catch (e) {
      report.linkRewriteFailed.push({ title: note.title, error: e instanceof Error ? e.message : String(e) });
    }
  }

  return report;
}

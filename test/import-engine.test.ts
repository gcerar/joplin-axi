import { describe, expect, it, vi } from 'vitest';
import { runImport } from '../src/lib/import/importer.js';
import { emptyParsedImport, ROOT_NOTEBOOK_REF, type ParsedImport } from '../src/lib/import/types.js';
import { fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  createNotebook: vi.fn(),
  createTag: vi.fn(),
  listTags: vi.fn().mockResolvedValue([]),
  createNote: vi.fn(),
  updateNote: vi.fn(),
  addTagToNote: vi.fn(),
  getNote: vi.fn(),
  createResource: vi.fn(),
  listNotes: vi.fn().mockResolvedValue({ items: [] }),
});

function baseParsed(): ParsedImport {
  return emptyParsedImport();
}

describe('runImport — notebooks', () => {
  it('creates a nested notebook chain parent-before-child, memoized by ref', async () => {
    const createNotebook = vi
      .fn()
      .mockResolvedValueOnce({ id: 'parent-id', title: 'Parent' })
      .mockResolvedValueOnce({ id: 'child-id', title: 'Child' });
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const client = fakeClient({ createNotebook, createNote });

    const parsed = baseParsed();
    parsed.notebooks = [
      { ref: 'child', title: 'Child', parentRef: 'parent' },
      { ref: 'parent', title: 'Parent', parentRef: undefined },
    ];
    parsed.notes = [{ title: 'N', body: 'b', notebookRef: 'child', tagRefs: [], isTodo: false, todoCompleted: false }];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(createNotebook).toHaveBeenCalledTimes(2);
    expect(createNotebook).toHaveBeenNthCalledWith(1, { title: 'Parent' });
    expect(createNotebook).toHaveBeenNthCalledWith(2, { title: 'Child', parent_id: 'parent-id' });
    expect(createNote).toHaveBeenCalledWith(expect.objectContaining({ parent_id: 'child-id' }));
    expect(report.notebooksCreated).toBe(2);
  });

  it('grafts a root-level notebook under the --notebook target when given', async () => {
    const createNotebook = vi.fn().mockResolvedValue({ id: 'new-id', title: 'Top' });
    const client = fakeClient({ createNotebook });

    const parsed = baseParsed();
    parsed.notebooks = [{ ref: 'top', title: 'Top', parentRef: undefined }];

    await runImport(parsed, client, { targetNotebookId: 'graft-target', onDuplicate: 'skip' });

    expect(createNotebook).toHaveBeenCalledWith({ title: 'Top', parent_id: 'graft-target' });
  });

  it('sends a note with ROOT_NOTEBOOK_REF directly to the --notebook target, with no notebook creation', async () => {
    const createNotebook = vi.fn();
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const client = fakeClient({ createNotebook, createNote });

    const parsed = baseParsed();
    parsed.notes = [{ title: 'N', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false }];

    await runImport(parsed, client, { targetNotebookId: 'target-nb', onDuplicate: 'skip' });

    expect(createNotebook).not.toHaveBeenCalled();
    expect(createNote).toHaveBeenCalledWith(expect.objectContaining({ parent_id: 'target-nb' }));
  });
});

describe('runImport — tags', () => {
  it('creates a tag once and applies it to every note referencing it', async () => {
    const createTag = vi.fn().mockResolvedValue({ id: 'tag-id', title: 'work' });
    const addTagToNote = vi.fn();
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const client = fakeClient({ createTag, addTagToNote, createNote, listTags: vi.fn().mockResolvedValue([]) });

    const parsed = baseParsed();
    parsed.tags = [{ ref: 'work', title: 'work' }];
    parsed.notes = [{ title: 'N', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: ['work'], isTodo: false, todoCompleted: false }];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(createTag).toHaveBeenCalledTimes(1);
    expect(addTagToNote).toHaveBeenCalledWith('tag-id', 'note-id');
    expect(report.tagsCreated).toBe(1);
  });

  it('reuses an existing tag by title instead of creating a duplicate', async () => {
    const createTag = vi.fn();
    const listTags = vi.fn().mockResolvedValue([{ id: 'existing-id', title: 'work' }]);
    const client = fakeClient({ createTag, listTags });

    const parsed = baseParsed();
    parsed.tags = [{ ref: 'work', title: 'work' }];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(createTag).not.toHaveBeenCalled();
    expect(report.tagsCreated).toBe(0);
  });
});

describe('runImport — timestamps', () => {
  it('follows up with updateNote for created_time/updated_time when present', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'N', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, createdTime: 1000, updatedTime: 2000 },
    ];

    await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).toHaveBeenCalledWith('note-id', { created_time: 1000, updated_time: 2000 });
  });

  it('skips the timestamp updateNote call entirely when neither is set', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [{ title: 'N', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false }];

    await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).not.toHaveBeenCalled();
  });
});

describe('runImport — dedup', () => {
  it('skips a note whose title already exists in the target notebook (default skip policy)', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [{ id: 'existing', title: 'Dup' }] });
    const createNote = vi.fn();
    const client = fakeClient({ listNotes, createNote });

    const parsed = baseParsed();
    parsed.notes = [{ title: 'Dup', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false }];

    const report = await runImport(parsed, client, { targetNotebookId: 'nb1', onDuplicate: 'skip' });

    expect(createNote).not.toHaveBeenCalled();
    expect(report.notesSkipped).toEqual([{ title: 'Dup', reason: 'duplicate title in target notebook' }]);
  });

  it('renames a duplicate title with a numeric suffix under the rename policy', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [{ id: 'existing', title: 'Dup' }] });
    const createNote = vi.fn().mockResolvedValue({ id: 'new-id' });
    const client = fakeClient({ listNotes, createNote });

    const parsed = baseParsed();
    parsed.notes = [{ title: 'Dup', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false }];

    await runImport(parsed, client, { targetNotebookId: 'nb1', onDuplicate: 'rename' });

    expect(createNote).toHaveBeenCalledWith(expect.objectContaining({ title: 'Dup (1)' }));
  });

  it('renames a second within-batch duplicate to (2), not (1) again', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const createNote = vi.fn().mockResolvedValueOnce({ id: 'id1' }).mockResolvedValueOnce({ id: 'id2' }).mockResolvedValueOnce({ id: 'id3' });
    const client = fakeClient({ listNotes, createNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'Same', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
      { title: 'Same', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
      { title: 'Same', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
    ];

    await runImport(parsed, client, { targetNotebookId: 'nb1', onDuplicate: 'rename' });

    expect(createNote).toHaveBeenNthCalledWith(1, expect.objectContaining({ title: 'Same' }));
    expect(createNote).toHaveBeenNthCalledWith(2, expect.objectContaining({ title: 'Same (1)' }));
    expect(createNote).toHaveBeenNthCalledWith(3, expect.objectContaining({ title: 'Same (2)' }));
  });
});

describe('runImport — link rewriting (second pass, JEX-style :/id tokens)', () => {
  it('rewrites a forward reference to a note created later in the batch', async () => {
    const createNote = vi.fn().mockResolvedValueOnce({ id: 'new-a' }).mockResolvedValueOnce({ id: 'new-b' });
    const getNote = vi.fn().mockImplementation((id: string) => {
      if (id === 'new-a') return Promise.resolve({ id: 'new-a', body: 'See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).' });
      return Promise.resolve({ id: 'new-b', body: 'no links' });
    });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'A', body: 'See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
      { title: 'B', body: 'no links', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).toHaveBeenCalledWith('new-a', { body: 'See [B](:/new-b).' });
    expect(report.unresolvedLinks).toBe(0);
  });

  it('uploads a referenced resource exactly once and rewrites the token to the new resource ID', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'new-a' });
    const getNote = vi.fn().mockResolvedValue({ id: 'new-a', body: 'Photo: ![x](:/cccccccccccccccccccccccccccccccc).' });
    const createResource = vi.fn().mockResolvedValue({ id: 'new-res', title: 'photo.png' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, createResource, updateNote });

    const parsed = baseParsed();
    parsed.resources = [{ id: 'cccccccccccccccccccccccccccccccc', filename: 'photo.png', mime: 'image/png', data: Buffer.from([1, 2, 3]) }];
    parsed.notes = [
      { title: 'A', body: 'Photo: ![x](:/cccccccccccccccccccccccccccccccc).', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(createResource).toHaveBeenCalledTimes(1);
    expect(createResource).toHaveBeenCalledWith(Buffer.from([1, 2, 3]), 'photo.png', 'image/png');
    expect(updateNote).toHaveBeenCalledWith('new-a', { body: 'Photo: ![x](:/new-res).' });
    expect(report.resourcesUploaded).toBe(1);
  });

  it('leaves a :/id token untouched and uncounted when it belongs to neither this batch\'s notes nor resources', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'new-a' });
    const getNote = vi.fn().mockResolvedValue({ id: 'new-a', body: 'Pre-existing link: [x](:/ffffffffffffffffffffffffffffffff).' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      {
        title: 'A',
        body: 'Pre-existing link: [x](:/ffffffffffffffffffffffffffffffff).',
        notebookRef: ROOT_NOTEBOOK_REF,
        tagRefs: [],
        isTodo: false,
        todoCompleted: false,
        sourceId: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).not.toHaveBeenCalled();
    expect(report.unresolvedLinks).toBe(0);
  });

  it('counts an unresolved link when the referenced note was skipped as a duplicate', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [{ id: 'existing', title: 'B' }] });
    const createNote = vi.fn().mockResolvedValueOnce({ id: 'new-a' });
    const getNote = vi.fn().mockResolvedValue({ id: 'new-a', body: 'See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).' });
    const updateNote = vi.fn();
    const client = fakeClient({ listNotes, createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'A', body: 'See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
      { title: 'B', body: 'no links', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' },
    ];

    const report = await runImport(parsed, client, { targetNotebookId: 'nb1', onDuplicate: 'skip' });

    expect(report.notesSkipped).toHaveLength(1);
    expect(report.unresolvedLinks).toBe(1);
    expect(updateNote).not.toHaveBeenCalled();
  });
});

describe('runImport — link rewriting (markdown relative-path links)', () => {
  it('rewrites a relative .md link to an internal :/id link once the target note exists', async () => {
    const createNote = vi.fn().mockResolvedValueOnce({ id: 'new-a' }).mockResolvedValueOnce({ id: 'new-b' });
    const getNote = vi.fn().mockImplementation((id: string) =>
      id === 'new-a' ? Promise.resolve({ id: 'new-a', body: 'See [B](./b.md) for details.' }) : Promise.resolve({ id: 'new-b', body: 'body' })
    );
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      {
        title: 'A',
        body: 'See [B](./b.md) for details.',
        notebookRef: ROOT_NOTEBOOK_REF,
        tagRefs: [],
        isTodo: false,
        todoCompleted: false,
        sourceFilePath: '/import/a.md',
      },
      { title: 'B', body: 'body', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceFilePath: '/import/b.md' },
    ];

    await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).toHaveBeenCalledWith('new-a', { body: 'See [B](:/new-b) for details.' });
  });

  it('leaves an external http(s) link untouched', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'new-a' });
    const getNote = vi.fn().mockResolvedValue({ id: 'new-a', body: 'See [site](https://example.com).' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      {
        title: 'A',
        body: 'See [site](https://example.com).',
        notebookRef: ROOT_NOTEBOOK_REF,
        tagRefs: [],
        isTodo: false,
        todoCompleted: false,
        sourceFilePath: '/import/a.md',
      },
    ];

    await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(updateNote).not.toHaveBeenCalled();
  });

  it('uploads a markdown-sourced local asset (a note with sourceFilePath but no sourceId) — regression for the resource pass being gated on sourceId', async () => {
    const assetId = 'd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1';
    const createNote = vi.fn().mockResolvedValue({ id: 'new-a' });
    const getNote = vi.fn().mockResolvedValue({ id: 'new-a', body: `![diagram](:/${assetId})` });
    const createResource = vi.fn().mockResolvedValue({ id: 'uploaded-res', title: 'diagram.png' });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, createResource, updateNote });

    const parsed = baseParsed();
    parsed.resources = [{ id: assetId, filename: 'diagram.png', mime: 'image/png', data: Buffer.from([9, 9, 9]) }];
    parsed.notes = [
      {
        title: 'A',
        body: `![diagram](:/${assetId})`,
        notebookRef: ROOT_NOTEBOOK_REF,
        tagRefs: [],
        isTodo: false,
        todoCompleted: false,
        sourceFilePath: '/import/a.md',
        // deliberately no sourceId — this is what a markdown-sourced note looks like.
      },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(createResource).toHaveBeenCalledWith(Buffer.from([9, 9, 9]), 'diagram.png', 'image/png');
    expect(updateNote).toHaveBeenCalledWith('new-a', { body: '![diagram](:/uploaded-res)' });
    expect(report.resourcesUploaded).toBe(1);
  });
});

describe('runImport — failures', () => {
  it('records a note as failed without aborting the rest of the batch', async () => {
    const createNote = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ id: 'ok-id' });
    const client = fakeClient({ createNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'Bad', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
      { title: 'Good', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    expect(report.notesFailed).toEqual([{ title: 'Bad', error: 'boom' }]);
    expect(report.notesCreated).toBe(1);
  });

  it('records a link-rewrite failure without losing the rest of the report or aborting later notes', async () => {
    const createNote = vi.fn().mockResolvedValueOnce({ id: 'new-a' }).mockResolvedValueOnce({ id: 'new-b' });
    const getNote = vi.fn().mockImplementation((id: string) => {
      if (id === 'new-a') return Promise.reject(new Error('network blip'));
      return Promise.resolve({ id: 'new-b', body: 'no links' });
    });
    const updateNote = vi.fn();
    const client = fakeClient({ createNote, getNote, updateNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'A', body: 'a', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
      { title: 'B', body: 'no links', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false, sourceId: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    // Both notes were still created in pass 1 — the pass-2 failure on A must
    // not prevent B's own link rewrite from running, and must not throw out
    // of runImport losing the whole report.
    expect(report.notesCreated).toBe(2);
    expect(report.linkRewriteFailed).toEqual([{ title: 'A', error: 'network blip' }]);
    expect(getNote).toHaveBeenCalledWith('new-b', ['id', 'body']);
  });

  it('does not treat a duplicate title as taken when its predecessor failed to create', async () => {
    const createNote = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ id: 'second-id' });
    const client = fakeClient({ createNote });

    const parsed = baseParsed();
    parsed.notes = [
      { title: 'Same', body: 'a', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
      { title: 'Same', body: 'b', notebookRef: ROOT_NOTEBOOK_REF, tagRefs: [], isTodo: false, todoCompleted: false },
    ];

    const report = await runImport(parsed, client, { onDuplicate: 'skip' });

    // The first "Same" failed to create, so the second one isn't a real
    // duplicate — it must still be attempted (and succeed) with its own title.
    expect(createNote).toHaveBeenCalledTimes(2);
    expect(createNote).toHaveBeenNthCalledWith(2, expect.objectContaining({ title: 'Same' }));
    expect(report.notesFailed).toEqual([{ title: 'Same', error: 'boom' }]);
    expect(report.notesSkipped).toEqual([]);
    expect(report.notesCreated).toBe(1);
  });

  it('resolves a cyclic notebook parentRef chain without hanging, treating the cycle-closing edge as top-level', async () => {
    const createNotebook = vi.fn().mockImplementation((fields: { title: string }) => Promise.resolve({ id: `id-${fields.title}`, title: fields.title }));
    const createNote = vi.fn().mockResolvedValue({ id: 'note-id' });
    const client = fakeClient({ createNotebook, createNote });

    const parsed = baseParsed();
    parsed.notebooks = [
      { ref: 'a', title: 'A', parentRef: 'b' },
      { ref: 'b', title: 'B', parentRef: 'a' },
    ];
    parsed.notes = [{ title: 'Note', body: 'x', notebookRef: 'a', tagRefs: [], isTodo: false, todoCompleted: false }];

    const report = await Promise.race([
      runImport(parsed, client, { onDuplicate: 'skip' }),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('runImport hung on a notebook cycle')), 2000)),
    ]);

    expect(report.notebooksCreated).toBe(2);
    expect(createNote).toHaveBeenCalledWith(expect.objectContaining({ parent_id: 'id-A' }));
  });
});

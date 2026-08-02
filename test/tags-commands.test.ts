import { describe, expect, it, vi } from 'vitest';
import { tagsCommands } from '../src/commands/tags.js';
import { UsageError } from '../src/lib/args.js';
import { args, exitCodeOf, fakeClientFactory, textOf } from './helpers.js';

const fakeClient = fakeClientFactory({
  listTags: vi.fn().mockResolvedValue([{ id: 'tag1', title: 'active' }]),
  getTagsByNote: vi.fn(),
  createTag: vi.fn(),
  updateTag: vi.fn(),
  deleteTag: vi.fn(),
  addTagToNote: vi.fn(),
  removeTagFromNote: vi.fn(),
  getNote: vi.fn().mockImplementation((id: string) => Promise.resolve({ id, title: `note-${id}` })),
  listNotes: vi.fn().mockResolvedValue({ items: [] }),
});

describe('tags create', () => {
  it('requires <title>', async () => {
    const client = fakeClient();
    await expect(tagsCommands.create.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('creates a tag and reports its id', async () => {
    const createTag = vi.fn().mockResolvedValue({ id: 'tag2', title: 'urgent' });
    const client = fakeClient({ createTag });

    const out = await tagsCommands.create.run(args(['urgent']), client);

    expect(createTag).toHaveBeenCalledWith('urgent');
    expect(out).toContain('id: tag2');
  });
});

describe('tags update', () => {
  it('requires <id> and <title>', async () => {
    const client = fakeClient();
    await expect(tagsCommands.update.run(args(['tag1']), client)).rejects.toThrow(UsageError);
  });

  it('renames a tag', async () => {
    const updateTag = vi.fn().mockResolvedValue({ id: 'tag1', title: 'renamed' });
    const client = fakeClient({ updateTag });

    await tagsCommands.update.run(args(['tag1', 'renamed']), client);

    expect(updateTag).toHaveBeenCalledWith('tag1', 'renamed');
  });
});

describe('tags of', () => {
  it('shows a hint for adding a tag to the note', async () => {
    const client = fakeClient({ getTagsByNote: vi.fn().mockResolvedValue([]) });
    const out = await tagsCommands.of.run(args(['n1']), client);
    expect(out).toContain('tags add <title> --notes n1');
  });
});

describe('tags delete', () => {
  it('requires an id', async () => {
    const client = fakeClient();
    await expect(tagsCommands.delete.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('deletes a tag', async () => {
    const deleteTag = vi.fn();
    const client = fakeClient({ deleteTag });

    const out = await tagsCommands.delete.run(args(['tag1']), client);

    expect(deleteTag).toHaveBeenCalledWith('tag1');
    expect(out).toContain('deleted: true');
  });
});

describe('tags add', () => {
  it('requires <tag-title>', async () => {
    const client = fakeClient();
    await expect(tagsCommands.add.run(args([], { notes: 'n1' }), client)).rejects.toThrow(UsageError);
  });

  it('requires --notes or a filter', async () => {
    const client = fakeClient();
    await expect(tagsCommands.add.run(args(['active']), client)).rejects.toThrow(UsageError);
  });

  it('rejects --notes combined with a filter', async () => {
    const client = fakeClient();
    await expect(tagsCommands.add.run(args(['active'], { notes: 'n1', query: 'x' }), client)).rejects.toThrow(UsageError);
  });

  it('errors if the tag title does not exist', async () => {
    const client = fakeClient({ listTags: vi.fn().mockResolvedValue([]) });
    await expect(tagsCommands.add.run(args(['nonexistent'], { notes: 'n1' }), client)).rejects.toThrow(UsageError);
  });

  it('resolves the tag title to an id and applies it to each explicit note', async () => {
    const addTagToNote = vi.fn();
    const client = fakeClient({ addTagToNote });

    const out = await tagsCommands.add.run(args(['active'], { notes: 'n1,n2' }), client);

    expect(addTagToNote).toHaveBeenCalledTimes(2);
    expect(addTagToNote).toHaveBeenNthCalledWith(1, 'tag1', 'n1');
    expect(addTagToNote).toHaveBeenNthCalledWith(2, 'tag1', 'n2');
    expect(out).toContain('added_to: 2');
    expect(out).toContain('note-n1');
    expect(typeof out).toBe('string'); // no failures — plain string, not {output, exitCode}
  });

  it('tags the valid explicit notes and reports an unresolvable ID as failed, instead of aborting the whole batch', async () => {
    const addTagToNote = vi.fn();
    const getNote = vi.fn().mockImplementation((id: string) =>
      id === 'badid' ? Promise.reject(new Error('Joplin API GET /notes/badid failed: 404')) : Promise.resolve({ id, title: `note-${id}` })
    );
    const client = fakeClient({ addTagToNote, getNote });

    const result = await tagsCommands.add.run(args(['active'], { notes: 'n1,badid,n3' }), client);

    expect(addTagToNote).toHaveBeenCalledTimes(2);
    expect(addTagToNote).toHaveBeenCalledWith('tag1', 'n1');
    expect(addTagToNote).toHaveBeenCalledWith('tag1', 'n3');
    expect(addTagToNote).not.toHaveBeenCalledWith('tag1', 'badid');
    expect(textOf(result)).toContain('added_to: 2');
    expect(textOf(result)).toContain('failed: 1');
    expect(textOf(result)).toContain('badid');
    expect(textOf(result)).toContain('404');
    expect(exitCodeOf(result)).toBe(1);
  });

  it('selects notes by filter (--notebook) instead of explicit IDs', async () => {
    const addTagToNote = vi.fn();
    const listNotes = vi.fn().mockResolvedValue({
      items: [
        { id: 'm1', title: 'Matched One', updated_time: 100 },
        { id: 'm2', title: 'Matched Two', updated_time: 200 },
      ],
    });
    const client = fakeClient({ addTagToNote, listNotes });

    const out = await tagsCommands.add.run(args(['active'], { notebook: 'nb1' }), client);

    expect(listNotes).toHaveBeenCalledWith(expect.objectContaining({ notebookId: 'nb1' }));
    expect(addTagToNote).toHaveBeenCalledTimes(2);
    expect(out).toContain('added_to: 2');
    expect(out).toContain('Matched One');
    expect(out).toContain('Matched Two');
    expect(out).toContain('notes list --tag active');
  });

  it('reports a definitive zero when the filter matches nothing, without erroring', async () => {
    const addTagToNote = vi.fn();
    const client = fakeClient({ addTagToNote, listNotes: vi.fn().mockResolvedValue({ items: [] }) });

    const out = await tagsCommands.add.run(args(['active'], { notebook: 'nb1' }), client);

    expect(addTagToNote).not.toHaveBeenCalled();
    expect(out).toContain('added_to: 0');
    expect(out).toContain('nothing tagged');
    expect(out).toContain('notebooks list');
  });

  it('reports both successes and failures when one note fails mid-batch, instead of throwing and hiding the rest', async () => {
    const addTagToNote = vi.fn().mockImplementation((_tagId: string, noteId: string) =>
      noteId === 'n2' ? Promise.reject(new Error('note locked')) : Promise.resolve()
    );
    const client = fakeClient({ addTagToNote });

    const result = await tagsCommands.add.run(args(['active'], { notes: 'n1,n2,n3' }), client);

    expect(addTagToNote).toHaveBeenCalledTimes(3);
    expect(textOf(result)).toContain('added_to: 2');
    expect(textOf(result)).toContain('failed: 1');
    expect(textOf(result)).toContain('note-n1');
    expect(textOf(result)).toContain('note-n3');
    expect(textOf(result)).toContain('note-n2');
    expect(textOf(result)).toContain('note locked');
    expect(textOf(result)).toContain('retry with');
    expect(exitCodeOf(result)).toBe(1);
  });
});

describe('tags remove', () => {
  it('resolves the tag title to an id and removes it from each note', async () => {
    const removeTagFromNote = vi.fn();
    const client = fakeClient({ removeTagFromNote });

    const out = await tagsCommands.remove.run(args(['active'], { notes: 'n1' }), client);

    expect(removeTagFromNote).toHaveBeenCalledWith('tag1', 'n1');
    expect(out).toContain('removed_from: 1');
  });

  it('selects notes by filter (--query) instead of explicit IDs', async () => {
    const removeTagFromNote = vi.fn();
    const listNotes = vi.fn().mockResolvedValue({ items: [{ id: 'm1', title: 'Matched', updated_time: 100 }] });
    const client = fakeClient({ removeTagFromNote, listNotes });

    const out = await tagsCommands.remove.run(args(['active'], { query: 'x' }), client);

    expect(listNotes).toHaveBeenCalledWith(expect.objectContaining({ query: 'x' }));
    expect(removeTagFromNote).toHaveBeenCalledWith('tag1', 'm1');
    expect(out).toContain('removed_from: 1');
    expect(out).toContain('tags add active --notes');
  });

  it('reports a definitive zero when the filter matches nothing, without erroring', async () => {
    const removeTagFromNote = vi.fn();
    const client = fakeClient({ removeTagFromNote, listNotes: vi.fn().mockResolvedValue({ items: [] }) });

    const out = await tagsCommands.remove.run(args(['active'], { notebook: 'nb1' }), client);

    expect(removeTagFromNote).not.toHaveBeenCalled();
    expect(out).toContain('removed_from: 0');
    expect(out).toContain('nothing removed');
  });
});

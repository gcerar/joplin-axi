import { describe, expect, it, vi } from 'vitest';
import { notesCommands } from '../src/commands/notes.js';
import { UsageError } from '../src/lib/args.js';
import { args as argsWithPositionals, fakeClientFactory } from './helpers.js';

function args(flags: Record<string, string | boolean | number> = {}) {
  return argsWithPositionals([], flags);
}

const fakeClient = fakeClientFactory({
  listTags: vi.fn().mockResolvedValue([{ id: 'tag1', title: 'active' }]),
  listNotebooks: vi.fn().mockResolvedValue([{ id: 'nb1', title: 'Inbox', parent_id: '' }]),
  listNotes: vi.fn().mockResolvedValue({ items: [] }),
});

const D = { limit: 20, task: false, trash: false };

function note(id: string, updated: number, extra: Record<string, unknown> = {}) {
  return { id, title: `note-${id}`, parent_id: 'nb1', updated_time: updated, ...extra };
}

describe('notes list — --limit validation', () => {
  it('rejects a non-positive --limit instead of silently returning an empty result', async () => {
    const client = fakeClient();
    await expect(notesCommands.list.run(args({ ...D, limit: 0 }), client)).rejects.toThrow(UsageError);
    await expect(notesCommands.list.run(args({ ...D, limit: -5 }), client)).rejects.toThrow(UsageError);
  });
});

describe('notes list — --trash stays exclusive', () => {
  it('rejects --trash combined with --query', async () => {
    const client = fakeClient();
    await expect(notesCommands.list.run(args({ ...D, trash: true, query: 'x' }), client)).rejects.toThrow(UsageError);
  });

  it('rejects --trash combined with --notebook', async () => {
    const client = fakeClient();
    await expect(notesCommands.list.run(args({ ...D, trash: true, notebook: 'nb1' }), client)).rejects.toThrow(UsageError);
  });
});

describe('notes list — single filter (unchanged fast paths)', () => {
  it('--notebook alone hits the notebook-scoped endpoint', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [note('n1', 100)] });
    const client = fakeClient({ listNotes });

    await notesCommands.list.run(args({ ...D, notebook: 'nb1' }), client);

    expect(listNotes).toHaveBeenCalledTimes(1);
    expect(listNotes.mock.calls[0][0].notebookId).toBe('nb1');
    expect(listNotes.mock.calls[0][0].query).toBeUndefined();
  });

  it('--query alone hits search with the plain text (no type:todo)', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const client = fakeClient({ listNotes });

    await notesCommands.list.run(args({ ...D, query: 'annual report' }), client);

    expect(listNotes.mock.calls[0][0].query).toBe('annual report');
  });

  it('--task alone appends type:todo to a wildcard search', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const client = fakeClient({ listNotes });

    await notesCommands.list.run(args({ ...D, task: true }), client);

    expect(listNotes.mock.calls[0][0].query).toBe('* type:todo');
  });

  it('no filters at all uses the plain unfiltered listing', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const client = fakeClient({ listNotes });

    await notesCommands.list.run(args({ ...D }), client);

    const call = listNotes.mock.calls[0][0];
    expect(call.query).toBeUndefined();
    expect(call.notebookId).toBeUndefined();
    expect(call.tagId).toBeUndefined();
  });
});

describe('notes list — combined filters now intersect by ID', () => {
  it('--query + --notebook: only notes in both sets survive', async () => {
    const listNotes = vi
      .fn()
      // notebook source (full fetch)
      .mockResolvedValueOnce({ items: [note('n1', 300), note('n2', 200)] })
      // query source (capped fetch)
      .mockResolvedValueOnce({ items: [note('n2', 200), note('n3', 100)] });
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', query: 'x' }), client);

    expect(listNotes).toHaveBeenCalledTimes(2);
    expect(listNotes.mock.calls[0][0].notebookId).toBe('nb1');
    expect(listNotes.mock.calls[1][0].query).toBe('x');
    expect(out).toContain('note-n2');
    expect(out).not.toContain('note-n1');
    expect(out).not.toContain('note-n3');
  });

  it('--query + --tag: intersects by ID the same way', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('n1', 300)] }) // tag source
      .mockResolvedValueOnce({ items: [note('n1', 300), note('n2', 200)] }); // query source
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, tag: 'active', query: 'x' }), client);

    expect(listNotes.mock.calls[0][0].tagId).toBe('tag1');
    expect(out).toContain('note-n1');
    expect(out).not.toContain('note-n2');
  });

  it('--notebook + --tag (no query): also intersects — regression test for the old silent-ignore bug', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('n1', 300), note('n2', 200)] }) // notebook source
      .mockResolvedValueOnce({ items: [note('n2', 200), note('n3', 100)] }); // tag source
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', tag: 'active' }), client);

    expect(listNotes).toHaveBeenCalledTimes(2);
    expect(out).toContain('note-n2');
    expect(out).not.toContain('note-n1');
    expect(out).not.toContain('note-n3');
  });

  it('--notebook + --tag + --query: three-way intersection', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('n1', 300), note('n2', 200), note('n4', 50)] }) // notebook
      .mockResolvedValueOnce({ items: [note('n2', 200), note('n3', 100)] }) // tag
      .mockResolvedValueOnce({ items: [note('n2', 200), note('n4', 50)] }); // query
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', tag: 'active', query: 'x' }), client);

    expect(listNotes).toHaveBeenCalledTimes(3);
    expect(out).toContain('note-n2');
    expect(out).not.toContain('note-n1');
    expect(out).not.toContain('note-n3');
    expect(out).not.toContain('note-n4');
  });

  it('--task + --notebook: type:todo is folded into the search source, then intersected', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('n1', 300, { is_todo: 1 }), note('n2', 200, { is_todo: 0 })] }) // notebook
      .mockResolvedValueOnce({ items: [note('n1', 300, { is_todo: 1 })] }); // search w/ type:todo
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', task: true }), client);

    expect(listNotes.mock.calls[1][0].query).toBe('* type:todo');
    expect(out).toContain('note-n1');
    expect(out).not.toContain('note-n2');
  });

  it('empty intersection reports a definitive zero, not an error', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('n1', 300)] })
      .mockResolvedValueOnce({ items: [note('n2', 200)] });
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', query: 'x' }), client);

    expect(out).toContain('notes: 0');
    expect(out).toContain('for query `x`, notebook `nb1`');
  });

  it('empty-state context includes --notebook even without a --query (regression)', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1' }), client);

    expect(out).toContain('notebook `nb1`');
  });

  it('results are sorted by recency and truncated to --limit', async () => {
    const listNotes = vi
      .fn()
      .mockResolvedValueOnce({ items: [note('old', 100), note('new', 300), note('mid', 200)] })
      .mockResolvedValueOnce({ items: [note('old', 100), note('new', 300), note('mid', 200)] });
    const client = fakeClient({ listNotes });

    const out = await notesCommands.list.run(args({ ...D, notebook: 'nb1', query: 'x', limit: 2 }), client);

    const idsInOrder = ['new', 'mid', 'old'].filter((id) => out.includes(`note-${id}`));
    expect(idsInOrder).toEqual(['new', 'mid']);
  });
});

import { describe, expect, it, vi } from 'vitest';
import { notebooksCommands } from '../src/commands/notebooks.js';
import { UsageError } from '../src/lib/args.js';
import { args, fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  listNotebooks: vi.fn().mockResolvedValue([]),
  createNotebook: vi.fn(),
  updateNotebook: vi.fn(),
  deleteNotebook: vi.fn(),
});

describe('notebooks list', () => {
  const nbs = [
    { id: 'root1', title: 'Root A', parent_id: '' },
    { id: 'root2', title: 'Root B', parent_id: '' },
    { id: 'child1', title: 'Child of A', parent_id: 'root1' },
  ];

  it('returns everything when --parent is omitted', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockResolvedValue(nbs) });
    const out = await notebooksCommands.list.run(args([]), client);
    expect(out).toContain('Root A');
    expect(out).toContain('Root B');
    expect(out).toContain('Child of A');
  });

  it('filters to children of --parent', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockResolvedValue(nbs) });
    const out = await notebooksCommands.list.run(args([], { parent: 'root1' }), client);
    expect(out).toContain('Child of A');
    expect(out).not.toContain('Root A');
    expect(out).not.toContain('Root B');
  });

  it('filters to top-level with an empty-string --parent', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockResolvedValue(nbs) });
    const out = await notebooksCommands.list.run(args([], { parent: '' }), client);
    expect(out).toContain('Root A');
    expect(out).toContain('Root B');
    expect(out).not.toContain('Child of A');
  });

  it('reports a definitive empty state naming the parent', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockResolvedValue(nbs) });
    const out = await notebooksCommands.list.run(args([], { parent: 'nonexistent' }), client);
    expect(out).toContain('0 notebooks found under parent `nonexistent`');
  });
});

describe('notebooks create', () => {
  it('requires <title>', async () => {
    const client = fakeClient();
    await expect(notebooksCommands.create.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('sends title/parent_id/icon and reports the new notebook id', async () => {
    const createNotebook = vi.fn().mockResolvedValue({ id: 'nb1', title: 'Side project' });
    const client = fakeClient({ createNotebook });

    const out = await notebooksCommands.create.run(args(['Side project'], { parent: 'parent1', icon: '🚀' }), client);

    expect(createNotebook).toHaveBeenCalledWith({
      title: 'Side project',
      parent_id: 'parent1',
      icon: JSON.stringify({ type: 1, emoji: '🚀', name: '' }),
    });
    expect(out).toContain('id: nb1');
  });

  it('omits icon/parent_id when not provided', async () => {
    const createNotebook = vi.fn().mockResolvedValue({ id: 'nb1', title: 'Plain' });
    const client = fakeClient({ createNotebook });

    await notebooksCommands.create.run(args(['Plain']), client);

    expect(createNotebook).toHaveBeenCalledWith({ title: 'Plain' });
  });
});

describe('notebooks update', () => {
  it('requires an id', async () => {
    const client = fakeClient();
    await expect(notebooksCommands.update.run(args([], { title: 'x' }), client)).rejects.toThrow(UsageError);
  });

  it('requires at least one field to change', async () => {
    const client = fakeClient();
    await expect(notebooksCommands.update.run(args(['nb1']), client)).rejects.toThrow(UsageError);
  });

  it('encodes --icon the same way as create', async () => {
    const updateNotebook = vi.fn().mockResolvedValue({ id: 'nb1', title: 'Renamed' });
    const client = fakeClient({ updateNotebook });

    await notebooksCommands.update.run(args(['nb1'], { icon: '📚' }), client);

    expect(updateNotebook).toHaveBeenCalledWith('nb1', { icon: JSON.stringify({ type: 1, emoji: '📚', name: '' }) });
  });

  it('includes a next-step hint, consistent with notes/tags update', async () => {
    const updateNotebook = vi.fn().mockResolvedValue({ id: 'nb1', title: 'Renamed' });
    const client = fakeClient({ updateNotebook });

    const out = await notebooksCommands.update.run(args(['nb1'], { title: 'Renamed' }), client);

    expect(out).toContain('help[');
    expect(out).toContain('joplin-axi notebooks list');
  });
});

describe('notebooks delete', () => {
  it('requires an id', async () => {
    const client = fakeClient();
    await expect(notebooksCommands.delete.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('calls deleteNotebook and reports trashed status', async () => {
    const deleteNotebook = vi.fn();
    const client = fakeClient({ deleteNotebook });

    const out = await notebooksCommands.delete.run(args(['nb1']), client);

    expect(deleteNotebook).toHaveBeenCalledWith('nb1');
    expect(out).toContain('trashed: true');
  });
});

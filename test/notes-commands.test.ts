import { describe, expect, it, vi } from 'vitest';
import { notesCommands } from '../src/commands/notes.js';
import { UsageError } from '../src/lib/args.js';
import { args, fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  getNote: vi.fn(),
  createNote: vi.fn(),
  updateNote: vi.fn(),
  deleteNote: vi.fn(),
  listNotebooks: vi.fn().mockResolvedValue([]),
  listTags: vi.fn().mockResolvedValue([]),
  listNotes: vi.fn(),
  getTagsByNote: vi.fn(),
  getNoteResources: vi.fn(),
  ping: vi.fn(),
});

describe('notes create', () => {
  it('requires --title', async () => {
    const client = fakeClient();
    await expect(notesCommands.create.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('sends title/body/parent_id and reports the new note id', async () => {
    const createNote = vi.fn().mockResolvedValue({ id: 'abc123', title: 'Hello' });
    const client = fakeClient({ createNote });

    const out = await notesCommands.create.run(args([], { title: 'Hello', body: 'World', notebook: 'nb1' }), client);

    expect(createNote).toHaveBeenCalledWith({ title: 'Hello', body: 'World', parent_id: 'nb1' });
    expect(out).toContain('id: abc123');
    expect(out).toContain('notes get abc123');
  });
});

describe('notes update', () => {
  it('requires at least one field to change', async () => {
    const client = fakeClient();
    await expect(notesCommands.update.run(args(['id1']), client)).rejects.toThrow(UsageError);
  });

  it('requires an id', async () => {
    const client = fakeClient();
    await expect(notesCommands.update.run(args([], { title: 'x' }), client)).rejects.toThrow(UsageError);
  });

  it('maps --notebook to parent_id', async () => {
    const updateNote = vi.fn().mockResolvedValue({ id: 'id1', title: 'New' });
    const client = fakeClient({ updateNote });

    await notesCommands.update.run(args(['id1'], { notebook: 'nb2' }), client);

    expect(updateNote).toHaveBeenCalledWith('id1', { parent_id: 'nb2' });
  });
});

describe('notes edit', () => {
  it('requires one of --find/--append/--prepend', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1']), client)).rejects.toThrow(UsageError);
  });

  it('rejects combining --find and --append', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1'], { find: 'a', replace: 'b', append: 'c' }), client)).rejects.toThrow(UsageError);
  });

  it('requires --replace alongside --find', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1'], { find: 'a' }), client)).rejects.toThrow(UsageError);
  });

  it('rejects --replace without --find instead of silently ignoring it', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1'], { append: 'x', replace: 'y' }), client)).rejects.toThrow(UsageError);
  });

  it('rejects --all without --find instead of silently ignoring it', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1'], { append: 'x', all: true }), client)).rejects.toThrow(UsageError);
  });

  it('rejects an empty --find instead of matching (and corrupting) the whole body', async () => {
    const client = fakeClient();
    await expect(notesCommands.edit.run(args(['id1'], { find: '', replace: 'X' }), client)).rejects.toThrow(UsageError);
  });

  it('errors if the find text is not present in the body', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'hello world' }) });
    await expect(notesCommands.edit.run(args(['id1'], { find: 'xyz', replace: 'q' }), client)).rejects.toThrow(UsageError);
  });

  it('replaces only the first occurrence by default', async () => {
    const updateNote = vi.fn();
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'foo foo foo' }),
      updateNote,
    });

    await notesCommands.edit.run(args(['id1'], { find: 'foo', replace: 'bar' }), client);

    expect(updateNote).toHaveBeenCalledWith('id1', { body: 'bar foo foo' });
  });

  it('replaces all occurrences with --all', async () => {
    const updateNote = vi.fn();
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'foo foo foo' }),
      updateNote,
    });

    await notesCommands.edit.run(args(['id1'], { find: 'foo', replace: 'bar', all: true }), client);

    expect(updateNote).toHaveBeenCalledWith('id1', { body: 'bar bar bar' });
  });

  it('treats the replacement text literally, not as a $-pattern (regression)', async () => {
    // body.replace(find, replace) would interpret "$&" as "insert the match"
    // even though find/replace are both plain strings, not regexes — this
    // must behave identically to the --all path, which never had that bug.
    const updateNote = vi.fn();
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'see TODO here' }),
      updateNote,
    });

    await notesCommands.edit.run(args(['id1'], { find: 'TODO', replace: 'note: $& done' }), client);

    expect(updateNote).toHaveBeenCalledWith('id1', { body: 'see note: $& done here' });
  });

  it('replaces only the first occurrence literally, matching --all\'s literal semantics', async () => {
    const updateNote = vi.fn();
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'a$&b a$&b' }),
      updateNote,
    });

    await notesCommands.edit.run(args(['id1'], { find: 'a$&b', replace: 'X' }), client);

    expect(updateNote).toHaveBeenCalledWith('id1', { body: 'X a$&b' });
  });

  it('appends and prepends text', async () => {
    const updateNote = vi.fn();
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'id1', body: 'middle' }),
      updateNote,
    });

    await notesCommands.edit.run(args(['id1'], { append: '!' }), client);
    expect(updateNote).toHaveBeenCalledWith('id1', { body: 'middle!' });

    await notesCommands.edit.run(args(['id1'], { prepend: '>' }), client);
    expect(updateNote).toHaveBeenCalledWith('id1', { body: '>middle' });
  });
});

describe('notes delete', () => {
  it('requires an id', async () => {
    const client = fakeClient();
    await expect(notesCommands.delete.run(args([]), client)).rejects.toThrow(UsageError);
  });

  it('calls deleteNote (soft delete) and reports trashed status', async () => {
    const deleteNote = vi.fn();
    const client = fakeClient({ deleteNote });

    const out = await notesCommands.delete.run(args(['id1']), client);

    expect(deleteNote).toHaveBeenCalledWith('id1');
    expect(deleteNote).toHaveBeenCalledTimes(1);
    expect(out).toContain('trashed: true');
  });
});

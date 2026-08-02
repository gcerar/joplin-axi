import { describe, expect, it, vi } from 'vitest';
import { notesCommands } from '../src/commands/notes.js';
import { args, fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  getNote: vi.fn().mockResolvedValue({ id: 'n1', title: 'Note', body: 'hello', parent_id: 'nb1', is_todo: 0 }),
  getNoteResources: vi.fn().mockResolvedValue([]),
  listNotebooks: vi.fn().mockResolvedValue([{ id: 'nb1', title: 'Inbox', parent_id: '' }]),
});

describe('notes get — field-efficient fetching', () => {
  it('requests only the API fields needed for the default display fields', async () => {
    const getNote = vi.fn().mockResolvedValue({ id: 'n1', title: 'Note', body: 'hi', parent_id: 'nb1', is_todo: 0 });
    const client = fakeClient({ getNote });

    await notesCommands.get.run(args(['n1']), client);

    const requestedFields = getNote.mock.calls[0][1] as string[];
    expect(requestedFields.sort()).toEqual(['body', 'created_time', 'id', 'is_todo', 'parent_id', 'title', 'updated_time'].sort());
  });

  it('requests only id/title when --fields id,title is given (not body)', async () => {
    const getNote = vi.fn().mockResolvedValue({ id: 'n1', title: 'Note' });
    const client = fakeClient({ getNote });

    await notesCommands.get.run(args(['n1'], { fields: 'id,title' }), client);

    const requestedFields = getNote.mock.calls[0][1] as string[];
    expect(requestedFields.sort()).toEqual(['id', 'title']);
    expect(requestedFields).not.toContain('body');
  });
});

describe('notes resources — field-efficient fetching and OCR truncation', () => {
  it('does not request ocr_text unless --fields asks for it', async () => {
    const getNoteResources = vi.fn().mockResolvedValue([]);
    const client = fakeClient({ getNoteResources });

    await notesCommands.resources.run(args(['n1']), client);

    const requestedFields = getNoteResources.mock.calls[0][1] as string[];
    expect(requestedFields).not.toContain('ocr_text');
  });

  it('requests ocr_text when --fields includes it', async () => {
    const getNoteResources = vi.fn().mockResolvedValue([]);
    const client = fakeClient({ getNoteResources });

    await notesCommands.resources.run(args(['n1'], { fields: 'id,title,ocr_text' }), client);

    const requestedFields = getNoteResources.mock.calls[0][1] as string[];
    expect(requestedFields).toContain('ocr_text');
  });

  it('truncates long OCR text by default and shows a --full hint', async () => {
    const longText = 'x'.repeat(600);
    const client = fakeClient({
      getNoteResources: vi.fn().mockResolvedValue([{ id: 'r1', title: 'scan.png', mime: 'image/png', size: 100, ocr_text: longText }]),
    });

    const out = await notesCommands.resources.run(args(['n1'], { fields: 'id,title,ocr_text' }), client);

    expect(out).toContain('x'.repeat(500));
    expect(out).not.toContain('x'.repeat(501));
    expect(out).toContain('notes resources n1 --full');
  });

  it('shows complete OCR text with --full and no truncation hint', async () => {
    const longText = 'x'.repeat(600);
    const client = fakeClient({
      getNoteResources: vi.fn().mockResolvedValue([{ id: 'r1', title: 'scan.png', mime: 'image/png', size: 100, ocr_text: longText }]),
    });

    const out = await notesCommands.resources.run(args(['n1'], { fields: 'id,title,ocr_text', full: true }), client);

    expect(out).toContain(longText);
    expect(out).not.toContain('--full` to see complete OCR text');
  });
});

import { describe, expect, it, vi } from 'vitest';
import { notesCommands } from '../src/commands/notes.js';
import { UsageError } from '../src/lib/args.js';
import { args, fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  getNote: vi.fn(),
});

describe('notes find-in', () => {
  it('rejects a non-positive --limit', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: 'x' }) });
    await expect(notesCommands['find-in'].run(args(['n1', 'x'], { limit: 0 }), client)).rejects.toThrow(UsageError);
  });

  it('suggests --full for full context when matches are found', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: 'line one\nTODO here\nline three' }) });

    const out = await notesCommands['find-in'].run(args(['n1', 'TODO'], { limit: 20 }), client);

    expect(out).toContain('TODO here');
    expect(out).toContain('notes get n1 --full` to see these matches in full context');
  });

  it('omits the hint when there are no matches', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: 'nothing relevant' }) });

    const out = await notesCommands['find-in'].run(args(['n1', 'TODO'], { limit: 20 }), client);

    expect(out).toContain('0 matches');
    expect(out).not.toContain('help[');
  });

  it('flags truncated context lines instead of silently cutting them with no indication', async () => {
    const longLine = 'TODO ' + 'x'.repeat(200);
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: longLine }) });

    const out = await notesCommands['find-in'].run(args(['n1', 'TODO'], { limit: 20 }), client);

    expect(out).toContain('x'.repeat(115)); // 120-char preview minus the 5-char "TODO " prefix
    expect(out).not.toContain('x'.repeat(116));
    expect(out).toContain('Some context lines were truncated');
  });
});

describe('notes links', () => {
  it('suggests viewing a linked note only when an internal link is present', async () => {
    const client = fakeClient({
      getNote: vi.fn().mockResolvedValue({ id: 'n1', body: '[Internal](:/1234567890abcdef1234567890abcdef)' }),
    });

    const out = await notesCommands.links.run(args(['n1']), client);

    expect(out).toContain('note');
    expect(out).toContain('view a linked note');
  });

  it('omits the hint when all links are external', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: '[External](https://example.com)' }) });

    const out = await notesCommands.links.run(args(['n1']), client);

    expect(out).toContain('external');
    expect(out).not.toContain('help[');
  });

  it('omits the hint when there are no links at all', async () => {
    const client = fakeClient({ getNote: vi.fn().mockResolvedValue({ id: 'n1', body: 'no links here' }) });

    const out = await notesCommands.links.run(args(['n1']), client);

    expect(out).toContain('0 links found');
    expect(out).not.toContain('help[');
  });
});

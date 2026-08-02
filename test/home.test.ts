import { describe, expect, it, vi } from 'vitest';
import { homeView } from '../src/commands/home.js';
import { fakeClientFactory } from './helpers.js';

const fakeClient = fakeClientFactory({
  ping: vi.fn().mockResolvedValue(true),
  listNotebooks: vi.fn().mockResolvedValue([]),
  listTags: vi.fn().mockResolvedValue([]),
  listNotes: vi.fn().mockResolvedValue({ items: [] }),
});

describe('homeView', () => {
  it('shows unreachable state and stops without querying further when Joplin is down', async () => {
    const listNotebooks = vi.fn();
    const client = fakeClient({ ping: vi.fn().mockResolvedValue(false), listNotebooks });

    const out = await homeView(client);

    expect(out).toContain('clipper: unreachable');
    expect(out).toContain('Web Clipper is enabled');
    expect(listNotebooks).not.toHaveBeenCalled();
  });

  it('shows notebook/tag counts and recent notes when reachable', async () => {
    const client = fakeClient({
      listNotebooks: vi.fn().mockResolvedValue([{ id: 'nb1' }, { id: 'nb2' }]),
      listTags: vi.fn().mockResolvedValue([{ id: 't1' }]),
      listNotes: vi.fn().mockResolvedValue({ items: [{ id: 'n1', title: 'Recent note', updated_time: 100 }] }),
    });

    const out = await homeView(client);

    expect(out).toContain('clipper: reachable');
    expect(out).toContain('notebooks: 2');
    expect(out).toContain('tags: 1');
    expect(out).toContain('Recent note');
  });

  it('requests only the 5 most recent notes with minimal fields', async () => {
    const listNotes = vi.fn().mockResolvedValue({ items: [] });
    const client = fakeClient({ listNotes });

    await homeView(client);

    expect(listNotes).toHaveBeenCalledWith(expect.objectContaining({ limit: 5 }));
  });

  it('reports a definitive empty state for recent notes', async () => {
    const client = fakeClient({ listNotes: vi.fn().mockResolvedValue({ items: [] }) });
    const out = await homeView(client);
    expect(out).toContain('recent_notes: 0 notes found');
  });

  it('includes a pointer to the full command reference', async () => {
    const client = fakeClient();
    const out = await homeView(client);
    expect(out).toContain('joplin-axi --help');
  });
});

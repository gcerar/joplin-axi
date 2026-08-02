import { afterEach, describe, expect, it, vi } from 'vitest';
import { JoplinApiError, JoplinClient } from '../src/client.js';

function mockResponse(body: unknown, opts: { ok?: boolean; status?: number } = {}) {
  const text = typeof body === 'string' ? body : JSON.stringify(body);
  return { ok: opts.ok ?? true, status: opts.status ?? 200, text: () => Promise.resolve(text) } as Response;
}

function client(token = 'secret-token-123') {
  return new JoplinClient({ baseUrl: 'http://localhost:41184', token });
}

describe('JoplinClient — URL construction', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('encodes an ID containing special characters into the URL path (notes, notebooks, tags)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ id: 'x', title: 'y' }));
    vi.stubGlobal('fetch', fetchMock);

    await client().getNote('abc/def&ghi');
    const url = fetchMock.mock.calls[0][0] as string;

    expect(url).not.toContain('abc/def&ghi');
    expect(url).toContain(encodeURIComponent('abc/def&ghi'));
    expect(url).toMatch(/^http:\/\/localhost:41184\/notes\/abc%2Fdef%26ghi\?/);
  });

  it('encodes both IDs in the two-ID removeTagFromNote path', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({}));
    vi.stubGlobal('fetch', fetchMock);

    await client().removeTagFromNote('tag/1', 'note&2');
    const url = fetchMock.mock.calls[0][0] as string;

    expect(url).toContain(`/tags/${encodeURIComponent('tag/1')}/notes/${encodeURIComponent('note&2')}`);
  });

  it('appends the token as an encoded query param and never leaks it into a thrown error message', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse('not found', { ok: false, status: 404 }));
    vi.stubGlobal('fetch', fetchMock);

    let caught: JoplinApiError | undefined;
    try {
      await client('super-secret-token').getNote('n1');
    } catch (e) {
      caught = e as JoplinApiError;
    }

    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toContain(`token=${encodeURIComponent('super-secret-token')}`);
    expect(caught).toBeInstanceOf(JoplinApiError);
    expect(caught?.status).toBe(404);
    expect(caught?.message).not.toContain('super-secret-token');
  });

  it('sends a JSON body with Content-Type only when a body is present', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ id: 'n1' }));
    vi.stubGlobal('fetch', fetchMock);

    await client().createNote({ title: 'hi' });
    const [, opts] = fetchMock.mock.calls[0];
    expect(opts.method).toBe('POST');
    expect(opts.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(opts.body).toBe(JSON.stringify({ title: 'hi' }));

    fetchMock.mockClear();
    await client().deleteNote('n1');
    const [, deleteOpts] = fetchMock.mock.calls[0];
    expect(deleteOpts.headers).toBeUndefined();
    expect(deleteOpts.body).toBeUndefined();
  });

  it('wraps a network failure (fetch throwing) in a JoplinApiError', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockRejectedValue(new Error('ECONNREFUSED'))
    );

    await expect(client().ping()).resolves.toBe(false);
    await expect(client().getNote('n1')).rejects.toThrow(/cannot reach Joplin/);
  });
});

describe('JoplinClient — pagination (pagedLimited via listNotebooks)', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('follows has_more across pages and stops when it goes false', async () => {
    const page1 = { items: [{ id: 'a' }, { id: 'b' }], has_more: true };
    const page2 = { items: [{ id: 'c' }], has_more: false };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(page1))
      .mockResolvedValueOnce(mockResponse(page2));
    vi.stubGlobal('fetch', fetchMock);

    const result = await client().listNotebooks();

    expect(result).toEqual([{ id: 'a' }, { id: 'b' }, { id: 'c' }]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toContain('page=1');
    expect(fetchMock.mock.calls[1][0]).toContain('page=2');
  });

  it('stops paging when a page comes back empty even if has_more claims true', async () => {
    const page1 = { items: [{ id: 'a' }], has_more: true };
    const page2 = { items: [], has_more: true };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(page1))
      .mockResolvedValueOnce(mockResponse(page2));
    vi.stubGlobal('fetch', fetchMock);

    const result = await client().listNotebooks();

    expect(result).toEqual([{ id: 'a' }]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('respects a finite limit and slices the accumulated results', async () => {
    const page1 = { items: [{ id: 'a' }, { id: 'b' }], has_more: true };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(page1));
    vi.stubGlobal('fetch', fetchMock);

    const result = await client().listNotes({ limit: 1 });

    expect(result.items).toEqual([{ id: 'a' }]);
  });
});

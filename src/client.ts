// Thin wrapper around the Joplin Web Clipper (Data API). See TODO.md for the
// write-side operations still to add (restore-from-trash has no documented
// or discoverable implementation anywhere — REST API docs, the joppy client,
// and joplin-mcp's own source all lack one — so it's intentionally omitted
// rather than guessed at).

export interface JoplinClientOptions {
  baseUrl: string;
  token: string;
}

export class JoplinApiError extends Error {
  constructor(message: string, public status?: number) {
    super(message);
    this.name = 'JoplinApiError';
  }
}

interface Page<T> {
  items?: T[];
  has_more?: boolean;
}

export interface ListNotesOptions {
  query?: string;
  notebookId?: string;
  tagId?: string;
  orderBy?: string;
  orderDir?: 'ASC' | 'DESC';
  fields?: string[];
  limit: number;
  /**
   * Include trashed/conflict notes. Per the Joplin Data API docs this is only
   * documented for the unfiltered GET /notes listing, not for /search,
   * /folders/:id/notes, or /tags/:id/notes — callers must not combine this
   * with query/notebookId/tagId (the command layer enforces that).
   */
  includeDeleted?: boolean;
}

// Joplin's documented per-page maximum — always request it to minimize round trips.
const MAX_PAGE_SIZE = 100;

const DEFAULT_LIST_FIELDS = ['id', 'title', 'parent_id', 'updated_time'];

export class JoplinClient {
  private base: string;
  private token: string;

  constructor(opts: JoplinClientOptions) {
    this.base = opts.baseUrl.replace(/\/$/, '');
    this.token = opts.token;
  }

  private async request<T>(path: string, opts: { method?: string; body?: unknown } = {}): Promise<T> {
    const sep = path.includes('?') ? '&' : '?';
    const url = `${this.base}${path}${sep}token=${encodeURIComponent(this.token)}`;
    const method = opts.method ?? 'GET';

    let res: Response;
    try {
      res = await fetch(url, {
        method,
        headers: opts.body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      });
    } catch (e) {
      throw new JoplinApiError(`cannot reach Joplin at ${this.base} (${(e as Error).message})`);
    }

    const text = await res.text();
    if (!res.ok) {
      throw new JoplinApiError(`Joplin API ${method} ${path} failed: ${res.status} ${text.slice(0, 300)}`, res.status);
    }
    return text ? (JSON.parse(text) as T) : ({} as T);
  }

  /** True if the Joplin Web Clipper service is reachable at all (no token required). */
  async ping(): Promise<boolean> {
    try {
      const res = await fetch(`${this.base}/ping`);
      const text = await res.text();
      return res.ok && text.includes('JoplinClipperServer');
    } catch {
      return false;
    }
  }

  async listNotebooks(fields = ['id', 'title', 'parent_id']): Promise<Record<string, any>[]> {
    return this.pagedLimited(`/folders?fields=${fields.join(',')}`, Infinity);
  }

  async listTags(fields = ['id', 'title']): Promise<Record<string, any>[]> {
    return this.pagedLimited(`/tags?fields=${fields.join(',')}`, Infinity);
  }

  async getTagsByNote(noteId: string, fields = ['id', 'title']): Promise<Record<string, any>[]> {
    return this.pagedLimited(`/notes/${encodeURIComponent(noteId)}/tags?fields=${fields.join(',')}`, Infinity);
  }

  async getNote(
    id: string,
    fields = ['id', 'title', 'body', 'parent_id', 'updated_time', 'created_time', 'is_todo']
  ): Promise<Record<string, any>> {
    return this.request(`/notes/${encodeURIComponent(id)}?fields=${fields.join(',')}`);
  }

  async getNoteResources(
    noteId: string,
    fields = ['id', 'title', 'mime', 'size', 'ocr_text']
  ): Promise<Record<string, any>[]> {
    return this.pagedLimited(`/notes/${encodeURIComponent(noteId)}/resources?fields=${fields.join(',')}`, Infinity);
  }

  async createNote(fields: Record<string, unknown>): Promise<Record<string, any>> {
    return this.request('/notes', { method: 'POST', body: fields });
  }

  async updateNote(id: string, fields: Record<string, unknown>): Promise<Record<string, any>> {
    return this.request(`/notes/${encodeURIComponent(id)}`, { method: 'PUT', body: fields });
  }

  /**
   * Always a soft delete (moves the note to Joplin's trash) — joplin-axi never
   * sends `permanent=1`, by design, regardless of caller input.
   */
  async deleteNote(id: string): Promise<void> {
    await this.request(`/notes/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  async createNotebook(fields: Record<string, unknown>): Promise<Record<string, any>> {
    return this.request('/folders', { method: 'POST', body: fields });
  }

  async updateNotebook(id: string, fields: Record<string, unknown>): Promise<Record<string, any>> {
    return this.request(`/folders/${encodeURIComponent(id)}`, { method: 'PUT', body: fields });
  }

  /** Always a soft delete (moves the notebook to Joplin's trash) — same policy as deleteNote. */
  async deleteNotebook(id: string): Promise<void> {
    await this.request(`/folders/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  async createTag(title: string): Promise<Record<string, any>> {
    return this.request('/tags', { method: 'POST', body: { title } });
  }

  async updateTag(id: string, title: string): Promise<Record<string, any>> {
    return this.request(`/tags/${encodeURIComponent(id)}`, { method: 'PUT', body: { title } });
  }

  /** Unlike notes/folders, Joplin documents no trash concept for tags — this is immediate. */
  async deleteTag(id: string): Promise<void> {
    await this.request(`/tags/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  async addTagToNote(tagId: string, noteId: string): Promise<void> {
    await this.request(`/tags/${encodeURIComponent(tagId)}/notes`, { method: 'POST', body: { id: noteId } });
  }

  async removeTagFromNote(tagId: string, noteId: string): Promise<void> {
    await this.request(`/tags/${encodeURIComponent(tagId)}/notes/${encodeURIComponent(noteId)}`, { method: 'DELETE' });
  }

  async listNotes(opts: ListNotesOptions): Promise<{ items: Record<string, any>[] }> {
    const fields = opts.fields ?? DEFAULT_LIST_FIELDS;
    const orderBy = opts.orderBy ?? 'updated_time';
    const orderDir = opts.orderDir ?? 'DESC';

    let path: string;
    if (opts.query) {
      // Joplin search DSL (e.g. "type:todo", "notebook:\"name\"") — see
      // https://joplinapp.org/help/apps/search/#search-filters. Notebook/tag
      // scoping uses the dedicated endpoints below instead, which take IDs
      // directly rather than requiring a title lookup.
      path = `/search?query=${encodeURIComponent(opts.query)}&fields=${fields.join(',')}`;
    } else if (opts.notebookId) {
      path = `/folders/${encodeURIComponent(opts.notebookId)}/notes?fields=${fields.join(',')}&order_by=${orderBy}&order_dir=${orderDir}`;
    } else if (opts.tagId) {
      path = `/tags/${encodeURIComponent(opts.tagId)}/notes?fields=${fields.join(',')}&order_by=${orderBy}&order_dir=${orderDir}`;
    } else {
      path = `/notes?fields=${fields.join(',')}&order_by=${orderBy}&order_dir=${orderDir}`;
      if (opts.includeDeleted) path += '&include_deleted=1';
    }

    const items = await this.pagedLimited(path, opts.limit);
    return { items };
  }

  private async pagedLimited(path: string, limit: number): Promise<Record<string, any>[]> {
    const all: Record<string, any>[] = [];
    let page = 1;

    while (all.length < limit) {
      const sep = path.includes('?') ? '&' : '?';
      const data = await this.request<Page<Record<string, any>> | Record<string, any>[]>(
        `${path}${sep}page=${page}&limit=${MAX_PAGE_SIZE}`
      );
      const items = Array.isArray(data) ? data : data.items ?? [];
      all.push(...items);

      const hasMore = Array.isArray(data) ? false : Boolean(data.has_more);
      if (!hasMore || items.length === 0) break;
      page++;
    }

    return limit === Infinity ? all : all.slice(0, limit);
  }
}

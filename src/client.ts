// Thin wrapper around the Joplin Web Clipper (Data API).

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

    // A FormData body (resource upload) must be passed through as-is — fetch
    // sets its own multipart boundary in Content-Type, and JSON.stringify-ing
    // it would silently send `{}` instead of the file.
    const isFormData = opts.body instanceof FormData;

    let res: Response;
    try {
      res = await fetch(url, {
        method,
        headers: opts.body !== undefined && !isFormData ? { 'Content-Type': 'application/json' } : undefined,
        body: opts.body === undefined ? undefined : isFormData ? (opts.body as FormData) : JSON.stringify(opts.body),
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

  /**
   * Restores a soft-deleted note by clearing `deleted_time`. Undocumented in
   * the REST API reference (deleted_time isn't listed among PUT-modifiable
   * fields), but confirmed working — this is exactly what joplin-mcp's own
   * `restore_from_trash` tool does under the hood (`tools/trash.py`, via the
   * joppy client's generic `modify_note(id, deleted_time=0)`).
   */
  async restoreNote(id: string): Promise<void> {
    await this.request(`/notes/${encodeURIComponent(id)}`, { method: 'PUT', body: { deleted_time: 0 } });
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

  /**
   * Restores a soft-deleted notebook, same mechanism as restoreNote. Only
   * clears deleted_time on this one notebook — per joplin-mcp's own docs,
   * Joplin sets deleted_time on every descendant when a notebook is trashed,
   * and restoring the parent does not clear it on sub-notebooks/notes.
   */
  async restoreNotebook(id: string): Promise<void> {
    await this.request(`/folders/${encodeURIComponent(id)}`, { method: 'PUT', body: { deleted_time: 0 } });
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

  /**
   * Uploads a file as a new Joplin resource. Per the REST API reference this is
   * the one endpoint that requires `multipart/form-data`: the file goes in a
   * `data` field, metadata in a `props` field. Joplin always assigns its own
   * resource ID — there's no way to request a specific one — so callers that
   * need to rewrite `:/oldId` references in note bodies must track the
   * returned ID themselves (see src/lib/import/importer.ts).
   */
  async createResource(data: Buffer, filename: string, mime?: string): Promise<Record<string, any>> {
    const form = new FormData();
    form.append('data', new Blob([data], mime ? { type: mime } : undefined), filename);
    form.append('props', JSON.stringify({ title: filename }));
    return this.request('/resources', { method: 'POST', body: form });
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

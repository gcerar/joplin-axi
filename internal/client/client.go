// Package client is a thin wrapper around the Joplin Web Clipper (Data API).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// Options configures a new HTTPClient.
type Options struct {
	BaseURL string
	Token   string
}

// APIError is returned for any non-2xx Joplin API response, or when the
// server can't be reached at all (Status is 0 in that case).
type APIError struct {
	Message string
	Status  int
}

func (e *APIError) Error() string { return e.Message }

// Unlimited tells pagedLimited (and the list methods that don't take an
// explicit limit) to fetch every page rather than stopping at a count —
// Go has no integer Infinity, so this is an explicit sentinel instead.
const Unlimited = -1

// Joplin's documented per-page maximum — always requested, to minimize round trips.
const maxPageSize = 100

var (
	DefaultNotebookFields = []string{"id", "title", "parent_id"}
	DefaultTagFields      = []string{"id", "title"}
	DefaultNoteFields     = []string{"id", "title", "body", "parent_id", "updated_time", "created_time", "is_todo"}
	DefaultResourceFields = []string{"id", "title", "mime", "size", "ocr_text"}
	DefaultListFields     = []string{"id", "title", "parent_id", "updated_time"}
)

// ListNotesOptions mirrors the TS ListNotesOptions shape. Limit uses the
// Unlimited sentinel for "no limit" rather than a pointer or math.MaxInt.
type ListNotesOptions struct {
	Query      string
	NotebookID string
	TagID      string
	OrderBy    string
	OrderDir   string // "ASC" or "DESC"; defaults to "DESC"
	Fields     []string
	Limit      int
	// IncludeDeleted is only documented by Joplin for the unfiltered /notes
	// listing (not /search, /folders/:id/notes, or /tags/:id/notes) — callers
	// must not combine this with Query/NotebookID/TagID (the command layer
	// enforces that, same as the TS version).
	IncludeDeleted bool
}

// Client is the interface every command depends on — never *HTTPClient
// directly — so tests can substitute a stub.
type Client interface {
	Ping(ctx context.Context) bool
	ListNotebooks(ctx context.Context, fields []string) ([]map[string]any, error)
	ListTags(ctx context.Context, fields []string) ([]map[string]any, error)
	GetTagsByNote(ctx context.Context, noteID string, fields []string) ([]map[string]any, error)
	GetNote(ctx context.Context, id string, fields []string) (map[string]any, error)
	GetNoteResources(ctx context.Context, noteID string, fields []string) ([]map[string]any, error)
	CreateNote(ctx context.Context, fields map[string]any) (map[string]any, error)
	UpdateNote(ctx context.Context, id string, fields map[string]any) (map[string]any, error)
	DeleteNote(ctx context.Context, id string) error
	RestoreNote(ctx context.Context, id string) error
	CreateNotebook(ctx context.Context, fields map[string]any) (map[string]any, error)
	UpdateNotebook(ctx context.Context, id string, fields map[string]any) (map[string]any, error)
	DeleteNotebook(ctx context.Context, id string) error
	RestoreNotebook(ctx context.Context, id string) error
	CreateTag(ctx context.Context, title string) (map[string]any, error)
	UpdateTag(ctx context.Context, id, title string) (map[string]any, error)
	DeleteTag(ctx context.Context, id string) error
	AddTagToNote(ctx context.Context, tagID, noteID string) error
	RemoveTagFromNote(ctx context.Context, tagID, noteID string) error
	CreateResource(ctx context.Context, data []byte, filename, mimeType string) (map[string]any, error)
	ListNotes(ctx context.Context, opts ListNotesOptions) ([]map[string]any, error)
}

// HTTPClient is the real implementation — talks to a local Joplin Web
// Clipper server over HTTP.
type HTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// New constructs a production HTTPClient using the default transport.
func New(opts Options) *HTTPClient {
	return NewWithTransport(opts, http.DefaultTransport)
}

// NewWithTransport is New with an injectable http.RoundTripper — the seam
// tests use to stub responses without a real network call.
func NewWithTransport(opts Options, transport http.RoundTripper) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimSuffix(opts.BaseURL, "/"),
		token:   opts.Token,
		http:    &http.Client{Transport: transport},
	}
}

var _ Client = (*HTTPClient)(nil)

// doRequest is the lowest-level HTTP call: builds the token-bearing URL,
// sends the request, and returns the raw response body (or an *APIError for
// a non-2xx status or an unreachable server). Every higher-level method goes
// through this so the token-in-URL and error-message construction stay in
// exactly one place.
func (c *HTTPClient) doRequest(ctx context.Context, path, method string, bodyReader io.Reader, contentType string) ([]byte, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	fullURL := c.baseURL + path + sep + "token=" + url.QueryEscape(c.token)

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := c.http.Do(req)
	if err != nil {
		// http.Client wraps transport errors in a *url.Error, whose Error()
		// string embeds the full request URL — including the token query
		// param. Unwrap to the underlying cause so the token never ends up
		// in a message that gets printed to the user.
		cause := err
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Err != nil {
			cause = urlErr.Err
		}
		return nil, &APIError{Message: fmt.Sprintf("cannot reach Joplin at %s (%s)", c.baseURL, cause.Error())}
	}
	defer res.Body.Close()

	text, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		truncated := text
		if len(truncated) > 300 {
			truncated = truncated[:300]
		}
		return nil, &APIError{
			Message: fmt.Sprintf("Joplin API %s %s failed: %d %s", method, path, res.StatusCode, truncated),
			Status:  res.StatusCode,
		}
	}
	return text, nil
}

// requestJSON marshals body (if non-nil) as a JSON request and unmarshals
// the response into out (if out is non-nil and the response is non-empty).
func (c *HTTPClient) requestJSON(ctx context.Context, path, method string, body any, out any) error {
	var bodyReader io.Reader
	contentType := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
		contentType = "application/json"
	}

	text, err := c.doRequest(ctx, path, method, bodyReader, contentType)
	if err != nil {
		return err
	}
	if out == nil || len(text) == 0 {
		return nil
	}
	return json.Unmarshal(text, out)
}

// Ping reports whether the Joplin Web Clipper service is reachable at all —
// no token required, and (matching the TS version) any failure is swallowed
// and reported as simply unreachable rather than as an error.
func (c *HTTPClient) Ping(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ping", nil)
	if err != nil {
		return false
	}
	res, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	text, err := io.ReadAll(res.Body)
	if err != nil {
		return false
	}
	return res.StatusCode >= 200 && res.StatusCode < 300 && strings.Contains(string(text), "JoplinClipperServer")
}

func (c *HTTPClient) ListNotebooks(ctx context.Context, fields []string) ([]map[string]any, error) {
	if fields == nil {
		fields = DefaultNotebookFields
	}
	return c.pagedLimited(ctx, "/folders?fields="+strings.Join(fields, ","), Unlimited)
}

func (c *HTTPClient) ListTags(ctx context.Context, fields []string) ([]map[string]any, error) {
	if fields == nil {
		fields = DefaultTagFields
	}
	return c.pagedLimited(ctx, "/tags?fields="+strings.Join(fields, ","), Unlimited)
}

func (c *HTTPClient) GetTagsByNote(ctx context.Context, noteID string, fields []string) ([]map[string]any, error) {
	if fields == nil {
		fields = DefaultTagFields
	}
	path := fmt.Sprintf("/notes/%s/tags?fields=%s", url.QueryEscape(noteID), strings.Join(fields, ","))
	return c.pagedLimited(ctx, path, Unlimited)
}

func (c *HTTPClient) GetNote(ctx context.Context, id string, fields []string) (map[string]any, error) {
	if fields == nil {
		fields = DefaultNoteFields
	}
	var result map[string]any
	path := fmt.Sprintf("/notes/%s?fields=%s", url.QueryEscape(id), strings.Join(fields, ","))
	err := c.requestJSON(ctx, path, http.MethodGet, nil, &result)
	return result, err
}

func (c *HTTPClient) GetNoteResources(ctx context.Context, noteID string, fields []string) ([]map[string]any, error) {
	if fields == nil {
		fields = DefaultResourceFields
	}
	path := fmt.Sprintf("/notes/%s/resources?fields=%s", url.QueryEscape(noteID), strings.Join(fields, ","))
	return c.pagedLimited(ctx, path, Unlimited)
}

func (c *HTTPClient) CreateNote(ctx context.Context, fields map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/notes", http.MethodPost, fields, &result)
	return result, err
}

func (c *HTTPClient) UpdateNote(ctx context.Context, id string, fields map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/notes/"+url.QueryEscape(id), http.MethodPut, fields, &result)
	return result, err
}

// DeleteNote is always a soft delete (moves the note to Joplin's trash) —
// this client never sends permanent=1, by design, regardless of caller input.
func (c *HTTPClient) DeleteNote(ctx context.Context, id string) error {
	return c.requestJSON(ctx, "/notes/"+url.QueryEscape(id), http.MethodDelete, nil, nil)
}

// RestoreNote restores a soft-deleted note by clearing deleted_time.
// Undocumented in the REST API reference but confirmed working — this is
// exactly what joplin-mcp's own restore_from_trash tool does under the hood.
func (c *HTTPClient) RestoreNote(ctx context.Context, id string) error {
	return c.requestJSON(ctx, "/notes/"+url.QueryEscape(id), http.MethodPut, map[string]any{"deleted_time": 0}, nil)
}

func (c *HTTPClient) CreateNotebook(ctx context.Context, fields map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/folders", http.MethodPost, fields, &result)
	return result, err
}

func (c *HTTPClient) UpdateNotebook(ctx context.Context, id string, fields map[string]any) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/folders/"+url.QueryEscape(id), http.MethodPut, fields, &result)
	return result, err
}

// DeleteNotebook is always a soft delete — same policy as DeleteNote.
func (c *HTTPClient) DeleteNotebook(ctx context.Context, id string) error {
	return c.requestJSON(ctx, "/folders/"+url.QueryEscape(id), http.MethodDelete, nil, nil)
}

// RestoreNotebook restores a soft-deleted notebook, same mechanism as
// RestoreNote. Only clears deleted_time on this one notebook — Joplin sets
// deleted_time on every descendant when a notebook is trashed, and restoring
// the parent does not clear it on sub-notebooks/notes.
func (c *HTTPClient) RestoreNotebook(ctx context.Context, id string) error {
	return c.requestJSON(ctx, "/folders/"+url.QueryEscape(id), http.MethodPut, map[string]any{"deleted_time": 0}, nil)
}

func (c *HTTPClient) CreateTag(ctx context.Context, title string) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/tags", http.MethodPost, map[string]any{"title": title}, &result)
	return result, err
}

func (c *HTTPClient) UpdateTag(ctx context.Context, id, title string) (map[string]any, error) {
	var result map[string]any
	err := c.requestJSON(ctx, "/tags/"+url.QueryEscape(id), http.MethodPut, map[string]any{"title": title}, &result)
	return result, err
}

// DeleteTag is immediate — unlike notes/folders, Joplin documents no trash
// concept for tags.
func (c *HTTPClient) DeleteTag(ctx context.Context, id string) error {
	return c.requestJSON(ctx, "/tags/"+url.QueryEscape(id), http.MethodDelete, nil, nil)
}

func (c *HTTPClient) AddTagToNote(ctx context.Context, tagID, noteID string) error {
	path := "/tags/" + url.QueryEscape(tagID) + "/notes"
	return c.requestJSON(ctx, path, http.MethodPost, map[string]any{"id": noteID}, nil)
}

func (c *HTTPClient) RemoveTagFromNote(ctx context.Context, tagID, noteID string) error {
	path := "/tags/" + url.QueryEscape(tagID) + "/notes/" + url.QueryEscape(noteID)
	return c.requestJSON(ctx, path, http.MethodDelete, nil, nil)
}

// CreateResource uploads a file as a new Joplin resource. Per the REST API
// reference this is the one endpoint requiring multipart/form-data: the file
// goes in a "data" field, metadata in a "props" field. Unlike JS's fetch
// (which sets its own multipart Content-Type automatically for a FormData
// body, and must NOT be given one explicitly), Go's http.Client does no such
// inspection — the boundary-bearing Content-Type from multipart.Writer must
// be set explicitly here, a necessary difference from the TS version's
// "headers must be undefined" contract, not an inconsistency.
func (c *HTTPClient) CreateResource(ctx context.Context, data []byte, filename, mimeType string) (map[string]any, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="data"; filename="%s"`, filename))
	if mimeType != "" {
		partHeader.Set("Content-Type", mimeType)
	}
	part, err := w.CreatePart(partHeader)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}

	props, err := json.Marshal(map[string]string{"title": filename})
	if err != nil {
		return nil, err
	}
	if err := w.WriteField("props", string(props)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	text, err := c.doRequest(ctx, "/resources", http.MethodPost, buf, w.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if len(text) > 0 {
		if err := json.Unmarshal(text, &result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *HTTPClient) ListNotes(ctx context.Context, opts ListNotesOptions) ([]map[string]any, error) {
	fields := opts.Fields
	if fields == nil {
		fields = DefaultListFields
	}
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "updated_time"
	}
	orderDir := opts.OrderDir
	if orderDir == "" {
		orderDir = "DESC"
	}

	var path string
	switch {
	case opts.Query != "":
		// Joplin search DSL (e.g. "type:todo") — see
		// https://joplinapp.org/help/apps/search/#search-filters. Notebook/tag
		// scoping uses the dedicated endpoints below instead, which take IDs
		// directly rather than requiring a title lookup.
		path = fmt.Sprintf("/search?query=%s&fields=%s", url.QueryEscape(opts.Query), strings.Join(fields, ","))
	case opts.NotebookID != "":
		path = fmt.Sprintf("/folders/%s/notes?fields=%s&order_by=%s&order_dir=%s",
			url.QueryEscape(opts.NotebookID), strings.Join(fields, ","), orderBy, orderDir)
	case opts.TagID != "":
		path = fmt.Sprintf("/tags/%s/notes?fields=%s&order_by=%s&order_dir=%s",
			url.QueryEscape(opts.TagID), strings.Join(fields, ","), orderBy, orderDir)
	default:
		path = fmt.Sprintf("/notes?fields=%s&order_by=%s&order_dir=%s", strings.Join(fields, ","), orderBy, orderDir)
		if opts.IncludeDeleted {
			path += "&include_deleted=1"
		}
	}

	return c.pagedLimited(ctx, path, opts.Limit)
}

// pagedLimited follows Joplin's page/has_more cursor pagination, requesting
// the documented per-page maximum each time to minimize round trips, until
// either has_more is false, a page comes back empty, or limit is reached.
func (c *HTTPClient) pagedLimited(ctx context.Context, path string, limit int) ([]map[string]any, error) {
	var all []map[string]any
	page := 1

	for limit == Unlimited || len(all) < limit {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		pagedPath := fmt.Sprintf("%s%spage=%d&limit=%d", path, sep, page, maxPageSize)

		var data any
		if err := c.requestJSON(ctx, pagedPath, http.MethodGet, nil, &data); err != nil {
			return nil, err
		}

		items, hasMore := extractPage(data)
		all = append(all, items...)
		if !hasMore || len(items) == 0 {
			break
		}
		page++
	}

	if limit != Unlimited && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// extractPage handles the two shapes a Joplin list endpoint can return: a
// bare JSON array (no pagination wrapper — treated as the final/only page),
// or {items, has_more}.
func extractPage(data any) (items []map[string]any, hasMore bool) {
	switch v := data.(type) {
	case []any:
		return toMapSlice(v), false
	case map[string]any:
		raw, _ := v["items"].([]any)
		hasMore, _ = v["has_more"].(bool)
		return toMapSlice(raw), hasMore
	default:
		return nil, false
	}
}

func toMapSlice(raw []any) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func note(id string, updated int64, extra map[string]any) map[string]any {
	n := map[string]any{"id": id, "title": "note-" + id, "updated_time": float64(updated)}
	for k, v := range extra {
		n[k] = v
	}
	return n
}

func ids(notes []map[string]any) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i], _ = n["id"].(string)
	}
	return out
}

func contains(haystack []string, want string) bool {
	for _, h := range haystack {
		if h == want {
			return true
		}
	}
	return false
}

// dispatchListNotes routes a stub ListNotes call to the right canned result
// by inspecting which filter it was called with — necessary (rather than
// vitest's sequential mockResolvedValueOnce) because ResolveNoteScope fetches
// concurrently, so call order across goroutines isn't guaranteed.
func dispatchListNotes(byNotebook, byTag, byQuery []map[string]any) func(client.ListNotesOptions) ([]map[string]any, error) {
	return func(opts client.ListNotesOptions) ([]map[string]any, error) {
		switch {
		case opts.NotebookID != "":
			return byNotebook, nil
		case opts.TagID != "":
			return byTag, nil
		case opts.Query != "":
			return byQuery, nil
		default:
			return nil, nil
		}
	}
}

func TestResolveNoteScope(t *testing.T) {
	ctx := context.Background()

	t.Run("zero filters uses the plain unfiltered listing", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: func(opts client.ListNotesOptions) ([]map[string]any, error) {
				if opts.NotebookID != "" || opts.TagID != "" || opts.Query != "" {
					t.Errorf("expected an unfiltered call, got %+v", opts)
				}
				return []map[string]any{note("n1", 100, nil)}, nil
			},
		}
		got, err := ResolveNoteScope(ctx, stub, Options{Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0]["id"] != "n1" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("a single filter is the fast path — no intersection needed", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: func(opts client.ListNotesOptions) ([]map[string]any, error) {
				if opts.NotebookID != "nb1" {
					t.Errorf("expected notebookId=nb1, got %+v", opts)
				}
				return []map[string]any{note("n1", 100, nil)}, nil
			},
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0]["id"] != "n1" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("--query + --notebook: only notes in both sets survive", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				[]map[string]any{note("n1", 300, nil), note("n2", 200, nil)},
				nil,
				[]map[string]any{note("n2", 200, nil), note("n3", 100, nil)},
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", Query: "x", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !contains(gotIDs, "n2") {
			t.Errorf("got %v, want only n2", gotIDs)
		}
	})

	t.Run("--query + --tag: intersects by ID the same way", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				nil,
				[]map[string]any{note("n1", 300, nil)},
				[]map[string]any{note("n1", 300, nil), note("n2", 200, nil)},
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{TagID: "tag1", Query: "x", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !contains(gotIDs, "n1") {
			t.Errorf("got %v, want only n1", gotIDs)
		}
	})

	t.Run("--notebook + --tag (no query): also intersects", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				[]map[string]any{note("n1", 300, nil), note("n2", 200, nil)},
				[]map[string]any{note("n2", 200, nil), note("n3", 100, nil)},
				nil,
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", TagID: "tag1", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !contains(gotIDs, "n2") {
			t.Errorf("got %v, want only n2", gotIDs)
		}
	})

	t.Run("--notebook + --tag + --query: three-way intersection", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				[]map[string]any{note("n1", 300, nil), note("n2", 200, nil), note("n4", 50, nil)},
				[]map[string]any{note("n2", 200, nil), note("n3", 100, nil)},
				[]map[string]any{note("n2", 200, nil), note("n4", 50, nil)},
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", TagID: "tag1", Query: "x", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !contains(gotIDs, "n2") {
			t.Errorf("got %v, want only n2", gotIDs)
		}
	})

	t.Run("--task folds type:todo into the search source, then intersects", func(t *testing.T) {
		var capturedQuery string
		stub := &clienttest.StubClient{
			ListNotesFunc: func(opts client.ListNotesOptions) ([]map[string]any, error) {
				if opts.NotebookID != "" {
					return []map[string]any{note("n1", 300, nil), note("n2", 200, nil)}, nil
				}
				capturedQuery = opts.Query
				return []map[string]any{note("n1", 300, nil)}, nil
			},
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", Task: true, Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedQuery != "* type:todo" {
			t.Errorf("query = %q, want '* type:todo'", capturedQuery)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !contains(gotIDs, "n1") {
			t.Errorf("got %v, want only n1", gotIDs)
		}
	})

	t.Run("empty intersection reports zero results, not an error", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				[]map[string]any{note("n1", 300, nil)},
				nil,
				[]map[string]any{note("n2", 200, nil)},
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", Query: "x", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("results are sorted by recency", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: dispatchListNotes(
				[]map[string]any{note("old", 100, nil), note("new", 300, nil), note("mid", 200, nil)},
				nil,
				[]map[string]any{note("old", 100, nil), note("new", 300, nil), note("mid", 200, nil)},
			),
		}
		got, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", Query: "x", Fields: []string{"id"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotIDs := ids(got)
		want := []string{"new", "mid", "old"}
		if len(gotIDs) != len(want) {
			t.Fatalf("got %v, want %v", gotIDs, want)
		}
		for i := range want {
			if gotIDs[i] != want[i] {
				t.Errorf("got %v, want %v", gotIDs, want)
			}
		}
	})

	t.Run("propagates an error from any source", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListNotesFunc: func(opts client.ListNotesOptions) ([]map[string]any, error) {
				if opts.TagID != "" {
					return nil, errors.New("boom")
				}
				return []map[string]any{note("n1", 100, nil)}, nil
			},
		}
		_, err := ResolveNoteScope(ctx, stub, Options{NotebookID: "nb1", TagID: "tag1", Fields: []string{"id"}})
		if err == nil || err.Error() != "boom" {
			t.Errorf("got error %v, want boom", err)
		}
	})
}

func TestResolveTagID(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves a matching tag title to its ID", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListTagsFunc: func([]string) ([]map[string]any, error) {
				return []map[string]any{{"id": "tag1", "title": "active"}, {"id": "tag2", "title": "archived"}}, nil
			},
		}
		got, err := ResolveTagID(ctx, stub, "active")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "tag1" {
			t.Errorf("got %q, want tag1", got)
		}
	})

	t.Run("errors with a helpful hint when no tag matches", func(t *testing.T) {
		stub := &clienttest.StubClient{
			ListTagsFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		}
		_, err := ResolveTagID(ctx, stub, "nonexistent")
		var usageErr *args.UsageError
		if !errors.As(err, &usageErr) {
			t.Fatalf("got error %v (%T), want *args.UsageError", err, err)
		}
		if usageErr.Message != "no tag titled `nonexistent`" {
			t.Errorf("message = %q", usageErr.Message)
		}
	})
}

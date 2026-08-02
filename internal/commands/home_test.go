package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func fakeHomeClient() *clienttest.StubClient {
	return &clienttest.StubClient{
		PingFunc:          func() bool { return true },
		ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		ListTagsFunc:      func([]string) ([]map[string]any, error) { return nil, nil },
		ListNotesFunc:     func(client.ListNotesOptions) ([]map[string]any, error) { return nil, nil },
	}
}

func TestHomeView(t *testing.T) {
	ctx := context.Background()

	t.Run("shows unreachable state and stops without querying further when Joplin is down", func(t *testing.T) {
		stub := fakeHomeClient()
		stub.PingFunc = func() bool { return false }

		out, err := HomeView(ctx, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "clipper: unreachable") {
			t.Errorf("output %q does not contain 'clipper: unreachable'", out)
		}
		if !strings.Contains(out, "Web Clipper is enabled") {
			t.Errorf("output %q does not contain the help hint", out)
		}
		if len(stub.ListNotebooksCalls) != 0 {
			t.Error("ListNotebooks was called despite Joplin being unreachable")
		}
	})

	t.Run("shows notebook/tag counts and recent notes when reachable", func(t *testing.T) {
		stub := fakeHomeClient()
		stub.ListNotebooksFunc = func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "nb1"}, {"id": "nb2"}}, nil
		}
		stub.ListTagsFunc = func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "t1"}}, nil
		}
		stub.ListNotesFunc = func(client.ListNotesOptions) ([]map[string]any, error) {
			return []map[string]any{{"id": "n1", "title": "Recent note", "updated_time": float64(100)}}, nil
		}

		out, err := HomeView(ctx, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"clipper: reachable", "notebooks: 2", "tags: 1", "Recent note"} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q does not contain %q", out, want)
			}
		}
	})

	t.Run("requests only the 5 most recent notes with minimal fields", func(t *testing.T) {
		stub := fakeHomeClient()
		var gotOpts client.ListNotesOptions
		stub.ListNotesFunc = func(opts client.ListNotesOptions) ([]map[string]any, error) {
			gotOpts = opts
			return nil, nil
		}

		if _, err := HomeView(ctx, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotOpts.Limit != 5 {
			t.Errorf("limit = %d, want 5", gotOpts.Limit)
		}
	})

	t.Run("reports a definitive empty state for recent notes", func(t *testing.T) {
		stub := fakeHomeClient()
		out, err := HomeView(ctx, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "recent_notes: 0 notes found") {
			t.Errorf("output %q does not contain the empty-state message", out)
		}
	})

	t.Run("includes a pointer to the full command reference", func(t *testing.T) {
		stub := fakeHomeClient()
		out, err := HomeView(ctx, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "joplin-axi --help") {
			t.Errorf("output %q does not contain the --help pointer", out)
		}
	})
}

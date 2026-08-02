package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func fakeNotebooksClient() *clienttest.StubClient {
	return &clienttest.StubClient{
		ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		CreateNotebookFunc: func(map[string]any) (map[string]any, error) {
			return map[string]any{}, nil
		},
		UpdateNotebookFunc: func(string, map[string]any) (map[string]any, error) {
			return map[string]any{}, nil
		},
		DeleteNotebookFunc:  func(string) error { return nil },
		RestoreNotebookFunc: func(string) error { return nil },
	}
}

func mustUsageError(t *testing.T, err error) {
	t.Helper()
	var usageErr *args.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("got error %v (%T), want *args.UsageError", err, err)
	}
}

func TestNotebooksList(t *testing.T) {
	ctx := context.Background()
	nbs := []map[string]any{
		{"id": "root1", "title": "Root A", "parent_id": ""},
		{"id": "root2", "title": "Root B", "parent_id": ""},
		{"id": "child1", "title": "Child of A", "parent_id": "root1"},
	}

	t.Run("returns everything when --parent is omitted", func(t *testing.T) {
		stub := fakeNotebooksClient()
		stub.ListNotebooksFunc = func([]string) ([]map[string]any, error) { return nbs, nil }

		result, err := NotebooksCommands["list"].Run(ctx, args.ParsedArgs{}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"Root A", "Root B", "Child of A"} {
			if !strings.Contains(result.Output, want) {
				t.Errorf("output does not contain %q:\n%s", want, result.Output)
			}
		}
	})

	t.Run("filters to children of --parent", func(t *testing.T) {
		stub := fakeNotebooksClient()
		stub.ListNotebooksFunc = func([]string) ([]map[string]any, error) { return nbs, nil }

		result, err := NotebooksCommands["list"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"parent": "root1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "Child of A") {
			t.Errorf("output does not contain 'Child of A':\n%s", result.Output)
		}
		if strings.Contains(result.Output, "Root A") || strings.Contains(result.Output, "Root B") {
			t.Errorf("output should not contain top-level notebooks:\n%s", result.Output)
		}
	})

	t.Run("filters to top-level with an empty-string --parent", func(t *testing.T) {
		stub := fakeNotebooksClient()
		stub.ListNotebooksFunc = func([]string) ([]map[string]any, error) { return nbs, nil }

		result, err := NotebooksCommands["list"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"parent": ""}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "Root A") || !strings.Contains(result.Output, "Root B") {
			t.Errorf("output does not contain both top-level notebooks:\n%s", result.Output)
		}
		if strings.Contains(result.Output, "Child of A") {
			t.Errorf("output should not contain the child notebook:\n%s", result.Output)
		}
	})

	t.Run("reports a definitive empty state naming the parent", func(t *testing.T) {
		stub := fakeNotebooksClient()
		stub.ListNotebooksFunc = func([]string) ([]map[string]any, error) { return nbs, nil }

		result, err := NotebooksCommands["list"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"parent": "nonexistent"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "0 notebooks found under parent `nonexistent`") {
			t.Errorf("output does not contain the expected empty-state message:\n%s", result.Output)
		}
	})
}

func TestNotebooksCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires <title>", func(t *testing.T) {
		stub := fakeNotebooksClient()
		_, err := NotebooksCommands["create"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("sends title/parent_id/icon and reports the new notebook id", func(t *testing.T) {
		stub := fakeNotebooksClient()
		var captured map[string]any
		stub.CreateNotebookFunc = func(fields map[string]any) (map[string]any, error) {
			captured = fields
			return map[string]any{"id": "nb1", "title": "Side project"}, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"Side project"}, Flags: map[string]any{"parent": "parent1", "icon": "🚀"}}
		result, err := NotebooksCommands["create"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantIcon, _ := encodeIcon("🚀")
		if captured["title"] != "Side project" || captured["parent_id"] != "parent1" || captured["icon"] != wantIcon {
			t.Errorf("createNotebook called with %v", captured)
		}
		if !strings.Contains(result.Output, "id: nb1") {
			t.Errorf("output does not contain 'id: nb1':\n%s", result.Output)
		}
	})

	t.Run("omits icon/parent_id when not provided", func(t *testing.T) {
		stub := fakeNotebooksClient()
		var captured map[string]any
		stub.CreateNotebookFunc = func(fields map[string]any) (map[string]any, error) {
			captured = fields
			return map[string]any{"id": "nb1", "title": "Plain"}, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"Plain"}}
		if _, err := NotebooksCommands["create"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(captured) != 1 || captured["title"] != "Plain" {
			t.Errorf("createNotebook called with %v, want only {title: Plain}", captured)
		}
	})
}

func TestNotebooksUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotebooksClient()
		_, err := NotebooksCommands["update"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"title": "x"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("requires at least one field to change", func(t *testing.T) {
		stub := fakeNotebooksClient()
		_, err := NotebooksCommands["update"].Run(ctx, args.ParsedArgs{Positionals: []string{"nb1"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("encodes --icon the same way as create", func(t *testing.T) {
		stub := fakeNotebooksClient()
		var captured map[string]any
		stub.UpdateNotebookFunc = func(id string, fields map[string]any) (map[string]any, error) {
			captured = fields
			return map[string]any{"id": "nb1", "title": "Renamed"}, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"nb1"}, Flags: map[string]any{"icon": "📚"}}
		if _, err := NotebooksCommands["update"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantIcon, _ := encodeIcon("📚")
		if len(captured) != 1 || captured["icon"] != wantIcon {
			t.Errorf("updateNotebook called with %v, want only {icon: %s}", captured, wantIcon)
		}
	})

	t.Run("includes a next-step hint, consistent with notes/tags update", func(t *testing.T) {
		stub := fakeNotebooksClient()
		stub.UpdateNotebookFunc = func(string, map[string]any) (map[string]any, error) {
			return map[string]any{"id": "nb1", "title": "Renamed"}, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"nb1"}, Flags: map[string]any{"title": "Renamed"}}
		result, err := NotebooksCommands["update"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "help[") {
			t.Errorf("output does not contain a help block:\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "joplin-axi notebooks list") {
			t.Errorf("output does not point at `joplin-axi notebooks list`:\n%s", result.Output)
		}
	})
}

func TestNotebooksDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotebooksClient()
		_, err := NotebooksCommands["delete"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("calls deleteNotebook and reports trashed status", func(t *testing.T) {
		stub := fakeNotebooksClient()
		var calledID string
		stub.DeleteNotebookFunc = func(id string) error {
			calledID = id
			return nil
		}

		result, err := NotebooksCommands["delete"].Run(ctx, args.ParsedArgs{Positionals: []string{"nb1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calledID != "nb1" {
			t.Errorf("deleteNotebook called with %q, want nb1", calledID)
		}
		if !strings.Contains(result.Output, "trashed: true") {
			t.Errorf("output does not contain 'trashed: true':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "notebooks restore nb1") {
			t.Errorf("output does not point at `notebooks restore nb1`:\n%s", result.Output)
		}
	})
}

func TestNotebooksRestore(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotebooksClient()
		_, err := NotebooksCommands["restore"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("calls restoreNotebook and reports restored status, with a descendants caveat", func(t *testing.T) {
		stub := fakeNotebooksClient()
		var calledID string
		stub.RestoreNotebookFunc = func(id string) error {
			calledID = id
			return nil
		}

		result, err := NotebooksCommands["restore"].Run(ctx, args.ParsedArgs{Positionals: []string{"nb1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calledID != "nb1" {
			t.Errorf("restoreNotebook called with %q, want nb1", calledID)
		}
		if !strings.Contains(result.Output, "restored: true") {
			t.Errorf("output does not contain 'restored: true':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "stay trashed") {
			t.Errorf("output does not contain the descendants caveat:\n%s", result.Output)
		}
	})
}

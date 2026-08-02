package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func fakeNotesClient() *clienttest.StubClient {
	return &clienttest.StubClient{
		ListTagsFunc:      func([]string) ([]map[string]any, error) { return nil, nil },
		ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		ListNotesFunc:     func(client.ListNotesOptions) ([]map[string]any, error) { return nil, nil },
	}
}

func TestNotesCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires --title", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["create"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("sends title/body/parent_id and reports the new note id", func(t *testing.T) {
		stub := fakeNotesClient()
		var captured map[string]any
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			captured = fields
			return map[string]any{"id": "abc123", "title": "Hello"}, nil
		}

		parsed := args.ParsedArgs{Flags: map[string]any{"title": "Hello", "body": "World", "notebook": "nb1"}}
		result, err := NotesCommands["create"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(captured) != 3 || captured["title"] != "Hello" || captured["body"] != "World" || captured["parent_id"] != "nb1" {
			t.Errorf("createNote called with %v", captured)
		}
		if !strings.Contains(result.Output, "id: abc123") {
			t.Errorf("output does not contain 'id: abc123':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "notes get abc123") {
			t.Errorf("output does not point at `notes get abc123`:\n%s", result.Output)
		}
	})
}

func TestNotesUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires at least one field to change", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["update"].Run(ctx, args.ParsedArgs{Positionals: []string{"id1"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["update"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"title": "x"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("maps --notebook to parent_id", func(t *testing.T) {
		stub := fakeNotesClient()
		var capturedID string
		var capturedFields map[string]any
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedID, capturedFields = id, fields
			return map[string]any{"id": "id1", "title": "New"}, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"notebook": "nb2"}}
		if _, err := NotesCommands["update"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedID != "id1" || len(capturedFields) != 1 || capturedFields["parent_id"] != "nb2" {
			t.Errorf("updateNote called with (%q, %v)", capturedID, capturedFields)
		}
	})
}

func notesEditGetBody(body string) func(string, []string) (map[string]any, error) {
	return func(string, []string) (map[string]any, error) {
		return map[string]any{"id": "id1", "body": body}, nil
	}
}

func TestNotesEdit(t *testing.T) {
	ctx := context.Background()

	t.Run("requires one of --find/--append/--prepend", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["edit"].Run(ctx, args.ParsedArgs{Positionals: []string{"id1"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects combining --find and --append", func(t *testing.T) {
		stub := fakeNotesClient()
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "a", "replace": "b", "append": "c"}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("requires --replace alongside --find", func(t *testing.T) {
		stub := fakeNotesClient()
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "a"}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects --replace without --find instead of silently ignoring it", func(t *testing.T) {
		stub := fakeNotesClient()
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"append": "x", "replace": "y"}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects --all without --find instead of silently ignoring it", func(t *testing.T) {
		stub := fakeNotesClient()
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"append": "x", "all": true}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects an empty --find instead of matching (and corrupting) the whole body", func(t *testing.T) {
		stub := fakeNotesClient()
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "", "replace": "X"}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("errors if the find text is not present in the body", func(t *testing.T) {
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("hello world")
		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "xyz", "replace": "q"}}
		_, err := NotesCommands["edit"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("replaces only the first occurrence by default", func(t *testing.T) {
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("foo foo foo")
		var capturedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "foo", "replace": "bar"}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "bar foo foo"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}
	})

	t.Run("replaces all occurrences with --all", func(t *testing.T) {
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("foo foo foo")
		var capturedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "foo", "replace": "bar", "all": true}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "bar bar bar"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}
	})

	t.Run("treats the replacement text literally, not as a $-pattern (regression)", func(t *testing.T) {
		// Go's strings.ReplaceAll/replaceFirst never interpret "$&" specially
		// (unlike JS's String.replace with a string search value), so this is
		// really just confirming the port carries no such gotcha forward.
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("see TODO here")
		var capturedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "TODO", "replace": "note: $& done"}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "see note: $& done here"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}
	})

	t.Run("replaces only the first occurrence literally, matching --all's literal semantics", func(t *testing.T) {
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("a$&b a$&b")
		var capturedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"find": "a$&b", "replace": "X"}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "X a$&b"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}
	})

	t.Run("appends and prepends text", func(t *testing.T) {
		stub := fakeNotesClient()
		stub.GetNoteFunc = notesEditGetBody("middle")
		var capturedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			capturedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"append": "!"}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "middle!"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}

		parsed = args.ParsedArgs{Positionals: []string{"id1"}, Flags: map[string]any{"prepend": ">"}}
		if _, err := NotesCommands["edit"].Run(ctx, parsed, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := ">middle"; capturedBody != want {
			t.Errorf("body = %q, want %q", capturedBody, want)
		}
	})
}

func TestNotesDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["delete"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("calls deleteNote (soft delete) and reports trashed status", func(t *testing.T) {
		stub := fakeNotesClient()
		var calledID string
		stub.DeleteNoteFunc = func(id string) error {
			calledID = id
			return nil
		}

		result, err := NotesCommands["delete"].Run(ctx, args.ParsedArgs{Positionals: []string{"id1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calledID != "id1" {
			t.Errorf("deleteNote called with %q, want id1", calledID)
		}
		if len(stub.DeleteNoteCalls) != 1 {
			t.Errorf("deleteNote called %d times, want 1", len(stub.DeleteNoteCalls))
		}
		if !strings.Contains(result.Output, "trashed: true") {
			t.Errorf("output does not contain 'trashed: true':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "notes restore id1") {
			t.Errorf("output does not point at `notes restore id1`:\n%s", result.Output)
		}
	})
}

func TestNotesRestore(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeNotesClient()
		_, err := NotesCommands["restore"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("calls restoreNote and reports restored status", func(t *testing.T) {
		stub := fakeNotesClient()
		var calledID string
		stub.RestoreNoteFunc = func(id string) error {
			calledID = id
			return nil
		}

		result, err := NotesCommands["restore"].Run(ctx, args.ParsedArgs{Positionals: []string{"id1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calledID != "id1" {
			t.Errorf("restoreNote called with %q, want id1", calledID)
		}
		if len(stub.RestoreNoteCalls) != 1 {
			t.Errorf("restoreNote called %d times, want 1", len(stub.RestoreNoteCalls))
		}
		if !strings.Contains(result.Output, "restored: true") {
			t.Errorf("output does not contain 'restored: true':\n%s", result.Output)
		}
	})
}

// ── notes list ───────────────────────────────────────────────────────────────

var notesListDefaultFlags = map[string]any{"limit": float64(20), "task": false, "trash": false}

func notesListArgs(overrides map[string]any) args.ParsedArgs {
	flags := map[string]any{}
	for k, v := range notesListDefaultFlags {
		flags[k] = v
	}
	for k, v := range overrides {
		flags[k] = v
	}
	return args.ParsedArgs{Flags: flags}
}

func listNote(id string, updated int64, extra map[string]any) map[string]any {
	n := map[string]any{"id": id, "title": "note-" + id, "parent_id": "nb1", "updated_time": float64(updated)}
	for k, v := range extra {
		n[k] = v
	}
	return n
}

func fakeNotesListClient() *clienttest.StubClient {
	return &clienttest.StubClient{
		ListTagsFunc: func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "tag1", "title": "active"}}, nil
		},
		ListNotebooksFunc: func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "nb1", "title": "Inbox", "parent_id": ""}}, nil
		},
		ListNotesFunc: func(client.ListNotesOptions) ([]map[string]any, error) { return nil, nil },
	}
}

// dispatchListNotes routes a stub ListNotes call to the right canned result by
// inspecting which filter it was called with — necessary (rather than
// sequential mocks) because notes list's combined-filter path fetches
// concurrently via scope.ResolveNoteScope, so call order isn't guaranteed.
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

func TestNotesListLimitValidation(t *testing.T) {
	ctx := context.Background()
	stub := fakeNotesListClient()

	t.Run("rejects a non-positive --limit instead of silently returning an empty result", func(t *testing.T) {
		_, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"limit": float64(0)}), stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)

		_, err = NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"limit": float64(-5)}), stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})
}

func TestNotesListTrashExclusive(t *testing.T) {
	ctx := context.Background()
	stub := fakeNotesListClient()

	t.Run("rejects --trash combined with --query", func(t *testing.T) {
		_, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"trash": true, "query": "x"}), stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects --trash combined with --notebook", func(t *testing.T) {
		_, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"trash": true, "notebook": "nb1"}), stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})
}

func TestNotesListSingleFilterFastPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("--notebook alone hits the notebook-scoped endpoint", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = func(client.ListNotesOptions) ([]map[string]any, error) {
			return []map[string]any{listNote("n1", 100, nil)}, nil
		}

		if _, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1"}), stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stub.ListNotesCalls) != 1 {
			t.Fatalf("got %d calls, want 1", len(stub.ListNotesCalls))
		}
		opts := stub.ListNotesCalls[0][0].(client.ListNotesOptions)
		if opts.NotebookID != "nb1" {
			t.Errorf("notebookId = %q, want nb1", opts.NotebookID)
		}
		if opts.Query != "" {
			t.Errorf("query = %q, want empty", opts.Query)
		}
	})

	t.Run("--query alone hits search with the plain text (no type:todo)", func(t *testing.T) {
		stub := fakeNotesListClient()

		if _, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"query": "annual report"}), stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		opts := stub.ListNotesCalls[0][0].(client.ListNotesOptions)
		if opts.Query != "annual report" {
			t.Errorf("query = %q, want %q", opts.Query, "annual report")
		}
	})

	t.Run("--task alone appends type:todo to a wildcard search", func(t *testing.T) {
		stub := fakeNotesListClient()

		if _, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"task": true}), stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		opts := stub.ListNotesCalls[0][0].(client.ListNotesOptions)
		if opts.Query != "* type:todo" {
			t.Errorf("query = %q, want '* type:todo'", opts.Query)
		}
	})

	t.Run("no filters at all uses the plain unfiltered listing", func(t *testing.T) {
		stub := fakeNotesListClient()

		if _, err := NotesCommands["list"].Run(ctx, notesListArgs(nil), stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		opts := stub.ListNotesCalls[0][0].(client.ListNotesOptions)
		if opts.Query != "" || opts.NotebookID != "" || opts.TagID != "" {
			t.Errorf("expected an unfiltered call, got %+v", opts)
		}
	})
}

func TestNotesListCombinedFiltersIntersectByID(t *testing.T) {
	ctx := context.Background()

	t.Run("--query + --notebook: only notes in both sets survive", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			[]map[string]any{listNote("n1", 300, nil), listNote("n2", 200, nil)},
			nil,
			[]map[string]any{listNote("n2", 200, nil), listNote("n3", 100, nil)},
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "query": "x"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stub.ListNotesCalls) != 2 {
			t.Fatalf("got %d calls, want 2", len(stub.ListNotesCalls))
		}
		if !strings.Contains(result.Output, "note-n2") {
			t.Errorf("output does not contain note-n2:\n%s", result.Output)
		}
		if strings.Contains(result.Output, "note-n1") || strings.Contains(result.Output, "note-n3") {
			t.Errorf("output should only contain note-n2:\n%s", result.Output)
		}
	})

	t.Run("--query + --tag: intersects by ID the same way", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			nil,
			[]map[string]any{listNote("n1", 300, nil)},
			[]map[string]any{listNote("n1", 300, nil), listNote("n2", 200, nil)},
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"tag": "active", "query": "x"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "note-n1") {
			t.Errorf("output does not contain note-n1:\n%s", result.Output)
		}
		if strings.Contains(result.Output, "note-n2") {
			t.Errorf("output should not contain note-n2:\n%s", result.Output)
		}
	})

	t.Run("--notebook + --tag (no query): also intersects — regression test for the old silent-ignore bug", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			[]map[string]any{listNote("n1", 300, nil), listNote("n2", 200, nil)},
			[]map[string]any{listNote("n2", 200, nil), listNote("n3", 100, nil)},
			nil,
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "tag": "active"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stub.ListNotesCalls) != 2 {
			t.Fatalf("got %d calls, want 2", len(stub.ListNotesCalls))
		}
		if !strings.Contains(result.Output, "note-n2") {
			t.Errorf("output does not contain note-n2:\n%s", result.Output)
		}
		if strings.Contains(result.Output, "note-n1") || strings.Contains(result.Output, "note-n3") {
			t.Errorf("output should only contain note-n2:\n%s", result.Output)
		}
	})

	t.Run("--notebook + --tag + --query: three-way intersection", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			[]map[string]any{listNote("n1", 300, nil), listNote("n2", 200, nil), listNote("n4", 50, nil)},
			[]map[string]any{listNote("n2", 200, nil), listNote("n3", 100, nil)},
			[]map[string]any{listNote("n2", 200, nil), listNote("n4", 50, nil)},
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "tag": "active", "query": "x"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stub.ListNotesCalls) != 3 {
			t.Fatalf("got %d calls, want 3", len(stub.ListNotesCalls))
		}
		if !strings.Contains(result.Output, "note-n2") {
			t.Errorf("output does not contain note-n2:\n%s", result.Output)
		}
		for _, id := range []string{"n1", "n3", "n4"} {
			if strings.Contains(result.Output, "note-"+id) {
				t.Errorf("output should not contain note-%s:\n%s", id, result.Output)
			}
		}
	})

	t.Run("--task + --notebook: type:todo is folded into the search source, then intersected", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = func(opts client.ListNotesOptions) ([]map[string]any, error) {
			if opts.NotebookID != "" {
				return []map[string]any{
					listNote("n1", 300, map[string]any{"is_todo": float64(1)}),
					listNote("n2", 200, map[string]any{"is_todo": float64(0)}),
				}, nil
			}
			return []map[string]any{listNote("n1", 300, map[string]any{"is_todo": float64(1)})}, nil
		}

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "task": true}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var searchQuery string
		for _, c := range stub.ListNotesCalls {
			if opts := c[0].(client.ListNotesOptions); opts.NotebookID == "" {
				searchQuery = opts.Query
			}
		}
		if searchQuery != "* type:todo" {
			t.Errorf("search query = %q, want '* type:todo'", searchQuery)
		}
		if !strings.Contains(result.Output, "note-n1") {
			t.Errorf("output does not contain note-n1:\n%s", result.Output)
		}
		if strings.Contains(result.Output, "note-n2") {
			t.Errorf("output should not contain note-n2:\n%s", result.Output)
		}
	})

	t.Run("empty intersection reports a definitive zero, not an error", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			[]map[string]any{listNote("n1", 300, nil)},
			nil,
			[]map[string]any{listNote("n2", 200, nil)},
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "query": "x"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "notes: 0") {
			t.Errorf("output does not contain 'notes: 0':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "for query `x`, notebook `nb1`") {
			t.Errorf("output does not contain the expected context:\n%s", result.Output)
		}
	})

	t.Run("empty-state context includes --notebook even without a --query (regression)", func(t *testing.T) {
		stub := fakeNotesListClient()

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1"}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "notebook `nb1`") {
			t.Errorf("output does not contain 'notebook `nb1`':\n%s", result.Output)
		}
	})

	t.Run("results are sorted by recency and truncated to --limit", func(t *testing.T) {
		stub := fakeNotesListClient()
		stub.ListNotesFunc = dispatchListNotes(
			[]map[string]any{listNote("old", 100, nil), listNote("new", 300, nil), listNote("mid", 200, nil)},
			nil,
			[]map[string]any{listNote("old", 100, nil), listNote("new", 300, nil), listNote("mid", 200, nil)},
		)

		result, err := NotesCommands["list"].Run(ctx, notesListArgs(map[string]any{"notebook": "nb1", "query": "x", "limit": float64(2)}), stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result.Output, "note-old") {
			t.Errorf("output should not contain note-old once truncated to --limit 2:\n%s", result.Output)
		}
		newIdx := strings.Index(result.Output, "note-new")
		midIdx := strings.Index(result.Output, "note-mid")
		if newIdx == -1 || midIdx == -1 {
			t.Fatalf("expected both note-new and note-mid in output:\n%s", result.Output)
		}
		if newIdx > midIdx {
			t.Errorf("expected note-new before note-mid (sorted by recency):\n%s", result.Output)
		}
	})
}

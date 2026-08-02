package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func fakeTagsClient() *clienttest.StubClient {
	return &clienttest.StubClient{
		ListTagsFunc: func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "tag1", "title": "active"}}, nil
		},
		GetTagsByNoteFunc: func(string, []string) ([]map[string]any, error) { return nil, nil },
		CreateTagFunc:     func(string) (map[string]any, error) { return map[string]any{}, nil },
		UpdateTagFunc:     func(string, string) (map[string]any, error) { return map[string]any{}, nil },
		DeleteTagFunc:     func(string) error { return nil },
		AddTagToNoteFunc:  func(string, string) error { return nil },
		RemoveTagFromNoteFunc: func(string, string) error {
			return nil
		},
		GetNoteFunc: func(id string, _ []string) (map[string]any, error) {
			return map[string]any{"id": id, "title": "note-" + id}, nil
		},
		ListNotesFunc: func(client.ListNotesOptions) ([]map[string]any, error) { return nil, nil },
	}
}

func TestTagsCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires <title>", func(t *testing.T) {
		stub := fakeTagsClient()
		_, err := TagsCommands["create"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("creates a tag and reports its id", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedTitle string
		stub.CreateTagFunc = func(title string) (map[string]any, error) {
			capturedTitle = title
			return map[string]any{"id": "tag2", "title": "urgent"}, nil
		}

		result, err := TagsCommands["create"].Run(ctx, args.ParsedArgs{Positionals: []string{"urgent"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedTitle != "urgent" {
			t.Errorf("createTag called with %q, want urgent", capturedTitle)
		}
		if !strings.Contains(result.Output, "id: tag2") {
			t.Errorf("output does not contain 'id: tag2':\n%s", result.Output)
		}
	})
}

func TestTagsUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("requires <id> and <title>", func(t *testing.T) {
		stub := fakeTagsClient()
		_, err := TagsCommands["update"].Run(ctx, args.ParsedArgs{Positionals: []string{"tag1"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("renames a tag", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedID, capturedTitle string
		stub.UpdateTagFunc = func(id, title string) (map[string]any, error) {
			capturedID, capturedTitle = id, title
			return map[string]any{"id": "tag1", "title": "renamed"}, nil
		}

		if _, err := TagsCommands["update"].Run(ctx, args.ParsedArgs{Positionals: []string{"tag1", "renamed"}}, stub); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedID != "tag1" || capturedTitle != "renamed" {
			t.Errorf("updateTag called with (%q, %q), want (tag1, renamed)", capturedID, capturedTitle)
		}
	})
}

func TestTagsOf(t *testing.T) {
	t.Run("shows a hint for adding a tag to the note", func(t *testing.T) {
		stub := fakeTagsClient()
		result, err := TagsCommands["of"].Run(context.Background(), args.ParsedArgs{Positionals: []string{"n1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "tags add <title> --notes n1") {
			t.Errorf("output does not contain the expected hint:\n%s", result.Output)
		}
	})
}

func TestTagsDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("requires an id", func(t *testing.T) {
		stub := fakeTagsClient()
		_, err := TagsCommands["delete"].Run(ctx, args.ParsedArgs{}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("deletes a tag", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedID string
		stub.DeleteTagFunc = func(id string) error {
			capturedID = id
			return nil
		}

		result, err := TagsCommands["delete"].Run(ctx, args.ParsedArgs{Positionals: []string{"tag1"}}, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedID != "tag1" {
			t.Errorf("deleteTag called with %q, want tag1", capturedID)
		}
		if !strings.Contains(result.Output, "deleted: true") {
			t.Errorf("output does not contain 'deleted: true':\n%s", result.Output)
		}
	})
}

func TestTagsAdd(t *testing.T) {
	ctx := context.Background()

	t.Run("requires <tag-title>", func(t *testing.T) {
		stub := fakeTagsClient()
		_, err := TagsCommands["add"].Run(ctx, args.ParsedArgs{Flags: map[string]any{"notes": "n1"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("requires --notes or a filter", func(t *testing.T) {
		stub := fakeTagsClient()
		_, err := TagsCommands["add"].Run(ctx, args.ParsedArgs{Positionals: []string{"active"}}, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects --notes combined with a filter", func(t *testing.T) {
		stub := fakeTagsClient()
		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notes": "n1", "query": "x"}}
		_, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("errors if the tag title does not exist", func(t *testing.T) {
		stub := fakeTagsClient()
		stub.ListTagsFunc = func([]string) ([]map[string]any, error) { return nil, nil }
		parsed := args.ParsedArgs{Positionals: []string{"nonexistent"}, Flags: map[string]any{"notes": "n1"}}
		_, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("resolves the tag title to an id and applies it to each explicit note", func(t *testing.T) {
		stub := fakeTagsClient()
		var calls [][2]string
		stub.AddTagToNoteFunc = func(tagID, noteID string) error {
			calls = append(calls, [2]string{tagID, noteID})
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notes": "n1,n2"}}
		result, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(calls) != 2 || calls[0] != [2]string{"tag1", "n1"} || calls[1] != [2]string{"tag1", "n2"} {
			t.Errorf("addTagToNote calls = %v", calls)
		}
		if !strings.Contains(result.Output, "added_to: 2") || !strings.Contains(result.Output, "note-n1") {
			t.Errorf("output missing expected content:\n%s", result.Output)
		}
		if result.ExitCode != 0 {
			t.Errorf("exit code = %d, want 0 (no failures)", result.ExitCode)
		}
	})

	t.Run("tags the valid explicit notes and reports an unresolvable ID as failed, instead of aborting the whole batch", func(t *testing.T) {
		stub := fakeTagsClient()
		stub.GetNoteFunc = func(id string, _ []string) (map[string]any, error) {
			if id == "badid" {
				return nil, errors.New("Joplin API GET /notes/badid failed: 404")
			}
			return map[string]any{"id": id, "title": "note-" + id}, nil
		}
		var addedTo []string
		stub.AddTagToNoteFunc = func(_, noteID string) error {
			addedTo = append(addedTo, noteID)
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notes": "n1,badid,n3"}}
		result, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addedTo) != 2 {
			t.Errorf("addTagToNote called %d times, want 2", len(addedTo))
		}
		for _, want := range []string{"added_to: 2", "failed: 1", "badid", "404"} {
			if !strings.Contains(result.Output, want) {
				t.Errorf("output does not contain %q:\n%s", want, result.Output)
			}
		}
		if result.ExitCode != 1 {
			t.Errorf("exit code = %d, want 1", result.ExitCode)
		}
	})

	t.Run("selects notes by filter (--notebook) instead of explicit IDs", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedOpts client.ListNotesOptions
		stub.ListNotesFunc = func(opts client.ListNotesOptions) ([]map[string]any, error) {
			capturedOpts = opts
			return []map[string]any{
				{"id": "m1", "title": "Matched One", "updated_time": float64(100)},
				{"id": "m2", "title": "Matched Two", "updated_time": float64(200)},
			}, nil
		}
		var addCount int
		stub.AddTagToNoteFunc = func(string, string) error {
			addCount++
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notebook": "nb1"}}
		result, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedOpts.NotebookID != "nb1" {
			t.Errorf("notebookID = %q, want nb1", capturedOpts.NotebookID)
		}
		if addCount != 2 {
			t.Errorf("addTagToNote called %d times, want 2", addCount)
		}
		for _, want := range []string{"added_to: 2", "Matched One", "Matched Two", "notes list --tag active"} {
			if !strings.Contains(result.Output, want) {
				t.Errorf("output does not contain %q:\n%s", want, result.Output)
			}
		}
	})

	t.Run("reports a definitive zero when the filter matches nothing, without erroring", func(t *testing.T) {
		stub := fakeTagsClient()
		called := false
		stub.AddTagToNoteFunc = func(string, string) error {
			called = true
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notebook": "nb1"}}
		result, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("addTagToNote should not have been called")
		}
		for _, want := range []string{"added_to: 0", "nothing tagged", "notebooks list"} {
			if !strings.Contains(result.Output, want) {
				t.Errorf("output does not contain %q:\n%s", want, result.Output)
			}
		}
	})

	t.Run("reports both successes and failures when one note fails mid-batch, instead of throwing and hiding the rest", func(t *testing.T) {
		stub := fakeTagsClient()
		var callCount int
		stub.AddTagToNoteFunc = func(_, noteID string) error {
			callCount++
			if noteID == "n2" {
				return errors.New("note locked")
			}
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notes": "n1,n2,n3"}}
		result, err := TagsCommands["add"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 3 {
			t.Errorf("addTagToNote called %d times, want 3", callCount)
		}
		for _, want := range []string{"added_to: 2", "failed: 1", "note-n1", "note-n3", "note-n2", "note locked", "retry with"} {
			if !strings.Contains(result.Output, want) {
				t.Errorf("output does not contain %q:\n%s", want, result.Output)
			}
		}
		if result.ExitCode != 1 {
			t.Errorf("exit code = %d, want 1", result.ExitCode)
		}
	})
}

func TestTagsRemove(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves the tag title to an id and removes it from each note", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedTagID, capturedNoteID string
		stub.RemoveTagFromNoteFunc = func(tagID, noteID string) error {
			capturedTagID, capturedNoteID = tagID, noteID
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notes": "n1"}}
		result, err := TagsCommands["remove"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedTagID != "tag1" || capturedNoteID != "n1" {
			t.Errorf("removeTagFromNote called with (%q, %q), want (tag1, n1)", capturedTagID, capturedNoteID)
		}
		if !strings.Contains(result.Output, "removed_from: 1") {
			t.Errorf("output does not contain 'removed_from: 1':\n%s", result.Output)
		}
	})

	t.Run("selects notes by filter (--query) instead of explicit IDs", func(t *testing.T) {
		stub := fakeTagsClient()
		var capturedOpts client.ListNotesOptions
		stub.ListNotesFunc = func(opts client.ListNotesOptions) ([]map[string]any, error) {
			capturedOpts = opts
			return []map[string]any{{"id": "m1", "title": "Matched", "updated_time": float64(100)}}, nil
		}
		var capturedNoteID string
		stub.RemoveTagFromNoteFunc = func(_, noteID string) error {
			capturedNoteID = noteID
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"query": "x"}}
		result, err := TagsCommands["remove"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedOpts.Query != "x" {
			t.Errorf("query = %q, want x", capturedOpts.Query)
		}
		if capturedNoteID != "m1" {
			t.Errorf("removeTagFromNote called with noteID %q, want m1", capturedNoteID)
		}
		if !strings.Contains(result.Output, "removed_from: 1") || !strings.Contains(result.Output, "tags add active --notes") {
			t.Errorf("output missing expected content:\n%s", result.Output)
		}
	})

	t.Run("reports a definitive zero when the filter matches nothing, without erroring", func(t *testing.T) {
		stub := fakeTagsClient()
		called := false
		stub.RemoveTagFromNoteFunc = func(string, string) error {
			called = true
			return nil
		}

		parsed := args.ParsedArgs{Positionals: []string{"active"}, Flags: map[string]any{"notebook": "nb1"}}
		result, err := TagsCommands["remove"].Run(ctx, parsed, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("removeTagFromNote should not have been called")
		}
		if !strings.Contains(result.Output, "removed_from: 0") || !strings.Contains(result.Output, "nothing removed") {
			t.Errorf("output missing expected content:\n%s", result.Output)
		}
	})
}

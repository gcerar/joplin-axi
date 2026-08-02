package importer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func baseStub() *clienttest.StubClient {
	return &clienttest.StubClient{
		ListTagsFunc:  func([]string) ([]map[string]any, error) { return nil, nil },
		ListNotesFunc: func(client.ListNotesOptions) ([]map[string]any, error) { return nil, nil },
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestRunImportNotebooks(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a nested notebook chain parent-before-child, memoized by ref", func(t *testing.T) {
		stub := baseStub()
		var calls []map[string]any
		stub.CreateNotebookFunc = func(fields map[string]any) (map[string]any, error) {
			calls = append(calls, fields)
			if len(calls) == 1 {
				return map[string]any{"id": "parent-id", "title": "Parent"}, nil
			}
			return map[string]any{"id": "child-id", "title": "Child"}, nil
		}
		var noteFields map[string]any
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			noteFields = fields
			return map[string]any{"id": "note-id"}, nil
		}

		parsed := ParsedImport{
			Notebooks: []ParsedNotebook{
				{Ref: "child", Title: "Child", ParentRef: "parent"},
				{Ref: "parent", Title: "Parent"},
			},
			Notes: []ParsedNote{{Title: "N", Body: "b", NotebookRef: "child"}},
		}

		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(calls) != 2 {
			t.Fatalf("got %d createNotebook calls, want 2", len(calls))
		}
		if calls[0]["title"] != "Parent" {
			t.Errorf("call 1 = %v, want title Parent with no parent_id", calls[0])
		}
		if _, ok := calls[0]["parent_id"]; ok {
			t.Errorf("call 1 = %v, should have no parent_id", calls[0])
		}
		if calls[1]["title"] != "Child" || calls[1]["parent_id"] != "parent-id" {
			t.Errorf("call 2 = %v, want title Child parent_id parent-id", calls[1])
		}
		if noteFields["parent_id"] != "child-id" {
			t.Errorf("createNote parent_id = %v, want child-id", noteFields["parent_id"])
		}
		if report.NotebooksCreated != 2 {
			t.Errorf("notebooksCreated = %d, want 2", report.NotebooksCreated)
		}
	})

	t.Run("grafts a root-level notebook under the --notebook target when given", func(t *testing.T) {
		stub := baseStub()
		var captured map[string]any
		stub.CreateNotebookFunc = func(fields map[string]any) (map[string]any, error) {
			captured = fields
			return map[string]any{"id": "new-id", "title": "Top"}, nil
		}

		parsed := ParsedImport{Notebooks: []ParsedNotebook{{Ref: "top", Title: "Top"}}}
		if _, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "graft-target", OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured["title"] != "Top" || captured["parent_id"] != "graft-target" {
			t.Errorf("createNotebook called with %v", captured)
		}
	})

	t.Run("sends a note with RootNotebookRef directly to the --notebook target, with no notebook creation", func(t *testing.T) {
		stub := baseStub()
		notebookCalled := false
		stub.CreateNotebookFunc = func(map[string]any) (map[string]any, error) {
			notebookCalled = true
			return nil, nil
		}
		var noteFields map[string]any
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			noteFields = fields
			return map[string]any{"id": "note-id"}, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{{Title: "N", Body: "b", NotebookRef: RootNotebookRef}}}
		if _, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "target-nb", OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if notebookCalled {
			t.Errorf("createNotebook should not have been called")
		}
		if noteFields["parent_id"] != "target-nb" {
			t.Errorf("createNote parent_id = %v, want target-nb", noteFields["parent_id"])
		}
	})
}

func TestRunImportTags(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a tag once and applies it to every note referencing it", func(t *testing.T) {
		stub := baseStub()
		createTagCalls := 0
		stub.CreateTagFunc = func(title string) (map[string]any, error) {
			createTagCalls++
			return map[string]any{"id": "tag-id", "title": "work"}, nil
		}
		var addedTagID, addedNoteID string
		stub.AddTagToNoteFunc = func(tagID, noteID string) error {
			addedTagID, addedNoteID = tagID, noteID
			return nil
		}
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "note-id"}, nil }

		parsed := ParsedImport{
			Tags:  []ParsedTag{{Ref: "work", Title: "work"}},
			Notes: []ParsedNote{{Title: "N", Body: "b", NotebookRef: RootNotebookRef, TagRefs: []string{"work"}}},
		}

		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createTagCalls != 1 {
			t.Errorf("createTag called %d times, want 1", createTagCalls)
		}
		if addedTagID != "tag-id" || addedNoteID != "note-id" {
			t.Errorf("addTagToNote called with (%q, %q)", addedTagID, addedNoteID)
		}
		if report.TagsCreated != 1 {
			t.Errorf("tagsCreated = %d, want 1", report.TagsCreated)
		}
	})

	t.Run("reuses an existing tag by title instead of creating a duplicate", func(t *testing.T) {
		stub := baseStub()
		createTagCalls := 0
		stub.CreateTagFunc = func(string) (map[string]any, error) { createTagCalls++; return nil, nil }
		stub.ListTagsFunc = func([]string) ([]map[string]any, error) {
			return []map[string]any{{"id": "existing-id", "title": "work"}}, nil
		}

		parsed := ParsedImport{Tags: []ParsedTag{{Ref: "work", Title: "work"}}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createTagCalls != 0 {
			t.Errorf("createTag called %d times, want 0", createTagCalls)
		}
		if report.TagsCreated != 0 {
			t.Errorf("tagsCreated = %d, want 0", report.TagsCreated)
		}
	})
}

func TestRunImportTimestamps(t *testing.T) {
	ctx := context.Background()

	t.Run("follows up with updateNote for created_time/updated_time when present", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "note-id"}, nil }
		var updateID string
		var updateFields map[string]any
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			updateID, updateFields = id, fields
			return nil, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "N", Body: "b", NotebookRef: RootNotebookRef, CreatedTime: int64Ptr(1000), UpdatedTime: int64Ptr(2000)},
		}}
		if _, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updateID != "note-id" {
			t.Errorf("updateNote id = %q, want note-id", updateID)
		}
		if updateFields["created_time"] != int64(1000) || updateFields["updated_time"] != int64(2000) {
			t.Errorf("updateNote fields = %v", updateFields)
		}
	})

	t.Run("skips the timestamp updateNote call entirely when neither is set", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "note-id"}, nil }
		updateCalled := false
		stub.UpdateNoteFunc = func(string, map[string]any) (map[string]any, error) { updateCalled = true; return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{{Title: "N", Body: "b", NotebookRef: RootNotebookRef}}}
		if _, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updateCalled {
			t.Errorf("updateNote should not have been called")
		}
	})
}

func TestRunImportDedup(t *testing.T) {
	ctx := context.Background()

	t.Run("skips a note whose title already exists in the target notebook (default skip policy)", func(t *testing.T) {
		stub := baseStub()
		stub.ListNotesFunc = func(client.ListNotesOptions) ([]map[string]any, error) {
			return []map[string]any{{"id": "existing", "title": "Dup"}}, nil
		}
		createCalled := false
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { createCalled = true; return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{{Title: "Dup", Body: "b", NotebookRef: RootNotebookRef}}}
		report, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "nb1", OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createCalled {
			t.Errorf("createNote should not have been called")
		}
		want := []TitledReason{{Title: "Dup", Reason: "duplicate title in target notebook"}}
		if len(report.NotesSkipped) != 1 || report.NotesSkipped[0] != want[0] {
			t.Errorf("notesSkipped = %+v, want %+v", report.NotesSkipped, want)
		}
	})

	t.Run("renames a duplicate title with a numeric suffix under the rename policy", func(t *testing.T) {
		stub := baseStub()
		stub.ListNotesFunc = func(client.ListNotesOptions) ([]map[string]any, error) {
			return []map[string]any{{"id": "existing", "title": "Dup"}}, nil
		}
		var capturedTitle string
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			capturedTitle, _ = fields["title"].(string)
			return map[string]any{"id": "new-id"}, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{{Title: "Dup", Body: "b", NotebookRef: RootNotebookRef}}}
		if _, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "nb1", OnDuplicate: OnDuplicateRename}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedTitle != "Dup (1)" {
			t.Errorf("title = %q, want %q", capturedTitle, "Dup (1)")
		}
	})

	t.Run("renames a second within-batch duplicate to (2), not (1) again", func(t *testing.T) {
		stub := baseStub()
		var titles []string
		i := 0
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			i++
			titles = append(titles, fields["title"].(string))
			return map[string]any{"id": fmt.Sprintf("id%d", i)}, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "Same", Body: "b", NotebookRef: RootNotebookRef},
			{Title: "Same", Body: "b", NotebookRef: RootNotebookRef},
			{Title: "Same", Body: "b", NotebookRef: RootNotebookRef},
		}}
		if _, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "nb1", OnDuplicate: OnDuplicateRename}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"Same", "Same (1)", "Same (2)"}
		if len(titles) != 3 || titles[0] != want[0] || titles[1] != want[1] || titles[2] != want[2] {
			t.Errorf("titles = %v, want %v", titles, want)
		}
	})
}

func TestRunImportLinkRewriteResourceStyle(t *testing.T) {
	ctx := context.Background()

	t.Run("rewrites a forward reference to a note created later in the batch", func(t *testing.T) {
		stub := baseStub()
		createCalls := 0
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) {
			createCalls++
			if createCalls == 1 {
				return map[string]any{"id": "new-a"}, nil
			}
			return map[string]any{"id": "new-b"}, nil
		}
		stub.GetNoteFunc = func(id string, fields []string) (map[string]any, error) {
			if id == "new-a" {
				return map[string]any{"id": "new-a", "body": "See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb)."}, nil
			}
			return map[string]any{"id": "new-b", "body": "no links"}, nil
		}
		var updatedID string
		var updatedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			updatedID, _ = id, id
			updatedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).", NotebookRef: RootNotebookRef, SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Title: "B", Body: "no links", NotebookRef: RootNotebookRef, SourceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updatedID != "new-a" || updatedBody != "See [B](:/new-b)." {
			t.Errorf("updateNote(%q, %q)", updatedID, updatedBody)
		}
		if report.UnresolvedLinks != 0 {
			t.Errorf("unresolvedLinks = %d, want 0", report.UnresolvedLinks)
		}
	})

	t.Run("uploads a referenced resource exactly once and rewrites the token to the new resource ID", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "new-a"}, nil }
		stub.GetNoteFunc = func(string, []string) (map[string]any, error) {
			return map[string]any{"id": "new-a", "body": "Photo: ![x](:/cccccccccccccccccccccccccccccccc)."}, nil
		}
		createResourceCalls := 0
		var capturedData []byte
		var capturedFilename, capturedMime string
		stub.CreateResourceFunc = func(data []byte, filename, mime string) (map[string]any, error) {
			createResourceCalls++
			capturedData, capturedFilename, capturedMime = data, filename, mime
			return map[string]any{"id": "new-res", "title": "photo.png"}, nil
		}
		var updatedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			updatedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := ParsedImport{
			Resources: []ParsedResource{{ID: "cccccccccccccccccccccccccccccccc", Filename: "photo.png", Mime: "image/png", Data: []byte{1, 2, 3}}},
			Notes: []ParsedNote{
				{Title: "A", Body: "Photo: ![x](:/cccccccccccccccccccccccccccccccc).", NotebookRef: RootNotebookRef, SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			},
		}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createResourceCalls != 1 {
			t.Errorf("createResource called %d times, want 1", createResourceCalls)
		}
		if string(capturedData) != "\x01\x02\x03" || capturedFilename != "photo.png" || capturedMime != "image/png" {
			t.Errorf("createResource args = (%v, %q, %q)", capturedData, capturedFilename, capturedMime)
		}
		if updatedBody != "Photo: ![x](:/new-res)." {
			t.Errorf("updated body = %q", updatedBody)
		}
		if report.ResourcesUploaded != 1 {
			t.Errorf("resourcesUploaded = %d, want 1", report.ResourcesUploaded)
		}
	})

	t.Run("leaves a :/id token untouched and uncounted when it belongs to neither this batch's notes nor resources", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "new-a"}, nil }
		stub.GetNoteFunc = func(string, []string) (map[string]any, error) {
			return map[string]any{"id": "new-a", "body": "Pre-existing link: [x](:/ffffffffffffffffffffffffffffffff)."}, nil
		}
		updateCalled := false
		stub.UpdateNoteFunc = func(string, map[string]any) (map[string]any, error) { updateCalled = true; return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "Pre-existing link: [x](:/ffffffffffffffffffffffffffffffff).", NotebookRef: RootNotebookRef, SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updateCalled {
			t.Errorf("updateNote should not have been called")
		}
		if report.UnresolvedLinks != 0 {
			t.Errorf("unresolvedLinks = %d, want 0", report.UnresolvedLinks)
		}
	})

	t.Run("counts an unresolved link when the referenced note was skipped as a duplicate", func(t *testing.T) {
		stub := baseStub()
		stub.ListNotesFunc = func(client.ListNotesOptions) ([]map[string]any, error) {
			return []map[string]any{{"id": "existing", "title": "B"}}, nil
		}
		createCalls := 0
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) {
			createCalls++
			return map[string]any{"id": "new-a"}, nil
		}
		stub.GetNoteFunc = func(string, []string) (map[string]any, error) {
			return map[string]any{"id": "new-a", "body": "See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb)."}, nil
		}
		updateCalled := false
		stub.UpdateNoteFunc = func(string, map[string]any) (map[string]any, error) { updateCalled = true; return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "See [B](:/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb).", NotebookRef: RootNotebookRef, SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Title: "B", Body: "no links", NotebookRef: RootNotebookRef, SourceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{TargetNotebookID: "nb1", OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.NotesSkipped) != 1 {
			t.Errorf("notesSkipped = %+v, want 1 entry", report.NotesSkipped)
		}
		if report.UnresolvedLinks != 1 {
			t.Errorf("unresolvedLinks = %d, want 1", report.UnresolvedLinks)
		}
		if updateCalled {
			t.Errorf("updateNote should not have been called")
		}
	})
}

func TestRunImportLinkRewriteMarkdown(t *testing.T) {
	ctx := context.Background()

	t.Run("rewrites a relative .md link to an internal :/id link once the target note exists", func(t *testing.T) {
		stub := baseStub()
		createCalls := 0
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) {
			createCalls++
			if createCalls == 1 {
				return map[string]any{"id": "new-a"}, nil
			}
			return map[string]any{"id": "new-b"}, nil
		}
		stub.GetNoteFunc = func(id string, _ []string) (map[string]any, error) {
			if id == "new-a" {
				return map[string]any{"id": "new-a", "body": "See [B](./b.md) for details."}, nil
			}
			return map[string]any{"id": "new-b", "body": "body"}, nil
		}
		var updatedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			updatedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "See [B](./b.md) for details.", NotebookRef: RootNotebookRef, SourceFilePath: "/import/a.md"},
			{Title: "B", Body: "body", NotebookRef: RootNotebookRef, SourceFilePath: "/import/b.md"},
		}}
		if _, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updatedBody != "See [B](:/new-b) for details." {
			t.Errorf("updated body = %q", updatedBody)
		}
	})

	t.Run("leaves an external http(s) link untouched", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "new-a"}, nil }
		stub.GetNoteFunc = func(string, []string) (map[string]any, error) {
			return map[string]any{"id": "new-a", "body": "See [site](https://example.com)."}, nil
		}
		updateCalled := false
		stub.UpdateNoteFunc = func(string, map[string]any) (map[string]any, error) { updateCalled = true; return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "See [site](https://example.com).", NotebookRef: RootNotebookRef, SourceFilePath: "/import/a.md"},
		}}
		if _, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updateCalled {
			t.Errorf("updateNote should not have been called")
		}
	})

	t.Run("uploads a markdown-sourced local asset (a note with sourceFilePath but no sourceId) — regression for the resource pass being gated on sourceId", func(t *testing.T) {
		const assetID = "d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1"
		stub := baseStub()
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) { return map[string]any{"id": "new-a"}, nil }
		stub.GetNoteFunc = func(string, []string) (map[string]any, error) {
			return map[string]any{"id": "new-a", "body": "![diagram](:/" + assetID + ")"}, nil
		}
		var capturedData []byte
		stub.CreateResourceFunc = func(data []byte, filename, mime string) (map[string]any, error) {
			capturedData = data
			return map[string]any{"id": "uploaded-res", "title": "diagram.png"}, nil
		}
		var updatedBody string
		stub.UpdateNoteFunc = func(id string, fields map[string]any) (map[string]any, error) {
			updatedBody, _ = fields["body"].(string)
			return nil, nil
		}

		parsed := ParsedImport{
			Resources: []ParsedResource{{ID: assetID, Filename: "diagram.png", Mime: "image/png", Data: []byte{9, 9, 9}}},
			Notes: []ParsedNote{
				{Title: "A", Body: "![diagram](:/" + assetID + ")", NotebookRef: RootNotebookRef, SourceFilePath: "/import/a.md"},
			},
		}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(capturedData) != "\x09\x09\x09" {
			t.Errorf("createResource data = %v", capturedData)
		}
		if updatedBody != "![diagram](:/uploaded-res)" {
			t.Errorf("updated body = %q", updatedBody)
		}
		if report.ResourcesUploaded != 1 {
			t.Errorf("resourcesUploaded = %d, want 1", report.ResourcesUploaded)
		}
	})
}

func TestRunImportFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("records a note as failed without aborting the rest of the batch", func(t *testing.T) {
		stub := baseStub()
		calls := 0
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("boom")
			}
			return map[string]any{"id": "ok-id"}, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "Bad", Body: "b", NotebookRef: RootNotebookRef},
			{Title: "Good", Body: "b", NotebookRef: RootNotebookRef},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []TitledError{{Title: "Bad", Error: "boom"}}
		if len(report.NotesFailed) != 1 || report.NotesFailed[0] != want[0] {
			t.Errorf("notesFailed = %+v, want %+v", report.NotesFailed, want)
		}
		if report.NotesCreated != 1 {
			t.Errorf("notesCreated = %d, want 1", report.NotesCreated)
		}
	})

	t.Run("records a link-rewrite failure without losing the rest of the report or aborting later notes", func(t *testing.T) {
		stub := baseStub()
		createCalls := 0
		stub.CreateNoteFunc = func(map[string]any) (map[string]any, error) {
			createCalls++
			if createCalls == 1 {
				return map[string]any{"id": "new-a"}, nil
			}
			return map[string]any{"id": "new-b"}, nil
		}
		var getNoteCalledWithB bool
		stub.GetNoteFunc = func(id string, fields []string) (map[string]any, error) {
			if id == "new-a" {
				return nil, errors.New("network blip")
			}
			getNoteCalledWithB = len(fields) == 2 && fields[0] == "id" && fields[1] == "body"
			return map[string]any{"id": "new-b", "body": "no links"}, nil
		}
		stub.UpdateNoteFunc = func(string, map[string]any) (map[string]any, error) { return nil, nil }

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "A", Body: "a", NotebookRef: RootNotebookRef, SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Title: "B", Body: "no links", NotebookRef: RootNotebookRef, SourceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Both notes were still created in pass 1 — the pass-2 failure on A
		// must not prevent B's own link rewrite from running, and must not
		// throw out of Run losing the whole report.
		if report.NotesCreated != 2 {
			t.Errorf("notesCreated = %d, want 2", report.NotesCreated)
		}
		want := []TitledError{{Title: "A", Error: "network blip"}}
		if len(report.LinkRewriteFailed) != 1 || report.LinkRewriteFailed[0] != want[0] {
			t.Errorf("linkRewriteFailed = %+v, want %+v", report.LinkRewriteFailed, want)
		}
		if !getNoteCalledWithB {
			t.Errorf("getNote should have been called for new-b with [id body]")
		}
	})

	t.Run("does not treat a duplicate title as taken when its predecessor failed to create", func(t *testing.T) {
		stub := baseStub()
		var titles []string
		calls := 0
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			calls++
			titles = append(titles, fields["title"].(string))
			if calls == 1 {
				return nil, errors.New("boom")
			}
			return map[string]any{"id": "second-id"}, nil
		}

		parsed := ParsedImport{Notes: []ParsedNote{
			{Title: "Same", Body: "a", NotebookRef: RootNotebookRef},
			{Title: "Same", Body: "b", NotebookRef: RootNotebookRef},
		}}
		report, err := Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The first "Same" failed to create, so the second one isn't a real
		// duplicate — it must still be attempted (and succeed) with its own
		// title.
		if calls != 2 {
			t.Fatalf("createNote called %d times, want 2", calls)
		}
		if titles[1] != "Same" {
			t.Errorf("second call title = %q, want Same", titles[1])
		}
		want := []TitledError{{Title: "Same", Error: "boom"}}
		if len(report.NotesFailed) != 1 || report.NotesFailed[0] != want[0] {
			t.Errorf("notesFailed = %+v, want %+v", report.NotesFailed, want)
		}
		if len(report.NotesSkipped) != 0 {
			t.Errorf("notesSkipped = %+v, want empty", report.NotesSkipped)
		}
		if report.NotesCreated != 1 {
			t.Errorf("notesCreated = %d, want 1", report.NotesCreated)
		}
	})

	t.Run("resolves a cyclic notebook parentRef chain without hanging, treating the cycle-closing edge as top-level", func(t *testing.T) {
		stub := baseStub()
		stub.CreateNotebookFunc = func(fields map[string]any) (map[string]any, error) {
			title := fields["title"].(string)
			return map[string]any{"id": "id-" + title, "title": title}, nil
		}
		var noteParentID string
		stub.CreateNoteFunc = func(fields map[string]any) (map[string]any, error) {
			noteParentID, _ = fields["parent_id"].(string)
			return map[string]any{"id": "note-id"}, nil
		}

		parsed := ParsedImport{
			Notebooks: []ParsedNotebook{
				{Ref: "a", Title: "A", ParentRef: "b"},
				{Ref: "b", Title: "B", ParentRef: "a"},
			},
			Notes: []ParsedNote{{Title: "Note", Body: "x", NotebookRef: "a"}},
		}

		done := make(chan struct{})
		var report ImportReport
		var runErr error
		go func() {
			report, runErr = Run(ctx, parsed, stub, RunOptions{OnDuplicate: OnDuplicateSkip})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run hung on a notebook cycle")
		}
		if runErr != nil {
			t.Fatalf("unexpected error: %v", runErr)
		}
		if report.NotebooksCreated != 2 {
			t.Errorf("notebooksCreated = %d, want 2", report.NotebooksCreated)
		}
		if noteParentID != "id-A" {
			t.Errorf("note parent_id = %q, want id-A", noteParentID)
		}
	})
}

package importer

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTar writes a tar archive (no gzip) at jexPath containing the given
// name -> content entries — the Go-native equivalent of the TS fixture's
// `tar.create({ cwd: src, gzip: false }, ['.'])` over a directory of files.
func buildTar(t *testing.T, jexPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(jexPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write content %s: %v", name, err)
		}
	}
}

func buildFixtureJex(t *testing.T, jexPath string) {
	t.Helper()

	folderMd := strings.Join([]string{
		"My Notebook", "",
		"id: nb0000000000000000000000000000",
		"created_time: 2023-01-01T00:00:00.000Z",
		"updated_time: 2023-01-01T00:00:00.000Z",
		"type_: 2",
	}, "\n")

	tagMd := strings.Join([]string{
		"work", "",
		"id: tag000000000000000000000000",
		"created_time: 2023-01-01T00:00:00.000Z",
		"updated_time: 2023-01-01T00:00:00.000Z",
		"type_: 5",
	}, "\n")

	resourceMetaMd := strings.Join([]string{
		"photo.png", "",
		"id: res0000000000000000000000000",
		"mime: image/png",
		"filename: photo.png",
		"created_time: 2023-01-01T00:00:00.000Z",
		"updated_time: 2023-01-01T00:00:00.000Z",
		"type_: 4",
	}, "\n")

	note2Md := strings.Join([]string{
		"Second Note", "",
		"Just some body text, no links.", "",
		"id: note2000000000000000000000000",
		"parent_id: nb0000000000000000000000000000",
		"created_time: 2023-02-01T00:00:00.000Z",
		"updated_time: 2023-02-02T00:00:00.000Z",
		"is_todo: 0",
		"todo_completed: 0",
		"type_: 1",
	}, "\n")

	note1Md := strings.Join([]string{
		"First Note", "",
		"Links to [Second Note](:/note2000000000000000000000000) and an image ![photo](:/res0000000000000000000000000).", "",
		"id: note1000000000000000000000000",
		"parent_id: nb0000000000000000000000000000",
		"created_time: 2023-01-05T00:00:00.000Z",
		"updated_time: 2023-01-06T00:00:00.000Z",
		"is_todo: 1",
		"todo_completed: 0",
		"type_: 1",
	}, "\n")

	relationMd := strings.Join([]string{
		"note1000000000000000000000000-tag000000000000000000000000", "",
		"id: rel0000000000000000000000000",
		"note_id: note1000000000000000000000000",
		"tag_id: tag000000000000000000000000",
		"created_time: 2023-01-05T00:00:00.000Z",
		"updated_time: 2023-01-05T00:00:00.000Z",
		"type_: 6",
	}, "\n")

	buildTar(t, jexPath, map[string]string{
		"nb0000000000000000000000000000.md":          folderMd,
		"tag000000000000000000000000.md":             tagMd,
		"res0000000000000000000000000.md":            resourceMetaMd,
		"resources/res0000000000000000000000000.png": string([]byte{0x89, 0x50, 0x4e, 0x47}),
		"note2000000000000000000000000.md":           note2Md,
		"note1000000000000000000000000.md":           note1Md,
		"rel0000000000000000000000000.md":            relationMd,
	})
}

func TestParseJexSource(t *testing.T) {
	t.Run("rejects a file that is not a valid tar archive", func(t *testing.T) {
		dir := t.TempDir()
		jexPath := filepath.Join(dir, "export.jex")
		if err := os.WriteFile(jexPath, []byte("not a tar archive at all"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := ParseJexSource(jexPath)
		if err == nil || !strings.Contains(err.Error(), "empty or invalid JEX archive") {
			t.Fatalf("got error %v, want 'empty or invalid JEX archive'", err)
		}
	})

	t.Run("reconstructs notebooks, tags, note-tag relations, notes, and resources from a real tar archive", func(t *testing.T) {
		dir := t.TempDir()
		jexPath := filepath.Join(dir, "export.jex")
		buildFixtureJex(t, jexPath)

		result, err := ParseJexSource(jexPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Notebooks) != 1 || result.Notebooks[0] != (ParsedNotebook{Ref: "nb0000000000000000000000000000", Title: "My Notebook"}) {
			t.Errorf("notebooks = %+v", result.Notebooks)
		}
		if len(result.Tags) != 1 || result.Tags[0] != (ParsedTag{Ref: "tag000000000000000000000000", Title: "work"}) {
			t.Errorf("tags = %+v", result.Tags)
		}

		if len(result.Notes) != 2 {
			t.Fatalf("got %d notes, want 2", len(result.Notes))
		}
		var first, second *ParsedNote
		for i := range result.Notes {
			switch result.Notes[i].Title {
			case "First Note":
				first = &result.Notes[i]
			case "Second Note":
				second = &result.Notes[i]
			}
		}
		if first == nil || second == nil {
			t.Fatalf("expected both First Note and Second Note, got %+v", result.Notes)
		}

		if first.NotebookRef != "nb0000000000000000000000000000" {
			t.Errorf("first.NotebookRef = %q", first.NotebookRef)
		}
		if first.SourceID != "note1000000000000000000000000" {
			t.Errorf("first.SourceID = %q", first.SourceID)
		}
		if !first.IsTodo {
			t.Errorf("first.IsTodo = false, want true")
		}
		if first.TodoCompleted {
			t.Errorf("first.TodoCompleted = true, want false")
		}
		if len(first.TagRefs) != 1 || first.TagRefs[0] != "tag000000000000000000000000" {
			t.Errorf("first.TagRefs = %v", first.TagRefs)
		}
		if !strings.Contains(first.Body, ":/note2000000000000000000000000") {
			t.Errorf("first.Body missing note link: %q", first.Body)
		}
		if !strings.Contains(first.Body, ":/res0000000000000000000000000") {
			t.Errorf("first.Body missing resource link: %q", first.Body)
		}
		wantCreated, _ := time.Parse(time.RFC3339, "2023-01-05T00:00:00.000Z")
		if first.CreatedTime == nil || *first.CreatedTime != wantCreated.UnixMilli() {
			t.Errorf("first.CreatedTime = %v, want %d", first.CreatedTime, wantCreated.UnixMilli())
		}

		if len(second.TagRefs) != 0 {
			t.Errorf("second.TagRefs = %v, want empty", second.TagRefs)
		}
		if second.NotebookRef != "nb0000000000000000000000000000" {
			t.Errorf("second.NotebookRef = %q", second.NotebookRef)
		}

		if len(result.Resources) != 1 {
			t.Fatalf("got %d resources, want 1", len(result.Resources))
		}
		res := result.Resources[0]
		if res.ID != "res0000000000000000000000000" || res.Filename != "photo.png" || res.Mime != "image/png" {
			t.Errorf("resource = %+v", res)
		}
		if !bytes.Equal(res.Data, []byte{0x89, 0x50, 0x4e, 0x47}) {
			t.Errorf("resource data = %v", res.Data)
		}
	})

	t.Run("uses RootNotebookRef for a note with no parent_id", func(t *testing.T) {
		dir := t.TempDir()
		jexPath := filepath.Join(dir, "export.jex")
		noteMd := strings.Join([]string{
			"Orphan Note", "", "Body.", "",
			"id: orphan00000000000000000000000",
			"created_time: 2023-01-01T00:00:00.000Z",
			"updated_time: 2023-01-01T00:00:00.000Z",
			"type_: 1",
		}, "\n")
		buildTar(t, jexPath, map[string]string{"orphan.md": noteMd})

		result, err := ParseJexSource(jexPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Notes[0].NotebookRef != RootNotebookRef {
			t.Errorf("NotebookRef = %q, want %q", result.Notes[0].NotebookRef, RootNotebookRef)
		}
	})

	t.Run("rejects an archive with no recognizable note/resource entries", func(t *testing.T) {
		dir := t.TempDir()
		jexPath := filepath.Join(dir, "export.jex")
		buildTar(t, jexPath, map[string]string{"readme.txt": "not a joplin item"})

		_, err := ParseJexSource(jexPath)
		if err == nil || !strings.Contains(err.Error(), "empty or invalid JEX archive") {
			t.Fatalf("got error %v, want 'empty or invalid JEX archive'", err)
		}
	})

	t.Run("drops a resource metadata item with no matching binary rather than failing", func(t *testing.T) {
		dir := t.TempDir()
		jexPath := filepath.Join(dir, "export.jex")
		resourceMetaMd := strings.Join([]string{
			"ghost.png", "",
			"id: ghost000000000000000000000",
			"mime: image/png",
			"created_time: 2023-01-01T00:00:00.000Z",
			"updated_time: 2023-01-01T00:00:00.000Z",
			"type_: 4",
		}, "\n")
		buildTar(t, jexPath, map[string]string{"ghost.md": resourceMetaMd})

		result, err := ParseJexSource(jexPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Resources) != 0 {
			t.Errorf("resources = %+v, want empty", result.Resources)
		}
	})
}

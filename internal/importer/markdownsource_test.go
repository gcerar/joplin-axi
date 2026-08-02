package importer

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestParseMarkdownSourceSingleFile(t *testing.T) {
	t.Run("rejects a file with an unsupported extension", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.txt")
		if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := ParseMarkdownSource(file)
		if err == nil || !strings.Contains(err.Error(), "not a markdown file") {
			t.Fatalf("got error %v, want 'not a markdown file'", err)
		}
	})

	t.Run("extracts a title from a leading heading and strips it from the body", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		if err := os.WriteFile(file, []byte("# My Title\n\nSome body text."), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		result, err := ParseMarkdownSource(file)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Notes) != 1 {
			t.Fatalf("got %d notes, want 1", len(result.Notes))
		}
		if result.Notes[0].Title != "My Title" {
			t.Errorf("title = %q", result.Notes[0].Title)
		}
		if result.Notes[0].Body != "Some body text." {
			t.Errorf("body = %q", result.Notes[0].Body)
		}
		if result.Notes[0].NotebookRef != RootNotebookRef {
			t.Errorf("notebookRef = %q", result.Notes[0].NotebookRef)
		}
		if len(result.Notebooks) != 0 {
			t.Errorf("notebooks = %+v, want empty", result.Notebooks)
		}
	})

	t.Run("falls back to the filename when there is no heading or usable first line", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "my_note-file.md")
		if err := os.WriteFile(file, []byte("\n\n"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		result, err := ParseMarkdownSource(file)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Notes[0].Title != "my note file" {
			t.Errorf("title = %q, want %q", result.Notes[0].Title, "my note file")
		}
	})

	t.Run("reads frontmatter title/tags/notebook overrides", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		content := strings.Join([]string{"---", "title: Frontmatter Title", "tags: [work, urgent]", "notebook: Custom / Nested", "---", "Body content."}, "\n")
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		result, err := ParseMarkdownSource(file)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Notes[0].Title != "Frontmatter Title" {
			t.Errorf("title = %q", result.Notes[0].Title)
		}
		if !reflectEqualStrings(result.Notes[0].TagRefs, []string{"work", "urgent"}) {
			t.Errorf("tagRefs = %v", result.Notes[0].TagRefs)
		}
		if result.Notes[0].NotebookRef != "Custom/Nested" {
			t.Errorf("notebookRef = %q", result.Notes[0].NotebookRef)
		}

		var refs []string
		for _, nb := range result.Notebooks {
			refs = append(refs, nb.Ref)
		}
		if !reflectEqualStrings(refs, []string{"Custom", "Custom/Nested"}) {
			t.Errorf("notebook refs = %v, want [Custom Custom/Nested]", refs)
		}
		var nested *ParsedNotebook
		for i := range result.Notebooks {
			if result.Notebooks[i].Ref == "Custom/Nested" {
				nested = &result.Notebooks[i]
			}
		}
		if nested == nil || nested.ParentRef != "Custom" {
			t.Errorf("Custom/Nested.parentRef = %+v, want Custom", nested)
		}

		wantTags := []ParsedTag{{Ref: "work", Title: "work"}, {Ref: "urgent", Title: "urgent"}}
		if len(result.Tags) != 2 || result.Tags[0] != wantTags[0] || result.Tags[1] != wantTags[1] {
			t.Errorf("tags = %+v, want %+v", result.Tags, wantTags)
		}
	})

	t.Run("parses is_todo/todo_completed frontmatter as booleans", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		if err := os.WriteFile(file, []byte("---\ntodo: true\ncompleted: true\n---\nDo the thing"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		result, err := ParseMarkdownSource(file)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Notes[0].IsTodo {
			t.Errorf("isTodo = false, want true")
		}
		if !result.Notes[0].TodoCompleted {
			t.Errorf("todoCompleted = false, want true")
		}
	})
}

func TestParseMarkdownSourceDirectory(t *testing.T) {
	t.Run("derives notebook structure from subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdirAll(t, filepath.Join(dir, "Projects", "Alpha"))
		mustWriteFile(t, filepath.Join(dir, "root-note.md"), "# Root Note\n\nAt the top.")
		mustWriteFile(t, filepath.Join(dir, "Projects", "overview.md"), "# Overview\n\nProjects overview.")
		mustWriteFile(t, filepath.Join(dir, "Projects", "Alpha", "plan.md"), "# Plan\n\nAlpha plan.")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Notes) != 3 {
			t.Fatalf("got %d notes, want 3", len(result.Notes))
		}

		byTitle := map[string]ParsedNote{}
		for _, n := range result.Notes {
			byTitle[n.Title] = n
		}
		if byTitle["Root Note"].NotebookRef != RootNotebookRef {
			t.Errorf("Root Note notebookRef = %q", byTitle["Root Note"].NotebookRef)
		}
		if byTitle["Overview"].NotebookRef != "Projects" {
			t.Errorf("Overview notebookRef = %q", byTitle["Overview"].NotebookRef)
		}
		if byTitle["Plan"].NotebookRef != "Projects/Alpha" {
			t.Errorf("Plan notebookRef = %q", byTitle["Plan"].NotebookRef)
		}

		var refs []string
		for _, nb := range result.Notebooks {
			refs = append(refs, nb.Ref)
		}
		sort.Strings(refs)
		if !reflectEqualStrings(refs, []string{"Projects", "Projects/Alpha"}) {
			t.Errorf("refs = %v", refs)
		}

		var projects, alpha *ParsedNotebook
		for i := range result.Notebooks {
			switch result.Notebooks[i].Ref {
			case "Projects":
				projects = &result.Notebooks[i]
			case "Projects/Alpha":
				alpha = &result.Notebooks[i]
			}
		}
		if projects == nil || projects.ParentRef != "" {
			t.Errorf("Projects.parentRef = %+v, want empty", projects)
		}
		if alpha == nil || alpha.ParentRef != "Projects" {
			t.Errorf("Projects/Alpha.parentRef = %+v, want Projects", alpha)
		}
	})

	t.Run("ignores dotfiles/dot-directories and non-markdown files", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdirAll(t, filepath.Join(dir, ".git"))
		mustWriteFile(t, filepath.Join(dir, ".git", "config.md"), "# Should be ignored")
		mustWriteFile(t, filepath.Join(dir, "readme.txt"), "not markdown")
		mustWriteFile(t, filepath.Join(dir, "note.md"), "# Kept\n\nBody")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Notes) != 1 {
			t.Fatalf("got %d notes, want 1", len(result.Notes))
		}
		if result.Notes[0].Title != "Kept" {
			t.Errorf("title = %q", result.Notes[0].Title)
		}
	})

	t.Run("records the absolute source file path on each note", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "note.md"), "# Title\n\nBody")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want, _ := filepath.Abs(filepath.Join(dir, "note.md"))
		if result.Notes[0].SourceFilePath != want {
			t.Errorf("sourceFilePath = %q, want %q", result.Notes[0].SourceFilePath, want)
		}
	})
}

func TestParseMarkdownSourceLocalAssetLinks(t *testing.T) {
	t.Run("rewrites a relative link to a real sibling file into a :/<hash> token and registers it as a resource", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "diagram.png"), string([]byte{0x89, 0x50, 0x4e, 0x47}))
		mustWriteFile(t, filepath.Join(dir, "note.md"), "# Title\n\nSee ![diagram](./diagram.png) above.")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Resources) != 1 {
			t.Fatalf("got %d resources, want 1", len(result.Resources))
		}
		resource := result.Resources[0]
		if resource.Filename != "diagram.png" {
			t.Errorf("filename = %q", resource.Filename)
		}
		if resource.Mime != "image/png" {
			t.Errorf("mime = %q", resource.Mime)
		}
		if !bytes.Equal(resource.Data, []byte{0x89, 0x50, 0x4e, 0x47}) {
			t.Errorf("data = %v", resource.Data)
		}
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(resource.ID) {
			t.Errorf("id = %q, want a 32-char hex string", resource.ID)
		}
		wantBody := "See ![diagram](:/" + resource.ID + ") above."
		if result.Notes[0].Body != wantBody {
			t.Errorf("body = %q, want %q", result.Notes[0].Body, wantBody)
		}
	})

	t.Run("deduplicates a shared asset referenced by multiple notes into a single resource", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "logo.png"), string([]byte{1, 2, 3}))
		mustWriteFile(t, filepath.Join(dir, "a.md"), "# A\n\n![logo](./logo.png)")
		mustWriteFile(t, filepath.Join(dir, "b.md"), "# B\n\n![logo](./logo.png)")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Resources) != 1 {
			t.Fatalf("got %d resources, want 1", len(result.Resources))
		}
		id := result.Resources[0].ID

		var a, b *ParsedNote
		for i := range result.Notes {
			switch result.Notes[i].Title {
			case "A":
				a = &result.Notes[i]
			case "B":
				b = &result.Notes[i]
			}
		}
		if a == nil || a.Body != "![logo](:/"+id+")" {
			t.Errorf("a.Body = %+v", a)
		}
		if b == nil || b.Body != "![logo](:/"+id+")" {
			t.Errorf("b.Body = %+v", b)
		}
	})

	t.Run("leaves a link to another imported note untouched, for the importer to rewrite once note IDs exist", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "a.md"), "# A\n\nSee [B](./b.md).")
		mustWriteFile(t, filepath.Join(dir, "b.md"), "# B\n\nBody.")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var a *ParsedNote
		for i := range result.Notes {
			if result.Notes[i].Title == "A" {
				a = &result.Notes[i]
			}
		}
		if a == nil || a.Body != "See [B](./b.md)." {
			t.Errorf("a.Body = %+v", a)
		}
		if len(result.Resources) != 0 {
			t.Errorf("resources = %+v, want empty", result.Resources)
		}
	})

	t.Run("leaves a link to a nonexistent file untouched and does not register a resource", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "note.md"), "# Title\n\nSee [missing](./nope.png).")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Notes[0].Body != "See [missing](./nope.png)." {
			t.Errorf("body = %q", result.Notes[0].Body)
		}
		if len(result.Resources) != 0 {
			t.Errorf("resources = %+v, want empty", result.Resources)
		}
	})

	t.Run("leaves external http(s) links and existing :/ links untouched", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "note.md"), "# Title\n\n[site](https://example.com) and [existing](:/abc123).")

		result, err := ParseMarkdownSource(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "[site](https://example.com) and [existing](:/abc123)."
		if result.Notes[0].Body != want {
			t.Errorf("body = %q, want %q", result.Notes[0].Body, want)
		}
		if len(result.Resources) != 0 {
			t.Errorf("resources = %+v, want empty", result.Resources)
		}
	})
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func reflectEqualStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

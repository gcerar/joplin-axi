package importer

import (
	"reflect"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	t.Run("returns no frontmatter and the whole content as body when there is no leading --- block", func(t *testing.T) {
		result := ParseFrontmatter("# Just a note\n\nbody text", nil)
		if len(result.Fields) != 0 {
			t.Errorf("fields = %v, want empty", result.Fields)
		}
		if len(result.Lists) != 0 {
			t.Errorf("lists = %v, want empty", result.Lists)
		}
		if result.Body != "# Just a note\n\nbody text" {
			t.Errorf("body = %q", result.Body)
		}
	})

	t.Run("parses flat scalar fields", func(t *testing.T) {
		result := ParseFrontmatter("---\ntitle: Hello World\nnotebook: Work / Projects\n---\nBody here", nil)
		want := map[string]string{"title": "Hello World", "notebook": "Work / Projects"}
		if !reflect.DeepEqual(result.Fields, want) {
			t.Errorf("fields = %v, want %v", result.Fields, want)
		}
		if result.Body != "Body here" {
			t.Errorf("body = %q", result.Body)
		}
	})

	t.Run("strips surrounding quotes from scalar values", func(t *testing.T) {
		result := ParseFrontmatter("---\ntitle: \"Quoted Title\"\n---\nBody", nil)
		if result.Fields["title"] != "Quoted Title" {
			t.Errorf("title = %q, want %q", result.Fields["title"], "Quoted Title")
		}
	})

	t.Run("parses a bracket inline list for any key", func(t *testing.T) {
		result := ParseFrontmatter("---\ntags: [work, urgent]\n---\nBody", nil)
		want := []string{"work", "urgent"}
		if !reflect.DeepEqual(result.Lists["tags"], want) {
			t.Errorf("tags = %v, want %v", result.Lists["tags"], want)
		}
	})

	t.Run("parses a bare comma-separated list only for known list keys, not an arbitrary scalar", func(t *testing.T) {
		result := ParseFrontmatter("---\ntags: work, urgent\ntitle: Hello, World\n---\nBody", nil)
		want := []string{"work", "urgent"}
		if !reflect.DeepEqual(result.Lists["tags"], want) {
			t.Errorf("tags = %v, want %v", result.Lists["tags"], want)
		}
		// "title" isn't a list key, so its comma must NOT cause a split — the
		// regression case DefaultListKeys scoping exists for.
		if result.Fields["title"] != "Hello, World" {
			t.Errorf("title = %q, want %q", result.Fields["title"], "Hello, World")
		}
		if _, ok := result.Lists["title"]; ok {
			t.Errorf("lists[title] should be absent, got %v", result.Lists["title"])
		}
	})

	t.Run("parses a YAML block list", func(t *testing.T) {
		result := ParseFrontmatter("---\ntags:\n  - work\n  - urgent\n---\nBody", nil)
		want := []string{"work", "urgent"}
		if !reflect.DeepEqual(result.Lists["tags"], want) {
			t.Errorf("tags = %v, want %v", result.Lists["tags"], want)
		}
	})

	t.Run("leaves the body untouched if the closing delimiter is never found", func(t *testing.T) {
		content := "---\ntitle: Hello\nno closing delimiter here"
		result := ParseFrontmatter(content, nil)
		if len(result.Fields) != 0 {
			t.Errorf("fields = %v, want empty", result.Fields)
		}
		if result.Body != content {
			t.Errorf("body = %q, want %q", result.Body, content)
		}
	})

	t.Run("trims leading blank lines from the body after the closing delimiter", func(t *testing.T) {
		result := ParseFrontmatter("---\ntitle: Hello\n---\n\n\nActual body", nil)
		if result.Body != "Actual body" {
			t.Errorf("body = %q, want %q", result.Body, "Actual body")
		}
	})
}

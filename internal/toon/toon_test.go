package toon

import "testing"

func TestTable(t *testing.T) {
	t.Run("renders header with count and fields", func(t *testing.T) {
		got := Table("notes", []string{"id", "title"}, []map[string]any{{"id": "1", "title": "Fix bug"}})
		want := "notes[1]{id,title}:\n  1,Fix bug"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("renders an empty table with zero count", func(t *testing.T) {
		got := Table("notes", []string{"id", "title"}, nil)
		want := "notes[0]{id,title}:"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("quotes values containing commas or quotes", func(t *testing.T) {
		got := Table("notes", []string{"title"}, []map[string]any{{"title": `a, "b"`}})
		want := "notes[1]{title}:\n  \"a, \"\"b\"\"\""
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestScalar(t *testing.T) {
	t.Run("renders key: value", func(t *testing.T) {
		got := Scalar("count", "30 of 847 total")
		want := "count: 30 of 847 total"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestObject(t *testing.T) {
	t.Run("renders nested key: value lines in declared order", func(t *testing.T) {
		got := Object("note", []Field{{"id", "42"}, {"title", "Example"}})
		want := "note:\n  id: 42\n  title: Example"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestHelp(t *testing.T) {
	t.Run("renders numbered help block", func(t *testing.T) {
		got := Help([]string{"do this", "do that"})
		want := "help[2]:\n  do this\n  do that"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("renders nothing for an empty list", func(t *testing.T) {
		if got := Help(nil); got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestErrorOut(t *testing.T) {
	t.Run("renders error and help lines", func(t *testing.T) {
		got := ErrorOut("bad flag", []string{"try --help"})
		want := "error: bad flag\nhelp: try --help"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestTruncate(t *testing.T) {
	t.Run("passes short text through untouched", func(t *testing.T) {
		got := Truncate("short", 10)
		want := TruncateResult{Text: "short", Truncated: false, Total: 5}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("truncates long text and reports total length", func(t *testing.T) {
		got := Truncate("0123456789", 5)
		want := TruncateResult{Text: "01234", Truncated: true, Total: 10}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("counts runes, not bytes, so multi-byte text isn't cut mid-character", func(t *testing.T) {
		// "café" is 4 runes but 5 bytes (é is 2 bytes in UTF-8) — a
		// byte-length-based limit of 4 would either slice mid-character or
		// under-count; rune-based must see this as exactly 4, untruncated.
		got := Truncate("café", 4)
		want := TruncateResult{Text: "café", Truncated: false, Total: 4}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

func TestSections(t *testing.T) {
	t.Run("joins non-empty parts with blank lines and drops empty ones", func(t *testing.T) {
		got := Sections("a", "", "b")
		want := "a\n\nb"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

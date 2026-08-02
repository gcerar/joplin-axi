package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

// noOpImportClient stubs nothing at all — any accidental Joplin API call
// panics via the embedded nil client.Client, which is exactly the assertion
// a --dry-run test wants: zero writes, in fact zero client calls at all.
func noOpImportClient() *clienttest.StubClient {
	return &clienttest.StubClient{}
}

func writeImportFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestImportValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects a nonexistent path", func(t *testing.T) {
		dir := t.TempDir()
		parsed := args.ParsedArgs{
			Positionals: []string{filepath.Join(dir, "missing.md")},
			Flags:       map[string]any{"notebook": "nb1", "on-duplicate": "skip"},
		}
		_, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("requires --notebook for a real (non-dry-run) md import", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		writeImportFixture(t, file, "# Title\n\nBody")

		parsed := args.ParsedArgs{Positionals: []string{file}, Flags: map[string]any{"on-duplicate": "skip"}}
		_, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		var usageErr *args.UsageError
		if !errors.As(err, &usageErr) || !strings.Contains(usageErr.Message, "--notebook is required") {
			t.Fatalf("got error %v, want '--notebook is required'", err)
		}
	})

	t.Run("rejects an invalid --format value", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		writeImportFixture(t, file, "Body")

		parsed := args.ParsedArgs{Positionals: []string{file}, Flags: map[string]any{"notebook": "nb1", "format": "csv", "on-duplicate": "skip"}}
		_, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})

	t.Run("rejects an invalid --on-duplicate value", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "note.md")
		writeImportFixture(t, file, "Body")

		parsed := args.ParsedArgs{Positionals: []string{file}, Flags: map[string]any{"notebook": "nb1", "on-duplicate": "overwrite"}}
		_, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err == nil {
			t.Fatal("expected an error")
		}
		mustUsageError(t, err)
	})
}

func TestImportFormatAutoDetection(t *testing.T) {
	ctx := context.Background()

	t.Run("detects jex from the .jex extension", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "export.jex")
		writeImportFixture(t, file, "not a real tar, but we only need format detection + dry-run to short-circuit before parsing")

		parsed := args.ParsedArgs{Positionals: []string{file}, Flags: map[string]any{"dry-run": true, "on-duplicate": "skip"}}
		// Invalid tar content still fails during ParseJexSource — this test
		// only confirms .jex routes to the jex parser (it errors from
		// *there*, not from a "not a markdown file" error, which would
		// indicate md was picked).
		_, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err == nil || !strings.Contains(err.Error(), "JEX archive") {
			t.Fatalf("got error %v, want it to mention 'JEX archive'", err)
		}
	})

	t.Run("detects md for anything else, including a directory", func(t *testing.T) {
		dir := t.TempDir()
		writeImportFixture(t, filepath.Join(dir, "note.md"), "# Title\n\nBody")

		parsed := args.ParsedArgs{Positionals: []string{dir}, Flags: map[string]any{"dry-run": true, "on-duplicate": "skip"}}
		result, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "format: md") {
			t.Errorf("output does not contain 'format: md':\n%s", result.Output)
		}
	})
}

func TestImportDryRun(t *testing.T) {
	ctx := context.Background()

	t.Run("reports counts and makes zero Joplin API calls", func(t *testing.T) {
		dir := t.TempDir()
		writeImportFixture(t, filepath.Join(dir, "a.md"), "# A\n\nBody A")
		writeImportFixture(t, filepath.Join(dir, "b.md"), "---\ntags: [work]\n---\n# B\n\nBody B")

		parsed := args.ParsedArgs{Positionals: []string{dir}, Flags: map[string]any{"dry-run": true, "on-duplicate": "skip"}}
		result, err := ImportCommand.Run(ctx, parsed, noOpImportClient())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "format: md") {
			t.Errorf("output does not contain 'format: md':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "notes: 2") {
			t.Errorf("output does not contain 'notes: 2':\n%s", result.Output)
		}
		if !strings.Contains(result.Output, "dry run") {
			t.Errorf("output does not contain 'dry run':\n%s", result.Output)
		}
	})

	t.Run("does not require --notebook when --dry-run is set", func(t *testing.T) {
		dir := t.TempDir()
		writeImportFixture(t, filepath.Join(dir, "a.md"), "# A\n\nBody A")

		parsed := args.ParsedArgs{Positionals: []string{dir}, Flags: map[string]any{"dry-run": true, "on-duplicate": "skip"}}
		// Should not error despite no --notebook, since --dry-run never writes.
		if _, err := ImportCommand.Run(ctx, parsed, noOpImportClient()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

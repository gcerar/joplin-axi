package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
)

func TestSkillIsNoClient(t *testing.T) {
	if !SkillCommand.NoClient {
		t.Error("SkillCommand.NoClient = false, want true — it must not require JOPLIN_TOKEN")
	}
}

func TestSkillPrintsMarkdownByDefault(t *testing.T) {
	ctx := context.Background()

	result, err := SkillCommand.Run(ctx, args.ParsedArgs{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result.Output, "---\n") {
		t.Errorf("output does not start with frontmatter delimiter: %q", result.Output[:min(40, len(result.Output))])
	}
	if !strings.Contains(result.Output, "name: joplin-axi") {
		t.Errorf("output does not contain the skill's frontmatter name:\n%s", result.Output)
	}
}

func TestSkillWritesToOutputPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")

	parsed := args.ParsedArgs{Flags: map[string]any{"output": path}}
	result, err := SkillCommand.Run(ctx, parsed, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "written: "+path) {
		t.Errorf("output does not confirm the write path:\n%s", result.Output)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if !strings.HasPrefix(string(written), "---\n") {
		t.Errorf("written file does not start with frontmatter delimiter: %q", string(written)[:40])
	}
}

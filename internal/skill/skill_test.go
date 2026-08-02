package skill

import (
	"os"
	"testing"
)

func TestMarkdownMatchesCanonicalFile(t *testing.T) {
	canonical, err := os.ReadFile("../../skills/joplin-axi/SKILL.md")
	if err != nil {
		t.Fatalf("reading canonical skill file: %v", err)
	}
	if Markdown != string(canonical) {
		t.Error("embedded skill.md has drifted from skills/joplin-axi/SKILL.md — run `go generate ./...` to refresh it")
	}
}

func TestMarkdownLooksLikeTheSkillFile(t *testing.T) {
	if len(Markdown) == 0 {
		t.Fatal("Markdown is empty")
	}
	if Markdown[:4] != "---\n" {
		t.Errorf("Markdown does not start with frontmatter delimiter, got: %q", Markdown[:20])
	}
}

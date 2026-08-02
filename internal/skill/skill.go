// Package skill embeds this project's Agent Skill file
// (skills/joplin-axi/SKILL.md) into the binary, so a downloaded release
// doesn't need a separate repo clone just to get it — see the `skill`
// command in internal/commands.
//
// skill.md is a copy, not the canonical file itself: go:embed patterns
// can't reach outside their own package directory, and the canonical
// copy stays at the top-level skills/joplin-axi/SKILL.md path, which is
// the AXI-ecosystem convention for cross-agent distribution via
// `npx skills add`. Run `go generate ./...` after editing the canonical
// file to refresh this copy — skill_test.go diffs the two and fails if
// they've drifted, so a forgotten refresh is caught by `go test ./...`
// (already a CI step) rather than silently shipping stale skill text.
package skill

import _ "embed"

//go:generate cp ../../skills/joplin-axi/SKILL.md skill.md

//go:embed skill.md
var Markdown string

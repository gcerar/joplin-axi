package commands

import (
	"context"
	"os"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/skill"
	"github.com/gcerar/joplin-axi/internal/toon"
)

var skillSpec = args.CommandSpec{
	Name:    "skill",
	Summary: "Print or write this project's Agent Skill file (skills/joplin-axi/SKILL.md), embedded in the binary.",
	Usage:   "joplin-axi skill [--output <path>]",
	Flags: []args.FlagSpec{
		{Name: "output", Type: args.FlagString, Description: "Write the skill file to this path instead of printing it to stdout"},
	},
	Examples: []string{
		"joplin-axi skill > SKILL.md",
		"joplin-axi skill --output ./skills/joplin-axi/SKILL.md",
	},
}

func runSkill(_ context.Context, parsed args.ParsedArgs, _ client.Client) (CommandResult, error) {
	outputPath, hasOutput := parsed.StringFlag("output")
	if !hasOutput || outputPath == "" {
		// Raw content, not TOON — the point of this command is a
		// redirectable/pipeable, byte-for-byte copy of the skill file, not a
		// data report.
		return Ok(skill.Markdown), nil
	}

	if err := os.WriteFile(outputPath, []byte(skill.Markdown), 0644); err != nil { // #nosec G306 -- public documentation, not secret; group/other-readable is the intended, useful default here
		return CommandResult{}, err
	}
	return Ok(toon.Object("skill", []toon.Field{{Key: "written", Value: outputPath}})), nil
}

// SkillCommand is the top-level `skill` command. NoClient: true — it never
// touches Joplin, so it works without JOPLIN_TOKEN/JOPLIN_BASE_URL set.
var SkillCommand = Command{Spec: skillSpec, Run: runSkill, NoClient: true}

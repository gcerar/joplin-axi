package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/importer"
	"github.com/gcerar/joplin-axi/internal/toon"
)

var importSpec = args.CommandSpec{
	Name:    "import",
	Summary: "Import notes from a markdown file/directory or a Joplin .jex export.",
	Usage:   "joplin-axi import <path> [--notebook <id>] [--format md|jex] [--on-duplicate skip|rename] [--dry-run]",
	Flags: []args.FlagSpec{
		{Name: "notebook", Type: args.FlagString, Description: "Target notebook ID — required for md (no default target), an optional graft point for jex"},
		{Name: "format", Type: args.FlagString, Description: "Source format: md or jex (auto-detected from the path extension if omitted)"},
		{Name: "on-duplicate", Type: args.FlagString, Description: "skip (default) or rename a note whose title already exists in the target notebook", Default: "skip"},
		{Name: "dry-run", Type: args.FlagBoolean, Description: "Parse and report counts only — no Joplin writes", Default: false},
	},
	Examples: []string{
		"joplin-axi import ./notes --notebook a1b2c3",
		"joplin-axi import export.jex --notebook a1b2c3",
		"joplin-axi import export.jex --dry-run",
		"joplin-axi import ./notes --notebook a1b2c3 --on-duplicate rename",
	},
}

func detectImportFormat(sourcePath string) string {
	if strings.ToLower(filepath.Ext(sourcePath)) == ".jex" {
		return "jex"
	}
	return "md"
}

func runImportCommand(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	sourcePath, err := args.RequirePositional(parsed, 0, "path", importSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}
	notebook, _ := parsed.StringFlag("notebook")
	dryRun := parsed.BoolFlag("dry-run")

	// on-duplicate has a spec Default, so args.ParseArgs already fills it in
	// when the flag is absent — no extra fallback needed (and none should be
	// added: an explicitly-empty value must still fail validation below,
	// same as the TS source's `?? 'skip'`, which only substitutes for a
	// genuinely absent flag, not an empty string).
	onDuplicateRaw, _ := parsed.StringFlag("on-duplicate")
	if onDuplicateRaw != "skip" && onDuplicateRaw != "rename" {
		return CommandResult{}, &args.UsageError{
			Message:   fmt.Sprintf("--on-duplicate must be `skip` or `rename`, got `%s`", onDuplicateRaw),
			HelpLines: []string{importSpec.Usage},
		}
	}

	format, hasFormat := parsed.StringFlag("format")
	if !hasFormat {
		format = detectImportFormat(sourcePath)
	}
	if format != "md" && format != "jex" {
		return CommandResult{}, &args.UsageError{
			Message:   fmt.Sprintf("--format must be `md` or `jex`, got `%s`", format),
			HelpLines: []string{importSpec.Usage},
		}
	}

	if _, err := os.Stat(sourcePath); err != nil {
		return CommandResult{}, &args.UsageError{
			Message:   fmt.Sprintf("path does not exist: %s", sourcePath),
			HelpLines: []string{importSpec.Usage},
		}
	}

	// No default "Imported" target like the reference implementation —
	// importing without confirming a destination is exactly the kind of
	// surprise this project's safety conventions want to avoid. jex carries
	// its own notebook hierarchy, so --notebook there is just an optional
	// graft point. Only enforced for a real run — a --dry-run is read-only
	// and useful precisely for deciding on a target notebook before
	// committing to one.
	if format == "md" && notebook == "" && !dryRun {
		return CommandResult{}, &args.UsageError{
			Message:   "--notebook is required for md imports",
			HelpLines: []string{"run `joplin-axi notebooks list` to find a target, or `joplin-axi notebooks create <title>` to make one"},
		}
	}

	var parsedImport importer.ParsedImport
	if format == "jex" {
		parsedImport, err = importer.ParseJexSource(sourcePath)
	} else {
		parsedImport, err = importer.ParseMarkdownSource(sourcePath)
	}
	if err != nil {
		return CommandResult{}, err
	}

	if dryRun {
		counts := []toon.Field{
			{Key: "format", Value: format},
			{Key: "notebooks", Value: len(parsedImport.Notebooks)},
			{Key: "tags", Value: len(parsedImport.Tags)},
			{Key: "notes", Value: len(parsedImport.Notes)},
			{Key: "resources", Value: len(parsedImport.Resources)},
		}
		return Ok(toon.Sections(
			toon.Object("import", counts),
			toon.Help([]string{"This was a dry run — nothing was written to Joplin. Re-run without --dry-run to apply."}),
		)), nil
	}

	report, err := importer.Run(ctx, parsedImport, c, importer.RunOptions{TargetNotebookID: notebook, OnDuplicate: importer.OnDuplicate(onDuplicateRaw)})
	if err != nil {
		return CommandResult{}, err
	}

	out := toon.Sections(
		toon.Object("import", []toon.Field{
			{Key: "format", Value: format},
			{Key: "notebooks_created", Value: report.NotebooksCreated},
			{Key: "tags_created", Value: report.TagsCreated},
			{Key: "notes_created", Value: report.NotesCreated},
			{Key: "notes_skipped", Value: len(report.NotesSkipped)},
			{Key: "notes_failed", Value: len(report.NotesFailed)},
			{Key: "link_rewrite_failed", Value: len(report.LinkRewriteFailed)},
			{Key: "resources_uploaded", Value: report.ResourcesUploaded},
			{Key: "unresolved_links", Value: report.UnresolvedLinks},
		}),
		toon.Help(importHints(report)),
	)

	if len(report.NotesFailed) > 0 || len(report.LinkRewriteFailed) > 0 {
		return Failed(out), nil
	}
	return Ok(out), nil
}

func importHints(report importer.ImportReport) []string {
	var hints []string
	if report.NotebooksCreated > 0 {
		hints = append(hints, "Run `joplin-axi notebooks list` to see the new notebook structure.")
	}
	if len(report.NotesFailed) > 0 {
		hints = append(hints, fmt.Sprintf("%d note(s) failed — see notes_failed above; nothing else in the batch was rolled back.", len(report.NotesFailed)))
	}
	if len(report.LinkRewriteFailed) > 0 {
		hints = append(hints, fmt.Sprintf("%d note(s) were created but their links couldn't be rewritten (see link_rewrite_failed above) — the note bodies may still contain stale :/id references.", len(report.LinkRewriteFailed)))
	}
	if report.UnresolvedLinks > 0 {
		hints = append(hints, fmt.Sprintf("%d link(s) in imported notes couldn't be resolved (see the design notes for why).", report.UnresolvedLinks))
	}
	return hints
}

// ImportCommand is the top-level `import` command.
var ImportCommand = Command{Spec: importSpec, Run: runImportCommand}

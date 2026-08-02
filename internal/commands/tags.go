package commands

import (
	"context"
	"fmt"
	"sync"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/mapfield"
	"github.com/gcerar/joplin-axi/internal/scope"
	"github.com/gcerar/joplin-axi/internal/toon"
)

// target is the shared {id, title} shape resolveTargets produces, whether
// from an explicit --notes list or a scope filter.
type target struct {
	ID    string
	Title string
}

func targetRows(targets []target) []map[string]any {
	rows := make([]map[string]any, len(targets))
	for i, t := range targets {
		rows[i] = map[string]any{"id": t.ID, "title": t.Title}
	}
	return rows
}

func failedTable(failed []FailedItem[target]) string {
	if len(failed) == 0 {
		return ""
	}
	rows := make([]map[string]any, len(failed))
	for i, f := range failed {
		rows[i] = map[string]any{"id": f.Item.ID, "title": f.Item.Title, "error": f.Error}
	}
	return toon.Table("failed", []string{"id", "title", "error"}, rows)
}

// resolveTargets picks notes either by an explicit --notes ID list, or by
// the same --query/--notebook/--task filters as notes list (intersected via
// scope.ResolveNoteScope). Deliberately no confirmation gate: mutating
// immediately and reporting every affected note is the AXI-consistent
// trade-off (a gate would double round-trips even in the common correct
// case; a wrong filter is visible immediately in the report and one more
// `tags remove` call away from being undone).
//
// An explicit --notes ID that doesn't resolve (typo, deleted note) is
// reported as a failure alongside any mutation failures rather than
// aborting the whole batch — one bad ID in a long --notes list shouldn't
// block tagging the rest.
func resolveTargets(ctx context.Context, parsed args.ParsedArgs, c client.Client, usage string) ([]target, []FailedItem[target], error) {
	explicitNotes, hasNotes := parsed.StringFlag("notes")
	query, _ := parsed.StringFlag("query")
	notebook, _ := parsed.StringFlag("notebook")
	task := parsed.BoolFlag("task")
	hasFilters := query != "" || notebook != "" || task

	if hasNotes && hasFilters {
		return nil, nil, &args.UsageError{
			Message:   "--notes cannot be combined with --query/--notebook/--task",
			HelpLines: []string{"use --notes for an explicit ID list, or filters to select notes by criteria — not both"},
		}
	}
	if !hasNotes && !hasFilters {
		return nil, nil, &args.UsageError{Message: "requires --notes or at least one of --query/--notebook/--task", HelpLines: []string{usage}}
	}

	if hasNotes {
		ids := args.SplitList(explicitNotes)
		notes := make([]map[string]any, len(ids))
		errs := make([]error, len(ids))
		var wg sync.WaitGroup
		wg.Add(len(ids))
		for i, id := range ids {
			go func(i int, id string) {
				defer wg.Done()
				notes[i], errs[i] = c.GetNote(ctx, id, []string{"id", "title"})
			}(i, id)
		}
		wg.Wait()

		var targets []target
		var failed []FailedItem[target]
		for i, id := range ids {
			if errs[i] != nil {
				failed = append(failed, FailedItem[target]{Item: target{ID: id}, Error: errs[i].Error()})
				continue
			}
			targets = append(targets, target{ID: mapfield.String(notes[i], "id"), Title: mapfield.String(notes[i], "title")})
		}
		return targets, failed, nil
	}

	notes, err := scope.ResolveNoteScope(ctx, c, scope.Options{NotebookID: notebook, Query: query, Task: task, Fields: []string{"id", "title", "updated_time"}})
	if err != nil {
		return nil, nil, err
	}
	targets := make([]target, len(notes))
	for i, n := range notes {
		targets[i] = target{ID: mapfield.String(n, "id"), Title: mapfield.String(n, "title")}
	}
	return targets, nil, nil
}

var selectionFlags = []args.FlagSpec{
	{Name: "notes", Type: args.FlagString, Description: "Comma-separated note IDs (mutually exclusive with the filters below)"},
	{Name: "query", Type: args.FlagString, Description: "Only notes matching this full-text search"},
	{Name: "notebook", Type: args.FlagString, Description: "Only notes in this notebook ID"},
	{Name: "task", Type: args.FlagBoolean, Description: "Only to-do notes", Default: false},
}

var tagsListSpec = args.CommandSpec{
	Name:     "tags list",
	Summary:  "List all tags.",
	Usage:    "joplin-axi tags list",
	Examples: []string{"joplin-axi tags list"},
}

func runTagsList(ctx context.Context, _ args.ParsedArgs, c client.Client) (CommandResult, error) {
	items, err := c.ListTags(ctx, nil)
	if err != nil {
		return CommandResult{}, err
	}
	body := "tags: 0 tags found"
	if len(items) > 0 {
		body = toon.Table("tags", []string{"id", "title"}, items)
	}
	return Ok(toon.Sections(body, toon.Help([]string{"Run `joplin-axi notes list --tag <title>` to see notes with a tag."}))), nil
}

var tagsOfSpec = args.CommandSpec{
	Name:     "tags of",
	Summary:  "List tags applied to a note.",
	Usage:    "joplin-axi tags of <note-id>",
	Examples: []string{"joplin-axi tags of 3f9c2a1b"},
}

func runTagsOf(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	noteID, err := args.RequirePositional(parsed, 0, "note-id", tagsOfSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	items, err := c.GetTagsByNote(ctx, noteID, nil)
	if err != nil {
		return CommandResult{}, err
	}

	body := fmt.Sprintf("tags: 0 tags on note %s", noteID)
	if len(items) > 0 {
		body = toon.Table("tags", []string{"id", "title"}, items)
	}
	return Ok(toon.Sections(body, toon.Help([]string{fmt.Sprintf("Run `joplin-axi tags add <title> --notes %s` to add a tag.", noteID)}))), nil
}

var tagsCreateSpec = args.CommandSpec{
	Name:     "tags create",
	Summary:  "Create a new tag.",
	Usage:    "joplin-axi tags create <title>",
	Examples: []string{"joplin-axi tags create urgent"},
}

func runTagsCreate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	title, err := args.RequirePositional(parsed, 0, "title", tagsCreateSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	tag, err := c.CreateTag(ctx, title)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("tag", []toon.Field{{Key: "id", Value: tag["id"]}, {Key: "title", Value: tag["title"]}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi tags add %s --notes <id[,id...]>` to apply it.", title)}),
	)), nil
}

var tagsUpdateSpec = args.CommandSpec{
	Name:     "tags update",
	Summary:  "Rename a tag.",
	Usage:    "joplin-axi tags update <id> <title>",
	Examples: []string{"joplin-axi tags update 9876c25986874f4ea588fff1b3ff9c1b renamed-tag"},
}

func runTagsUpdate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	if len(parsed.Positionals) < 2 || parsed.Positionals[0] == "" || parsed.Positionals[1] == "" {
		return CommandResult{}, &args.UsageError{Message: "requires <id> and <title>", HelpLines: []string{tagsUpdateSpec.Usage}}
	}
	id, title := parsed.Positionals[0], parsed.Positionals[1]

	tag, err := c.UpdateTag(ctx, id, title)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("tag", []toon.Field{{Key: "id", Value: mapfield.StringOr(tag, "id", id)}, {Key: "title", Value: tag["title"]}}),
		toon.Help([]string{"Run `joplin-axi tags list` to confirm the rename."}),
	)), nil
}

var tagsDeleteSpec = args.CommandSpec{
	Name: "tags delete",
	Summary: "Delete a tag. Unlike notes/notebooks, Joplin has no trash for tags — this is immediate and only " +
		"removes the tag and its note associations, not the notes themselves.",
	Usage:    "joplin-axi tags delete <id>",
	Examples: []string{"joplin-axi tags delete 9876c25986874f4ea588fff1b3ff9c1b"},
}

func runTagsDelete(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", tagsDeleteSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	if err := c.DeleteTag(ctx, id); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("tag", []toon.Field{{Key: "id", Value: id}, {Key: "deleted", Value: true}}),
		toon.Help([]string{"Run `joplin-axi tags list` to confirm."}),
	)), nil
}

var tagsAddSpec = args.CommandSpec{
	Name:    "tags add",
	Summary: "Apply a tag to notes — by explicit --notes IDs, or by --query/--notebook/--task filters (every match, intersected).",
	Usage:   "joplin-axi tags add <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)",
	Flags:   selectionFlags,
	Examples: []string{
		"joplin-axi tags add active --notes 3f9c2a1b",
		"joplin-axi tags add active --notes 3f9c2a1b,6f6a6757",
		`joplin-axi tags add active --notebook a1b2c3 --query "annual report"`,
	},
}

func runTagsAdd(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	title, err := args.RequirePositional(parsed, 0, "tag-title", tagsAddSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	tagID, err := scope.ResolveTagID(ctx, c, title)
	if err != nil {
		return CommandResult{}, err
	}

	targets, resolveFailed, err := resolveTargets(ctx, parsed, c, tagsAddSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	succeeded, applyFailed := ApplyToEach(targets, func(t target) error {
		return c.AddTagToNote(ctx, tagID, t.ID)
	})
	failed := append(resolveFailed, applyFailed...)

	body := "notes: 0 notes matched — nothing tagged"
	if len(succeeded) > 0 {
		body = toon.Table("notes", []string{"id", "title"}, targetRows(succeeded))
	}

	var hints []string
	if len(succeeded) > 0 {
		hints = append(hints, fmt.Sprintf("Run `joplin-axi notes list --tag %s` to see all notes with this tag.", title))
	}
	if len(failed) > 0 {
		hints = append(hints, fmt.Sprintf("%d note(s) failed — retry with `joplin-axi tags add %s --notes <id[,id...]>` for just those.", len(failed), title))
	}
	if len(succeeded) == 0 && len(failed) == 0 {
		hints = append(hints, scope.CheckScopesHint)
	}

	output := toon.Sections(
		toon.Object("tag", []toon.Field{{Key: "title", Value: title}, {Key: "added_to", Value: len(succeeded)}, {Key: "failed", Value: len(failed)}}),
		body, failedTable(failed), toon.Help(hints),
	)
	if len(failed) > 0 {
		return Failed(output), nil
	}
	return Ok(output), nil
}

var tagsRemoveSpec = args.CommandSpec{
	Name:     "tags remove",
	Summary:  "Remove a tag from notes — by explicit --notes IDs, or by --query/--notebook/--task filters (every match, intersected).",
	Usage:    "joplin-axi tags remove <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)",
	Flags:    selectionFlags,
	Examples: []string{"joplin-axi tags remove active --notes 3f9c2a1b", "joplin-axi tags remove active --notebook a1b2c3"},
}

func runTagsRemove(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	title, err := args.RequirePositional(parsed, 0, "tag-title", tagsRemoveSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	tagID, err := scope.ResolveTagID(ctx, c, title)
	if err != nil {
		return CommandResult{}, err
	}

	targets, resolveFailed, err := resolveTargets(ctx, parsed, c, tagsRemoveSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	succeeded, applyFailed := ApplyToEach(targets, func(t target) error {
		return c.RemoveTagFromNote(ctx, tagID, t.ID)
	})
	failed := append(resolveFailed, applyFailed...)

	body := "notes: 0 notes matched — nothing removed"
	if len(succeeded) > 0 {
		body = toon.Table("notes", []string{"id", "title"}, targetRows(succeeded))
	}

	var hints []string
	if len(succeeded) > 0 {
		hints = append(hints, fmt.Sprintf("Run `joplin-axi tags add %s --notes <id[,id...]>` to undo, if needed.", title))
	}
	if len(failed) > 0 {
		hints = append(hints, fmt.Sprintf("%d note(s) failed — retry with `joplin-axi tags remove %s --notes <id[,id...]>` for just those.", len(failed), title))
	}
	if len(succeeded) == 0 && len(failed) == 0 {
		hints = append(hints, scope.CheckScopesHint)
	}

	output := toon.Sections(
		toon.Object("tag", []toon.Field{{Key: "title", Value: title}, {Key: "removed_from", Value: len(succeeded)}, {Key: "failed", Value: len(failed)}}),
		body, failedTable(failed), toon.Help(hints),
	)
	if len(failed) > 0 {
		return Failed(output), nil
	}
	return Ok(output), nil
}

// TagsCommands is the tags <command> group.
var TagsCommands = map[string]Command{
	"list":   {Spec: tagsListSpec, Run: runTagsList},
	"of":     {Spec: tagsOfSpec, Run: runTagsOf},
	"create": {Spec: tagsCreateSpec, Run: runTagsCreate},
	"update": {Spec: tagsUpdateSpec, Run: runTagsUpdate},
	"delete": {Spec: tagsDeleteSpec, Run: runTagsDelete},
	"add":    {Spec: tagsAddSpec, Run: runTagsAdd},
	"remove": {Spec: tagsRemoveSpec, Run: runTagsRemove},
}

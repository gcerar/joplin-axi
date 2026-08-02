package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/toon"
)

// iconPayload is the format joplin-mcp itself uses for the Folder.icon field
// (undocumented in the REST API reference beyond "text") — a JSON string
// {"type":1,"emoji":"...","name":""}. A struct (not a map) guarantees field
// order in the marshaled output matches this exactly — Go's encoding/json
// marshals struct fields in declaration order, but map keys alphabetically.
type iconPayload struct {
	Type  int    `json:"type"`
	Emoji string `json:"emoji"`
	Name  string `json:"name"`
}

func encodeIcon(emoji string) (string, error) {
	b, err := json.Marshal(iconPayload{Type: 1, Emoji: emoji, Name: ""})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var notebooksListSpec = args.CommandSpec{
	Name:    "notebooks list",
	Summary: "List all notebooks.",
	Usage:   "joplin-axi notebooks list [--parent <id>]",
	Flags: []args.FlagSpec{
		{Name: "parent", Type: args.FlagString, Description: "Only notebooks directly under this parent ID (pass an empty string for top-level only)"},
	},
	Examples: []string{
		"joplin-axi notebooks list",
		"joplin-axi notebooks list --parent c8a068acf54642a9b50f7f5a45195e2a",
		`joplin-axi notebooks list --parent ""`,
	},
}

func runNotebooksList(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	parent, hasParent := parsed.StringFlag("parent")

	allItems, err := c.ListNotebooks(ctx, nil)
	if err != nil {
		return CommandResult{}, err
	}

	items := allItems
	if hasParent {
		items = make([]map[string]any, 0, len(allItems))
		for _, n := range allItems {
			if fieldString(n, "parent_id") == parent {
				items = append(items, n)
			}
		}
	}

	var body string
	if len(items) > 0 {
		body = toon.Table("notebooks", []string{"id", "title", "parent_id"}, items)
	} else {
		body = "notebooks: 0 notebooks found"
		if hasParent {
			label := parent
			if label == "" {
				label = "(top-level)"
			}
			body += fmt.Sprintf(" under parent `%s`", label)
		}
	}

	return Ok(toon.Sections(body, toon.Help([]string{"Run `joplin-axi notes list --notebook <id>` to see notes in a notebook."}))), nil
}

var notebooksCreateSpec = args.CommandSpec{
	Name:    "notebooks create",
	Summary: "Create a new notebook.",
	Usage:   "joplin-axi notebooks create <title> [--parent <id>] [--icon <emoji>]",
	Flags: []args.FlagSpec{
		{Name: "parent", Type: args.FlagString, Description: "Parent notebook ID (omit for top-level)"},
		{Name: "icon", Type: args.FlagString, Description: "Emoji icon for the notebook"},
	},
	Examples: []string{
		`joplin-axi notebooks create "Side project" --icon 🚀`,
		"joplin-axi notebooks create \"Sub notebook\" --parent c8a068acf54642a9b50f7f5a45195e2a",
	},
}

func runNotebooksCreate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	title, err := args.RequirePositional(parsed, 0, "title", notebooksCreateSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	fields := map[string]any{"title": title}
	if parent, ok := parsed.StringFlag("parent"); ok {
		fields["parent_id"] = parent
	}
	if icon, ok := parsed.StringFlag("icon"); ok {
		encoded, err := encodeIcon(icon)
		if err != nil {
			return CommandResult{}, err
		}
		fields["icon"] = encoded
	}

	notebook, err := c.CreateNotebook(ctx, fields)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("notebook", []toon.Field{{Key: "id", Value: notebook["id"]}, {Key: "title", Value: notebook["title"]}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes list --notebook %v` to see notes in it.", notebook["id"])}),
	)), nil
}

var notebooksUpdateSpec = args.CommandSpec{
	Name:    "notebooks update",
	Summary: "Rename, re-icon, or move a notebook under another parent.",
	Usage:   "joplin-axi notebooks update <id> [--title <text>] [--icon <emoji>] [--parent <id>]",
	Flags: []args.FlagSpec{
		{Name: "title", Type: args.FlagString, Description: "New title"},
		{Name: "icon", Type: args.FlagString, Description: "New emoji icon"},
		{Name: "parent", Type: args.FlagString, Description: "Move under this parent notebook ID"},
	},
	Examples: []string{`joplin-axi notebooks update c8a068acf54642a9b50f7f5a45195e2a --title "Renamed"`},
}

func runNotebooksUpdate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	const usage = "joplin-axi notebooks update <id> [--title] [--icon] [--parent]"
	id, err := args.RequirePositional(parsed, 0, "id", usage)
	if err != nil {
		return CommandResult{}, err
	}

	fields := map[string]any{}
	if title, ok := parsed.StringFlag("title"); ok {
		fields["title"] = title
	}
	if icon, ok := parsed.StringFlag("icon"); ok {
		encoded, err := encodeIcon(icon)
		if err != nil {
			return CommandResult{}, err
		}
		fields["icon"] = encoded
	}
	if parent, ok := parsed.StringFlag("parent"); ok {
		fields["parent_id"] = parent
	}

	if len(fields) == 0 {
		return CommandResult{}, &args.UsageError{
			Message:   "nothing to update — pass at least one of --title/--icon/--parent",
			HelpLines: []string{usage},
		}
	}

	notebook, err := c.UpdateNotebook(ctx, id, fields)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("notebook", []toon.Field{
			{Key: "id", Value: fieldStringOr(notebook, "id", id)},
			{Key: "title", Value: notebook["title"]},
			{Key: "updated", Value: "ok"},
		}),
		toon.Help([]string{"Run `joplin-axi notebooks list` to confirm the change."}),
	)), nil
}

var notebooksDeleteSpec = args.CommandSpec{
	Name:     "notebooks delete",
	Summary:  "Move a notebook (and its notes) to Joplin's trash. Always a soft delete — joplin-axi never permanently deletes.",
	Usage:    "joplin-axi notebooks delete <id>",
	Examples: []string{"joplin-axi notebooks delete c8a068acf54642a9b50f7f5a45195e2a"},
}

func runNotebooksDelete(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", notebooksDeleteSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	if err := c.DeleteNotebook(ctx, id); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("notebook", []toon.Field{{Key: "id", Value: id}, {Key: "trashed", Value: true}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notebooks restore %s` to undo.", id)}),
	)), nil
}

var notebooksRestoreSpec = args.CommandSpec{
	Name: "notebooks restore",
	Summary: "Restore a notebook from Joplin's trash. Only restores this one notebook — sub-notebooks and the notes " +
		"inside it stay trashed and must be restored individually.",
	Usage:    "joplin-axi notebooks restore <id>",
	Examples: []string{"joplin-axi notebooks restore c8a068acf54642a9b50f7f5a45195e2a"},
}

func runNotebooksRestore(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", notebooksRestoreSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	if err := c.RestoreNotebook(ctx, id); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("notebook", []toon.Field{{Key: "id", Value: id}, {Key: "restored", Value: true}}),
		toon.Help([]string{
			"Run `joplin-axi notebooks list` to confirm.",
			"Sub-notebooks and notes inside stay trashed — restore each individually (`notes list --trash` to find them).",
		}),
	)), nil
}

// NotebooksCommands is the notebooks <command> group.
var NotebooksCommands = map[string]Command{
	"list":    {Spec: notebooksListSpec, Run: runNotebooksList},
	"create":  {Spec: notebooksCreateSpec, Run: runNotebooksCreate},
	"update":  {Spec: notebooksUpdateSpec, Run: runNotebooksUpdate},
	"delete":  {Spec: notebooksDeleteSpec, Run: runNotebooksDelete},
	"restore": {Spec: notebooksRestoreSpec, Run: runNotebooksRestore},
}

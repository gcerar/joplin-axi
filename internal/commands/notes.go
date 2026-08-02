package commands

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/mapfield"
	"github.com/gcerar/joplin-axi/internal/scope"
	"github.com/gcerar/joplin-axi/internal/toon"
)

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// replaceFirst does a plain literal replacement of the first occurrence
// only — deliberately not strings.Replace(haystack, needle, replacement, 1)
// applied to... actually strings.Replace *is* literal in Go (unlike JS's
// String.replace, which treats $&/$$/$`/$' in a *string* replacement
// argument as special patterns even when the search value is a plain
// string, not a regex). Go has no such gotcha, but this stays a named
// helper anyway, matching the TS structure 1:1 and keeping the "why not
// String.replace" reasoning attached to the one place it matters.
func replaceFirst(haystack, needle, replacement string) string {
	idx := strings.Index(haystack, needle)
	return haystack[:idx] + replacement + haystack[idx+len(needle):]
}

// ── notes list ───────────────────────────────────────────────────────────────

var notesDefaultListFields = []string{"id", "title", "notebook", "updated"}
var notesAvailableListFields = []string{"id", "title", "notebook", "updated", "created", "is_todo", "deleted"}

var notesListSpec = args.CommandSpec{
	Name:    "notes list",
	Summary: "List or search notes. --query/--notebook/--tag/--task can all be combined (intersected).",
	Usage:   "joplin-axi notes list [--query <text>] [--notebook <id>] [--tag <title>] [--task] [--trash] [--limit <n>] [--fields <list>]",
	Flags: []args.FlagSpec{
		{Name: "query", Type: args.FlagString, Description: "Free-text search (Joplin search syntax); combinable with --notebook/--tag/--task"},
		{Name: "notebook", Type: args.FlagString, Description: "Restrict to a notebook ID; combinable with --query/--tag/--task"},
		{Name: "tag", Type: args.FlagString, Description: "Restrict to a tag title; combinable with --query/--notebook/--task"},
		{Name: "task", Type: args.FlagBoolean, Description: "Restrict to to-do notes; combinable with any other filter", Default: false},
		{Name: "trash", Type: args.FlagBoolean, Description: "List only trashed notes (cannot combine with --query/--task/--notebook/--tag)", Default: false},
		{Name: "limit", Type: args.FlagNumber, Description: "Max notes to return", Default: float64(20)},
		{Name: "fields", Type: args.FlagString, Description: fmt.Sprintf("Comma-separated output fields (%s)", strings.Join(notesAvailableListFields, ","))},
	},
	Examples: []string{
		"joplin-axi notes list --notebook a1b2c3",
		`joplin-axi notes list --query "annual report"`,
		"joplin-axi notes list --task --limit 50",
		`joplin-axi notes list --notebook a1b2c3 --query "meeting" --task`,
		`joplin-axi notes list --tag active --query "report"`,
		"joplin-axi notes list --trash",
	},
}

func runNotesList(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	query, _ := parsed.StringFlag("query")
	notebook, _ := parsed.StringFlag("notebook")
	tag, _ := parsed.StringFlag("tag")
	task := parsed.BoolFlag("task")
	trash := parsed.BoolFlag("trash")
	limit, _ := parsed.NumberFlag("limit")
	if limit <= 0 {
		return CommandResult{}, &args.UsageError{Message: "--limit must be a positive number", HelpLines: []string{notesListSpec.Usage}}
	}
	limitInt := int(limit)

	fieldsRaw, hasFields := parsed.StringFlag("fields")
	var fieldList []string
	if !hasFields || fieldsRaw == "" {
		fieldList = append([]string{}, notesDefaultListFields...)
		if trash {
			fieldList = append(fieldList, "deleted")
		}
	} else {
		fieldList = args.SplitList(fieldsRaw)
	}
	for _, f := range fieldList {
		if !contains(notesAvailableListFields, f) {
			return CommandResult{}, &args.UsageError{
				Message:   fmt.Sprintf("unknown field `%s` for `notes list`", f),
				HelpLines: []string{fmt.Sprintf("valid fields: %s", strings.Join(notesAvailableListFields, ", "))},
			}
		}
	}

	if trash && (query != "" || task || notebook != "" || tag != "") {
		return CommandResult{}, &args.UsageError{
			Message:   "--trash cannot be combined with --query/--task/--notebook/--tag",
			HelpLines: []string{"Joplin only documents include_deleted for the unfiltered /notes listing (not /search, /folders, or /tags)"},
		}
	}

	var tagID string
	if tag != "" {
		var err error
		tagID, err = scope.ResolveTagID(ctx, c, tag)
		if err != nil {
			return CommandResult{}, err
		}
	}

	apiFields := scope.NewSet("id", "title", "parent_id", "updated_time")
	if contains(fieldList, "created") {
		apiFields["created_time"] = struct{}{}
	}
	if contains(fieldList, "is_todo") || task {
		apiFields["is_todo"] = struct{}{}
	}
	if trash || contains(fieldList, "deleted") {
		apiFields["deleted_time"] = struct{}{}
	}
	apiFieldsArr := make([]string, 0, len(apiFields))
	for f := range apiFields {
		apiFieldsArr = append(apiFieldsArr, f)
	}

	searchCap := limitInt * 20
	if searchCap < 500 {
		searchCap = 500
	}

	var items []map[string]any
	switch {
	case trash:
		// include_deleted mixes trashed notes into the normal result set
		// rather than listing only them, so fetch a larger raw batch and
		// filter/slice client-side to approximate "list only trashed notes".
		rawItems, err := c.ListNotes(ctx, client.ListNotesOptions{Fields: apiFieldsArr, Limit: searchCap, IncludeDeleted: true})
		if err != nil {
			return CommandResult{}, err
		}
		for _, n := range rawItems {
			if mapfield.Int64(n, "deleted_time") > 0 {
				items = append(items, n)
				if len(items) >= limitInt {
					break
				}
			}
		}
	case notebook != "" || tagID != "" || query != "" || task:
		// See internal/scope for why intersecting ID-scoped sets is safe
		// where interpolating a notebook/tag title into the search DSL isn't.
		scoped, err := scope.ResolveNoteScope(ctx, c, scope.Options{
			NotebookID: notebook, TagID: tagID, Query: query, Task: task, Fields: apiFieldsArr, SearchCap: searchCap,
		})
		if err != nil {
			return CommandResult{}, err
		}
		if len(scoped) > limitInt {
			scoped = scoped[:limitInt]
		}
		items = scoped
	default:
		var err error
		items, err = c.ListNotes(ctx, client.ListNotesOptions{Fields: apiFieldsArr, Limit: limitInt})
		if err != nil {
			return CommandResult{}, err
		}
	}

	notebookNames := map[string]string{}
	if contains(fieldList, "notebook") && len(items) > 0 {
		nbs, err := c.ListNotebooks(ctx, nil)
		if err != nil {
			return CommandResult{}, err
		}
		for _, n := range nbs {
			notebookNames[mapfield.String(n, "id")] = mapfield.String(n, "title")
		}
	}

	rows := make([]map[string]any, len(items))
	for i, n := range items {
		parentID := mapfield.String(n, "parent_id")
		notebookName, ok := notebookNames[parentID]
		if !ok {
			notebookName = parentID
		}
		isTodo := "no"
		if mapfield.Bool(n, "is_todo") {
			isTodo = "yes"
		}
		rows[i] = map[string]any{
			"id":       n["id"],
			"title":    n["title"],
			"notebook": notebookName,
			"updated":  toon.FmtTime(mapfield.Int64(n, "updated_time")),
			"created":  toon.FmtTime(mapfield.Int64(n, "created_time")),
			"is_todo":  isTodo,
			"deleted":  toon.FmtTime(mapfield.Int64(n, "deleted_time")),
		}
	}

	var scopeParts []string
	if trash {
		scopeParts = append(scopeParts, "trashed")
	}
	if task {
		scopeParts = append(scopeParts, "to-do")
	}
	scopeDescription := strings.Join(scopeParts, " ")

	var contextParts []string
	if query != "" {
		contextParts = append(contextParts, fmt.Sprintf("query `%s`", query))
	}
	if notebook != "" {
		contextParts = append(contextParts, fmt.Sprintf("notebook `%s`", notebook))
	}
	if tag != "" {
		contextParts = append(contextParts, fmt.Sprintf("tag `%s`", tag))
	}
	context := ""
	if len(contextParts) > 0 {
		context = " for " + strings.Join(contextParts, ", ")
	}

	var body string
	if len(rows) > 0 {
		body = toon.Table("notes", fieldList, rows)
	} else {
		scopePrefix := ""
		if scopeDescription != "" {
			scopePrefix = scopeDescription + " "
		}
		body = fmt.Sprintf("notes: 0 %snotes found%s", scopePrefix, context)
	}

	var hints []string
	if len(rows) > 0 {
		hints = append(hints, "Run `joplin-axi notes get <id>` for the full note.")
		if len(rows) >= limitInt {
			hints = append(hints, fmt.Sprintf("Run with --limit %d to see more.", limitInt*2))
		}
	} else {
		hints = append(hints, scope.CheckScopesHint)
	}

	return Ok(toon.Sections(body, toon.Help(hints))), nil
}

// ── notes get ────────────────────────────────────────────────────────────────

var notesDefaultGetFields = []string{"id", "title", "notebook", "updated", "created", "is_todo", "body"}

var notesGetSpec = args.CommandSpec{
	Name:    "notes get",
	Summary: "Fetch a single note by ID.",
	Usage:   "joplin-axi notes get <id> [--full] [--fields <list>]",
	Flags: []args.FlagSpec{
		{Name: "full", Type: args.FlagBoolean, Description: "Show the full body instead of a truncated preview", Default: false},
		{Name: "fields", Type: args.FlagString, Description: fmt.Sprintf("Comma-separated output fields (%s)", strings.Join(notesDefaultGetFields, ","))},
	},
	Examples: []string{"joplin-axi notes get 3f9c2a1b", "joplin-axi notes get 3f9c2a1b --full"},
}

func runNotesGet(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", "joplin-axi notes get <id>")
	if err != nil {
		return CommandResult{}, err
	}

	fieldsRaw, hasFields := parsed.StringFlag("fields")
	fieldList := notesDefaultGetFields
	if hasFields && fieldsRaw != "" {
		fieldList = args.SplitList(fieldsRaw)
	}
	for _, f := range fieldList {
		if !contains(notesDefaultGetFields, f) {
			return CommandResult{}, &args.UsageError{
				Message:   fmt.Sprintf("unknown field `%s` for `notes get`", f),
				HelpLines: []string{fmt.Sprintf("valid fields: %s", strings.Join(notesDefaultGetFields, ", "))},
			}
		}
	}

	// Only fetch the API fields actually needed for the requested display
	// fields — mirrors notes list's apiFields pattern instead of always
	// pulling the full (potentially large) body regardless of --fields.
	apiFields := scope.NewSet[string]()
	if contains(fieldList, "id") {
		apiFields["id"] = struct{}{}
	}
	if contains(fieldList, "title") {
		apiFields["title"] = struct{}{}
	}
	if contains(fieldList, "notebook") {
		apiFields["parent_id"] = struct{}{}
	}
	if contains(fieldList, "updated") {
		apiFields["updated_time"] = struct{}{}
	}
	if contains(fieldList, "created") {
		apiFields["created_time"] = struct{}{}
	}
	if contains(fieldList, "is_todo") {
		apiFields["is_todo"] = struct{}{}
	}
	if contains(fieldList, "body") {
		apiFields["body"] = struct{}{}
	}
	apiFieldsArr := make([]string, 0, len(apiFields))
	for f := range apiFields {
		apiFieldsArr = append(apiFieldsArr, f)
	}

	note, err := c.GetNote(ctx, id, apiFieldsArr)
	if err != nil {
		return CommandResult{}, err
	}

	notebookName := mapfield.String(note, "parent_id")
	if contains(fieldList, "notebook") {
		notebooks, err := c.ListNotebooks(ctx, nil)
		if err != nil {
			return CommandResult{}, err
		}
		for _, n := range notebooks {
			if mapfield.String(n, "id") == notebookName {
				notebookName = mapfield.String(n, "title")
				break
			}
		}
	}

	full := parsed.BoolFlag("full")
	bodyText := mapfield.String(note, "body")
	shownBody, truncated, total := bodyText, false, len([]rune(bodyText))
	if !full {
		r := toon.Truncate(bodyText, 800)
		shownBody, truncated, total = r.Text, r.Truncated, r.Total
	}

	var out []toon.Field
	if contains(fieldList, "id") {
		out = append(out, toon.Field{Key: "id", Value: note["id"]})
	}
	if contains(fieldList, "title") {
		out = append(out, toon.Field{Key: "title", Value: note["title"]})
	}
	if contains(fieldList, "notebook") {
		out = append(out, toon.Field{Key: "notebook", Value: notebookName})
	}
	if contains(fieldList, "updated") {
		out = append(out, toon.Field{Key: "updated", Value: toon.FmtTime(mapfield.Int64(note, "updated_time"))})
	}
	if contains(fieldList, "created") {
		out = append(out, toon.Field{Key: "created", Value: toon.FmtTime(mapfield.Int64(note, "created_time"))})
	}
	if contains(fieldList, "is_todo") {
		isTodo := "no"
		if mapfield.Bool(note, "is_todo") {
			isTodo = "yes"
		}
		out = append(out, toon.Field{Key: "is_todo", Value: isTodo})
	}
	if contains(fieldList, "body") {
		out = append(out, toon.Field{Key: "body", Value: shownBody})
	}

	parts := []string{toon.Object("note", out)}
	if truncated {
		parts = append(parts, toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes get %s --full` to see the complete body (%d chars total).", id, total)}))
	}
	return Ok(toon.Sections(parts...)), nil
}

// ── notes find-in ────────────────────────────────────────────────────────────

const notesFindInContextLimit = 120

var notesFindInSpec = args.CommandSpec{
	Name:    "notes find-in",
	Summary: "Regex search within a single note's body (line-based, with context).",
	Usage:   "joplin-axi notes find-in <id> <pattern> [--ignore-case] [--limit <n>]",
	Flags: []args.FlagSpec{
		{Name: "ignore-case", Type: args.FlagBoolean, Description: "Case-insensitive match", Default: false},
		{Name: "limit", Type: args.FlagNumber, Description: "Max matches to return", Default: float64(20)},
	},
	Examples: []string{`joplin-axi notes find-in 3f9c2a1b "TODO:.*"`},
}

func runNotesFindIn(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	if len(parsed.Positionals) < 2 || parsed.Positionals[0] == "" || parsed.Positionals[1] == "" {
		return CommandResult{}, &args.UsageError{
			Message:   "requires <id> and <pattern>",
			HelpLines: []string{"joplin-axi notes find-in <id> <pattern>"},
		}
	}
	id, pattern := parsed.Positionals[0], parsed.Positionals[1]

	limit, _ := parsed.NumberFlag("limit")
	if limit <= 0 {
		return CommandResult{}, &args.UsageError{Message: "--limit must be a positive number", HelpLines: []string{notesFindInSpec.Usage}}
	}

	rePattern := pattern
	if parsed.BoolFlag("ignore-case") {
		rePattern = "(?i)" + pattern
	}
	// Go's RE2 engine (no backreferences/lookaround) rather than JS's regex
	// syntax — an accepted, documented difference; a compile error on an
	// unsupported pattern is a clear enough failure mode for this diagnostic
	// command, not worth a validation layer of its own.
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return CommandResult{}, &args.UsageError{Message: fmt.Sprintf("invalid regex: %s", err.Error())}
	}

	note, err := c.GetNote(ctx, id, []string{"id", "body"})
	if err != nil {
		return CommandResult{}, err
	}
	lines := strings.Split(mapfield.String(note, "body"), "\n")

	limitInt := int(limit)
	anyContextTruncated := false
	var rows []map[string]any
	for i := 0; i < len(lines) && len(rows) < limitInt; i++ {
		loc := re.FindStringIndex(lines[i])
		if loc == nil {
			continue
		}
		r := toon.Truncate(strings.TrimSpace(lines[i]), notesFindInContextLimit)
		if r.Truncated {
			anyContextTruncated = true
		}
		rows = append(rows, map[string]any{"line": i + 1, "match": lines[i][loc[0]:loc[1]], "context": r.Text})
	}

	var body string
	if len(rows) > 0 {
		body = toon.Table("matches", []string{"line", "match", "context"}, rows)
	} else {
		body = fmt.Sprintf("matches: 0 matches for /%s/ in note %s", pattern, id)
	}

	var hints []string
	if len(rows) > 0 {
		if anyContextTruncated {
			hints = append(hints, fmt.Sprintf("Some context lines were truncated; run `joplin-axi notes get %s --full` to see complete lines.", id))
		} else {
			hints = append(hints, fmt.Sprintf("Run `joplin-axi notes get %s --full` to see these matches in full context.", id))
		}
	}

	return Ok(toon.Sections(body, toon.Help(hints))), nil
}

// ── notes links ──────────────────────────────────────────────────────────────

var noteLinkRE = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
var internalTargetRE = regexp.MustCompile(`(?i)^:/[0-9a-f]{32}`)

var notesLinksSpec = args.CommandSpec{
	Name:     "notes links",
	Summary:  "Extract markdown links from a note's body.",
	Usage:    "joplin-axi notes links <id>",
	Examples: []string{"joplin-axi notes links 3f9c2a1b"},
}

func runNotesLinks(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", "joplin-axi notes links <id>")
	if err != nil {
		return CommandResult{}, err
	}

	note, err := c.GetNote(ctx, id, []string{"id", "body"})
	if err != nil {
		return CommandResult{}, err
	}

	matches := noteLinkRE.FindAllStringSubmatch(mapfield.String(note, "body"), -1)
	rows := make([]map[string]any, 0, len(matches))
	hasInternalLink := false
	for _, m := range matches {
		text, target := m[1], m[2]
		linkType := "external"
		if internalTargetRE.MatchString(target) {
			linkType = "note"
			hasInternalLink = true
		}
		rows = append(rows, map[string]any{"text": text, "target": target, "type": linkType})
	}

	var body string
	if len(rows) > 0 {
		body = toon.Table("links", []string{"text", "target", "type"}, rows)
	} else {
		body = fmt.Sprintf("links: 0 links found in note %s", id)
	}

	var hints []string
	if hasInternalLink {
		hints = append(hints, "Run `joplin-axi notes get <id>` (strip the leading `:/` from an internal link's target) to view a linked note.")
	}

	return Ok(toon.Sections(body, toon.Help(hints))), nil
}

// ── notes resources ──────────────────────────────────────────────────────────

var notesDefaultResourceFields = []string{"id", "title", "mime", "size"}
var notesAvailableResourceFields = []string{"id", "title", "mime", "size", "ocr_text"}

const notesOCRPreviewLimit = 500

var notesResourcesSpec = args.CommandSpec{
	Name:    "notes resources",
	Summary: "List a note's attached resources (images, PDFs, attachments).",
	Usage:   "joplin-axi notes resources <id> [--fields <list>] [--full]",
	Flags: []args.FlagSpec{
		{Name: "fields", Type: args.FlagString, Description: fmt.Sprintf("Comma-separated output fields (%s)", strings.Join(notesAvailableResourceFields, ","))},
		{Name: "full", Type: args.FlagBoolean, Description: "Show complete OCR text instead of a truncated preview", Default: false},
	},
	Examples: []string{
		"joplin-axi notes resources 3f9c2a1b",
		"joplin-axi notes resources 3f9c2a1b --fields id,title,ocr_text --full",
	},
}

func runNotesResources(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", "joplin-axi notes resources <id>")
	if err != nil {
		return CommandResult{}, err
	}

	fieldsRaw, hasFields := parsed.StringFlag("fields")
	fieldList := notesDefaultResourceFields
	if hasFields && fieldsRaw != "" {
		fieldList = args.SplitList(fieldsRaw)
	}
	for _, f := range fieldList {
		if !contains(notesAvailableResourceFields, f) {
			return CommandResult{}, &args.UsageError{
				Message:   fmt.Sprintf("unknown field `%s` for `notes resources`", f),
				HelpLines: []string{fmt.Sprintf("valid fields: %s", strings.Join(notesAvailableResourceFields, ", "))},
			}
		}
	}

	// Only request ocr_text from the API when it's actually going to be
	// shown — it can be large, and previously was always fetched regardless
	// of --fields.
	resources, err := c.GetNoteResources(ctx, id, fieldList)
	if err != nil {
		return CommandResult{}, err
	}

	full := parsed.BoolFlag("full")
	anyTruncated := false
	rows := make([]map[string]any, len(resources))
	for i, r := range resources {
		ocrText := ""
		if contains(fieldList, "ocr_text") {
			if raw := mapfield.String(r, "ocr_text"); raw != "" {
				if full {
					ocrText = raw
				} else {
					preview := toon.Truncate(raw, notesOCRPreviewLimit)
					ocrText = preview.Text
					if preview.Truncated {
						anyTruncated = true
					}
				}
			}
		}
		rows[i] = map[string]any{"id": r["id"], "title": r["title"], "mime": r["mime"], "size": r["size"], "ocr_text": ocrText}
	}

	var body string
	if len(rows) > 0 {
		body = toon.Table("resources", fieldList, rows)
	} else {
		body = fmt.Sprintf("resources: 0 resources attached to note %s", id)
	}

	var hints []string
	if anyTruncated {
		hints = append(hints, fmt.Sprintf("Run `joplin-axi notes resources %s --full` to see complete OCR text.", id))
	}

	return Ok(toon.Sections(body, toon.Help(hints))), nil
}

// ── notes create ─────────────────────────────────────────────────────────────

var notesCreateSpec = args.CommandSpec{
	Name:    "notes create",
	Summary: "Create a new note.",
	Usage:   "joplin-axi notes create --title <text> [--body <text>] [--notebook <id>]",
	Flags: []args.FlagSpec{
		{Name: "title", Type: args.FlagString, Description: "Note title (required)"},
		{Name: "body", Type: args.FlagString, Description: "Note body (Markdown)"},
		{Name: "notebook", Type: args.FlagString, Description: "Notebook ID (omit for Joplin's default notebook)"},
	},
	Examples: []string{
		`joplin-axi notes create --title "Meeting notes" --body "# Agenda"`,
		`joplin-axi notes create --title "Quick capture" --notebook b3b3d60013f04ccf8ad373cf7b2fc4d1`,
	},
}

func runNotesCreate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	title, hasTitle := parsed.StringFlag("title")
	if !hasTitle || title == "" {
		return CommandResult{}, &args.UsageError{
			Message:   "--title is required",
			HelpLines: []string{"joplin-axi notes create --title <text> [--body <text>] [--notebook <id>]"},
		}
	}

	fields := map[string]any{"title": title}
	if body, ok := parsed.StringFlag("body"); ok {
		fields["body"] = body
	}
	if notebook, ok := parsed.StringFlag("notebook"); ok {
		fields["parent_id"] = notebook
	}

	note, err := c.CreateNote(ctx, fields)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("note", []toon.Field{{Key: "id", Value: note["id"]}, {Key: "title", Value: note["title"]}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes get %v` to see the full note.", note["id"])}),
	)), nil
}

// ── notes update ─────────────────────────────────────────────────────────────

var notesUpdateSpec = args.CommandSpec{
	Name:    "notes update",
	Summary: "Update a note's title, body, and/or notebook (moves the note if --notebook is given).",
	Usage:   "joplin-axi notes update <id> [--title <text>] [--body <text>] [--notebook <id>]",
	Flags: []args.FlagSpec{
		{Name: "title", Type: args.FlagString, Description: "New title"},
		{Name: "body", Type: args.FlagString, Description: "New body (Markdown) — replaces the entire body"},
		{Name: "notebook", Type: args.FlagString, Description: "Move the note to this notebook ID"},
	},
	Examples: []string{
		`joplin-axi notes update 3f9c2a1b --title "Renamed"`,
		"joplin-axi notes update 3f9c2a1b --notebook 810dc26ff91b4133b7bc13532a9c3bdd",
	},
}

func runNotesUpdate(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	const usage = "joplin-axi notes update <id> [--title] [--body] [--notebook]"
	id, err := args.RequirePositional(parsed, 0, "id", usage)
	if err != nil {
		return CommandResult{}, err
	}

	fields := map[string]any{}
	if title, ok := parsed.StringFlag("title"); ok {
		fields["title"] = title
	}
	if body, ok := parsed.StringFlag("body"); ok {
		fields["body"] = body
	}
	if notebook, ok := parsed.StringFlag("notebook"); ok {
		fields["parent_id"] = notebook
	}

	if len(fields) == 0 {
		return CommandResult{}, &args.UsageError{
			Message:   "nothing to update — pass at least one of --title/--body/--notebook",
			HelpLines: []string{usage},
		}
	}

	note, err := c.UpdateNote(ctx, id, fields)
	if err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("note", []toon.Field{
			{Key: "id", Value: mapfield.StringOr(note, "id", id)},
			{Key: "title", Value: note["title"]},
			{Key: "updated", Value: "ok"},
		}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes get %s` to see the updated note.", id)}),
	)), nil
}

// ── notes edit ───────────────────────────────────────────────────────────────

var notesEditSpec = args.CommandSpec{
	Name:    "notes edit",
	Summary: "Precision-edit a note's body: find/replace, append, or prepend.",
	Usage:   "joplin-axi notes edit <id> [--find <text> --replace <text>] [--append <text>] [--prepend <text>] [--all]",
	Flags: []args.FlagSpec{
		{Name: "find", Type: args.FlagString, Description: "Exact text to find (used with --replace)"},
		{Name: "replace", Type: args.FlagString, Description: "Replacement text (used with --find)"},
		{Name: "append", Type: args.FlagString, Description: "Text to add to the end of the body"},
		{Name: "prepend", Type: args.FlagString, Description: "Text to add to the start of the body"},
		{Name: "all", Type: args.FlagBoolean, Description: "With --find/--replace, replace all occurrences instead of just the first", Default: false},
	},
	Examples: []string{
		`joplin-axi notes edit 3f9c2a1b --find "TODO" --replace "DONE"`,
		`joplin-axi notes edit 3f9c2a1b --append "\n\n## Update\nDone."`,
	},
}

func runNotesEdit(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", "joplin-axi notes edit <id> ...")
	if err != nil {
		return CommandResult{}, err
	}

	find, hasFind := parsed.StringFlag("find")
	replace, hasReplace := parsed.StringFlag("replace")
	appendText, hasAppend := parsed.StringFlag("append")
	prepend, hasPrepend := parsed.StringFlag("prepend")
	all := parsed.BoolFlag("all")

	modesUsed := 0
	if hasFind {
		modesUsed++
	}
	if hasAppend {
		modesUsed++
	}
	if hasPrepend {
		modesUsed++
	}

	if modesUsed == 0 {
		return CommandResult{}, &args.UsageError{
			Message:   "requires one of --find/--replace, --append, or --prepend",
			HelpLines: []string{"joplin-axi notes edit <id> [--find <text> --replace <text>] [--append <text>] [--prepend <text>]"},
		}
	}
	if modesUsed > 1 {
		return CommandResult{}, &args.UsageError{Message: "--find/--replace, --append, and --prepend are mutually exclusive", HelpLines: []string{"pass exactly one edit mode per call"}}
	}
	if hasFind && !hasReplace {
		return CommandResult{}, &args.UsageError{Message: "--find requires --replace", HelpLines: []string{"joplin-axi notes edit <id> --find <text> --replace <text>"}}
	}
	if hasFind && find == "" {
		return CommandResult{}, &args.UsageError{
			Message:   "--find must not be empty",
			HelpLines: []string{"an empty search string matches everywhere and would corrupt the body"},
		}
	}
	if hasReplace && !hasFind {
		return CommandResult{}, &args.UsageError{Message: "--replace requires --find", HelpLines: []string{"joplin-axi notes edit <id> --find <text> --replace <text>"}}
	}
	if all && !hasFind {
		return CommandResult{}, &args.UsageError{
			Message:   "--all only applies with --find/--replace",
			HelpLines: []string{"joplin-axi notes edit <id> --find <text> --replace <text> --all"},
		}
	}

	note, err := c.GetNote(ctx, id, []string{"id", "body"})
	if err != nil {
		return CommandResult{}, err
	}
	body := mapfield.String(note, "body")

	var newBody string
	switch {
	case hasFind:
		if !strings.Contains(body, find) {
			return CommandResult{}, &args.UsageError{
				Message:   fmt.Sprintf("text not found in note %s: %q", id, find),
				HelpLines: []string{"run `joplin-axi notes get " + id + "` to check the current body"},
			}
		}
		if all {
			newBody = strings.ReplaceAll(body, find, replace)
		} else {
			newBody = replaceFirst(body, find, replace)
		}
	case hasAppend:
		newBody = body + appendText
	default:
		newBody = prepend + body
	}

	if _, err := c.UpdateNote(ctx, id, map[string]any{"body": newBody}); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("note", []toon.Field{{Key: "id", Value: id}, {Key: "edited", Value: true}, {Key: "length", Value: len([]rune(newBody))}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes get %s --full` to see the edited body.", id)}),
	)), nil
}

// ── notes delete ─────────────────────────────────────────────────────────────

var notesDeleteSpec = args.CommandSpec{
	Name:     "notes delete",
	Summary:  "Move a note to Joplin's trash. Always a soft delete — joplin-axi never permanently deletes.",
	Usage:    "joplin-axi notes delete <id>",
	Examples: []string{"joplin-axi notes delete 3f9c2a1b"},
}

func runNotesDelete(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", notesDeleteSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	if err := c.DeleteNote(ctx, id); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("note", []toon.Field{{Key: "id", Value: id}, {Key: "trashed", Value: true}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes restore %s` to undo.", id)}),
	)), nil
}

// ── notes restore ────────────────────────────────────────────────────────────

var notesRestoreSpec = args.CommandSpec{
	Name:     "notes restore",
	Summary:  "Restore a note from Joplin's trash.",
	Usage:    "joplin-axi notes restore <id>",
	Examples: []string{"joplin-axi notes restore 3f9c2a1b"},
}

func runNotesRestore(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error) {
	id, err := args.RequirePositional(parsed, 0, "id", notesRestoreSpec.Usage)
	if err != nil {
		return CommandResult{}, err
	}

	if err := c.RestoreNote(ctx, id); err != nil {
		return CommandResult{}, err
	}

	return Ok(toon.Sections(
		toon.Object("note", []toon.Field{{Key: "id", Value: id}, {Key: "restored", Value: true}}),
		toon.Help([]string{fmt.Sprintf("Run `joplin-axi notes get %s` to confirm.", id)}),
	)), nil
}

// NotesCommands is the notes <command> group.
var NotesCommands = map[string]Command{
	"list":      {Spec: notesListSpec, Run: runNotesList},
	"get":       {Spec: notesGetSpec, Run: runNotesGet},
	"find-in":   {Spec: notesFindInSpec, Run: runNotesFindIn},
	"links":     {Spec: notesLinksSpec, Run: runNotesLinks},
	"resources": {Spec: notesResourcesSpec, Run: runNotesResources},
	"create":    {Spec: notesCreateSpec, Run: runNotesCreate},
	"update":    {Spec: notesUpdateSpec, Run: runNotesUpdate},
	"edit":      {Spec: notesEditSpec, Run: runNotesEdit},
	"delete":    {Spec: notesDeleteSpec, Run: runNotesDelete},
	"restore":   {Spec: notesRestoreSpec, Run: runNotesRestore},
}

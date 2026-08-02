package importer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/mapfield"
)

// RunOptions mirrors the TS RunImportOptions shape. TargetNotebookID is
// required for markdown sources (no default target); an optional graft
// point for JEX sources (whose notebook hierarchy is self-contained in the
// archive) — "" means none.
type RunOptions struct {
	TargetNotebookID string
	OnDuplicate      OnDuplicate
}

var resourceLinkRE = regexp.MustCompile(`:/([0-9a-f]{32})`)

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func generateUniqueTitle(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for n := 1; n <= 1000; n++ {
		candidate := fmt.Sprintf("%s (%d)", base, n)
		if !taken[candidate] {
			return candidate
		}
	}
	// Astronomically unlikely (1000 same-titled notes in one notebook) —
	// fall back to something guaranteed unique rather than looping forever.
	return fmt.Sprintf("%s (%d)", base, time.Now().UnixMilli())
}

func noteExistsAtPath(notes []ParsedNote, path string) bool {
	for _, n := range notes {
		if n.SourceFilePath == path {
			return true
		}
	}
	return false
}

// Run applies a ParsedImport (from ParseMarkdownSource or ParseJexSource) to
// a real Joplin instance. Two passes: notes are created first with their
// original body text, then a second pass re-fetches each created note and
// rewrites `:/resourceId`/`:/noteId` tokens (and, for markdown sources,
// relative `.md` links) to the newly-assigned IDs — this has to be a
// separate pass because a note can link to another note that doesn't exist
// yet at creation time. Resources are uploaded lazily, once per unique ID,
// the first time a reference to them is actually encountered during
// rewriting.
func Run(ctx context.Context, parsed ParsedImport, c client.Client, opts RunOptions) (ImportReport, error) {
	report := ImportReport{}

	// --- Notebooks: resolve top-down, memoized by ref, so a nested chain of
	// arbitrary depth only ever creates each notebook once regardless of the
	// order notebooks appear in the slice.
	notebooksByRef := map[string]ParsedNotebook{}
	for _, nb := range parsed.Notebooks {
		notebooksByRef[nb.Ref] = nb
	}
	notebookIDByRef := map[string]string{}
	// Refs currently being resolved, further up the same call chain — lets a
	// cyclic ParentRef (a corrupted/hand-crafted JEX where A's parent is B
	// and B's parent is A) be detected and broken instead of recursing
	// forever: notebookIDByRef is only populated *after* a resolution
	// completes, so without this a cycle would never hit the "already
	// resolved" memo either.
	resolvingRefs := map[string]bool{}

	var resolveNotebookID func(ref string) (string, error)
	resolveNotebookID = func(ref string) (string, error) {
		if ref == RootNotebookRef {
			return opts.TargetNotebookID, nil
		}
		if id, ok := notebookIDByRef[ref]; ok {
			return id, nil
		}
		nb, ok := notebooksByRef[ref]
		if !ok {
			return "", nil // parser invariant violated — treat as "no notebook" rather than erroring mid-batch.
		}
		if resolvingRefs[ref] {
			return "", nil // cyclic parentRef — treat the cycle-closing edge as "no parent".
		}

		resolvingRefs[ref] = true
		defer delete(resolvingRefs, ref)

		parentID := opts.TargetNotebookID
		if nb.ParentRef != "" {
			var err error
			parentID, err = resolveNotebookID(nb.ParentRef)
			if err != nil {
				return "", err
			}
		}

		fields := map[string]any{"title": nb.Title}
		if parentID != "" {
			fields["parent_id"] = parentID
		}
		created, err := c.CreateNotebook(ctx, fields)
		if err != nil {
			return "", err
		}
		id := mapfield.String(created, "id")
		notebookIDByRef[ref] = id
		report.NotebooksCreated++
		return id, nil
	}

	for _, nb := range parsed.Notebooks {
		if _, err := resolveNotebookID(nb.Ref); err != nil {
			return report, err
		}
	}

	// --- Tags: existing tags fetched once (not per tag — avoids both an N+1
	// round trip and a check-then-create race across concurrent lookups of
	// the same not-yet-existing title), then create-if-missing per unique
	// title.
	existingTags, err := c.ListTags(ctx, nil)
	if err != nil {
		return report, err
	}
	existingTagIDByTitle := map[string]string{}
	for _, t := range existingTags {
		existingTagIDByTitle[mapfield.String(t, "title")] = mapfield.String(t, "id")
	}
	tagIDByRef := map[string]string{}
	for _, tag := range parsed.Tags {
		if existingID, ok := existingTagIDByTitle[tag.Title]; ok {
			tagIDByRef[tag.Ref] = existingID
			continue
		}
		created, err := c.CreateTag(ctx, tag.Title)
		if err != nil {
			return report, err
		}
		id := mapfield.String(created, "id")
		tagIDByRef[tag.Ref] = id
		existingTagIDByTitle[tag.Title] = id
		report.TagsCreated++
	}

	// --- Resources: uploaded lazily during the link-rewrite pass, not here —
	// no point uploading one that never actually gets referenced by a
	// created note (e.g. because that note was skipped as a duplicate).
	resourcesByID := map[string]ParsedResource{}
	for _, r := range parsed.Resources {
		resourcesByID[r.ID] = r
	}
	resourceIDByOldID := map[string]string{}
	knownSourceIDs := map[string]bool{}
	for _, n := range parsed.Notes {
		if n.SourceID != "" {
			knownSourceIDs[n.SourceID] = true
		}
	}

	// --- Existing-title cache per notebook, for dedup — populated lazily
	// (only for notebooks an import actually targets), fetched by ID never
	// by title-in-a-search-query, consistent with internal/scope's
	// anti-injection reasoning.
	existingTitlesByNotebook := map[string]map[string]bool{}
	existingTitles := func(notebookID string) (map[string]bool, error) {
		if set, ok := existingTitlesByNotebook[notebookID]; ok {
			return set, nil
		}
		set := map[string]bool{}
		if notebookID != "" {
			items, err := c.ListNotes(ctx, client.ListNotesOptions{NotebookID: notebookID, Fields: []string{"id", "title"}, Limit: client.Unlimited})
			if err != nil {
				return nil, err
			}
			for _, n := range items {
				set[mapfield.String(n, "title")] = true
			}
		}
		existingTitlesByNotebook[notebookID] = set
		return set, nil
	}

	// --- Pass 1: create notes with their original body, unrewritten.
	noteIDBySourceID := map[string]string{}
	noteIDByFilePath := map[string]string{}

	for _, note := range parsed.Notes {
		notebookID, err := resolveNotebookID(note.NotebookRef)
		if err != nil {
			report.NotesFailed = append(report.NotesFailed, TitledError{Title: note.Title, Error: err.Error()})
			continue
		}
		taken, err := existingTitles(notebookID)
		if err != nil {
			report.NotesFailed = append(report.NotesFailed, TitledError{Title: note.Title, Error: err.Error()})
			continue
		}

		title := note.Title
		if taken[title] {
			if opts.OnDuplicate == OnDuplicateSkip {
				report.NotesSkipped = append(report.NotesSkipped, TitledReason{Title: title, Reason: "duplicate title in target notebook"})
				continue
			}
			title = generateUniqueTitle(title, taken)
		}

		fields := map[string]any{
			"title":          title,
			"body":           note.Body,
			"is_todo":        boolToInt(note.IsTodo),
			"todo_completed": boolToInt(note.TodoCompleted),
		}
		if notebookID != "" {
			fields["parent_id"] = notebookID
		}

		created, err := c.CreateNote(ctx, fields)
		if err != nil {
			report.NotesFailed = append(report.NotesFailed, TitledError{Title: note.Title, Error: err.Error()})
			continue
		}
		// Only mark the title as taken once creation actually succeeded —
		// doing this before the call would make a same-titled note later in
		// the batch look like a real duplicate even if this one failed and
		// doesn't exist.
		taken[title] = true
		createdID := mapfield.String(created, "id")

		timeFields := map[string]any{}
		if note.CreatedTime != nil {
			timeFields["created_time"] = *note.CreatedTime
		}
		if note.UpdatedTime != nil {
			timeFields["updated_time"] = *note.UpdatedTime
		}
		if len(timeFields) > 0 {
			if _, err := c.UpdateNote(ctx, createdID, timeFields); err != nil {
				report.NotesFailed = append(report.NotesFailed, TitledError{Title: note.Title, Error: err.Error()})
				continue
			}
		}

		tagFailed := false
		for _, tagRef := range note.TagRefs {
			tagID, ok := tagIDByRef[tagRef]
			if !ok {
				continue
			}
			if err := c.AddTagToNote(ctx, tagID, createdID); err != nil {
				report.NotesFailed = append(report.NotesFailed, TitledError{Title: note.Title, Error: err.Error()})
				tagFailed = true
				break
			}
		}
		if tagFailed {
			continue
		}

		report.NotesCreated++
		if note.SourceID != "" {
			noteIDBySourceID[note.SourceID] = createdID
		}
		if note.SourceFilePath != "" {
			noteIDByFilePath[note.SourceFilePath] = createdID
		}
	}

	// --- Pass 2: rewrite links now that every note in the batch has an ID.
	uploadResourceOnce := func(oldID string) (string, error) {
		if id, ok := resourceIDByOldID[oldID]; ok {
			return id, nil
		}
		res, ok := resourcesByID[oldID]
		if !ok {
			return "", nil
		}
		created, err := c.CreateResource(ctx, res.Data, res.Filename, res.Mime)
		if err != nil {
			return "", err
		}
		id := mapfield.String(created, "id")
		resourceIDByOldID[oldID] = id
		report.ResourcesUploaded++
		return id, nil
	}

	for _, note := range parsed.Notes {
		var newID string
		switch {
		case note.SourceID != "":
			newID = noteIDBySourceID[note.SourceID]
		case note.SourceFilePath != "":
			newID = noteIDByFilePath[note.SourceFilePath]
		}
		if newID == "" {
			continue // skipped as a duplicate, or failed to create — nothing to rewrite.
		}

		// Isolated per note — a failure here (e.g. a transient network
		// error, or the note being deleted concurrently) must not abort the
		// rest of the batch or lose the report entirely: pass 1 already
		// wrote real notes to Joplin by this point, so an unhandled error
		// here would discard all visibility into what actually happened.
		// The note itself still exists (just with stale :/oldId
		// references), which is why this is tracked separately from
		// NotesFailed rather than folded into it.
		if err := rewriteNoteLinks(ctx, c, &report, parsed.Notes, note, newID, resourcesByID, knownSourceIDs, noteIDBySourceID, noteIDByFilePath, uploadResourceOnce); err != nil {
			report.LinkRewriteFailed = append(report.LinkRewriteFailed, TitledError{Title: note.Title, Error: err.Error()})
		}
	}

	return report, nil
}

func rewriteNoteLinks(
	ctx context.Context,
	c client.Client,
	report *ImportReport,
	allNotes []ParsedNote,
	note ParsedNote,
	newID string,
	resourcesByID map[string]ParsedResource,
	knownSourceIDs map[string]bool,
	noteIDBySourceID map[string]string,
	noteIDByFilePath map[string]string,
	uploadResourceOnce func(string) (string, error),
) error {
	current, err := c.GetNote(ctx, newID, []string{"id", "body"})
	if err != nil {
		return err
	}
	body := mapfield.String(current, "body")
	changed := false

	// Resource tokens (:/id) can appear in either source's body now: JEX
	// notes have them natively, and markdownsource.go embeds the same shape
	// for local asset links (see rewriteAssetLinks's doc comment) so this
	// one pass handles both without caring which source produced the note.
	// Note-ID tokens (the knownSourceIDs branch) are JEX-only in practice —
	// markdown notes have no sourceId to begin with, so that branch simply
	// never matches for them.
	var replacements [][2]string
	for _, m := range resourceLinkRE.FindAllStringSubmatch(body, -1) {
		oldID := m[1]
		if _, ok := resourcesByID[oldID]; ok {
			newResourceID, err := uploadResourceOnce(oldID)
			if err != nil {
				return err
			}
			if newResourceID != "" {
				replacements = append(replacements, [2]string{oldID, newResourceID})
			} else {
				report.UnresolvedLinks++
			}
		} else if knownSourceIDs[oldID] {
			if mappedNoteID, ok := noteIDBySourceID[oldID]; ok {
				replacements = append(replacements, [2]string{oldID, mappedNoteID})
			} else {
				report.UnresolvedLinks++ // linked note existed in the source but was skipped/failed.
			}
		}
		// Anything else (an ID that isn't part of this import batch at all)
		// is left completely alone — it may be a valid reference to a note
		// that already existed in Joplin before this import.
	}
	for _, rep := range replacements {
		before := body
		body = strings.ReplaceAll(body, ":/"+rep[0], ":/"+rep[1])
		if body != before {
			changed = true
		}
	}

	if note.SourceFilePath != "" {
		dir := filepath.Dir(note.SourceFilePath)
		matches := mdLinkRE.FindAllStringSubmatchIndex(body, -1)
		var rewritten strings.Builder
		lastIndex := 0
		for _, m := range matches {
			fullStart, fullEnd := m[0], m[1]
			text := body[m[2]:m[3]]
			target := body[m[4]:m[5]]
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, ":/") {
				continue
			}
			resolved := resolvePath(dir, target)
			if targetNoteID, ok := noteIDByFilePath[resolved]; ok {
				rewritten.WriteString(body[lastIndex:fullStart])
				rewritten.WriteString(fmt.Sprintf("[%s](:/%s)", text, targetNoteID))
				lastIndex = fullEnd
				changed = true
			} else if noteExistsAtPath(allNotes, resolved) {
				report.UnresolvedLinks++ // pointed at another imported file that was skipped/failed.
			}
		}
		if changed {
			rewritten.WriteString(body[lastIndex:])
			body = rewritten.String()
		}
	}

	if changed {
		if _, err := c.UpdateNote(ctx, newID, map[string]any{"body": body}); err != nil {
			return err
		}
	}
	return nil
}

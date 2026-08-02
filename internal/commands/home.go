package commands

import (
	"context"
	"sync"

	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/mapfield"
	"github.com/gcerar/joplin-axi/internal/toon"
)

// HomeView is the no-args view (AXI principle 8: content-first). Shows live
// state instead of a usage manual — connectivity, notebook/tag counts, and
// recent notes.
func HomeView(ctx context.Context, c client.Client) (string, error) {
	if !c.Ping(ctx) {
		return toon.Sections(
			toon.Object("joplin-axi", []toon.Field{{Key: "bin", Value: "joplin-axi"}, {Key: "clipper", Value: "unreachable"}}),
			toon.Help([]string{"Check that Joplin is running and Web Clipper is enabled (Tools → Options → Web Clipper)."}),
		), nil
	}

	// Matches the TS version's Promise.all — these three reads are
	// independent, so fetch them concurrently rather than one at a time.
	var notebooks, tags, recent []map[string]any
	var notebooksErr, tagsErr, recentErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		notebooks, notebooksErr = c.ListNotebooks(ctx, nil)
	}()
	go func() {
		defer wg.Done()
		tags, tagsErr = c.ListTags(ctx, nil)
	}()
	go func() {
		defer wg.Done()
		recent, recentErr = c.ListNotes(ctx, client.ListNotesOptions{
			Fields: []string{"id", "title", "parent_id", "updated_time"},
			Limit:  5,
		})
	}()
	wg.Wait()

	if notebooksErr != nil {
		return "", notebooksErr
	}
	if tagsErr != nil {
		return "", tagsErr
	}
	if recentErr != nil {
		return "", recentErr
	}

	summary := toon.Object("joplin-axi", []toon.Field{
		{Key: "bin", Value: "joplin-axi"},
		{Key: "clipper", Value: "reachable"},
		{Key: "notebooks", Value: len(notebooks)},
		{Key: "tags", Value: len(tags)},
	})

	rows := make([]map[string]any, len(recent))
	for i, n := range recent {
		rows[i] = map[string]any{
			"id":      n["id"],
			"title":   n["title"],
			"updated": toon.FmtTime(mapfield.Int64(n, "updated_time")),
		}
	}

	recentTable := "recent_notes: 0 notes found"
	if len(rows) > 0 {
		recentTable = toon.Table("recent_notes", []string{"id", "title", "updated"}, rows)
	}

	hints := toon.Help([]string{
		"Run `joplin-axi notes list` to browse notes.",
		"Run `joplin-axi notebooks list` to see the notebook tree.",
		"Run `joplin-axi --help` for the full command reference.",
	})

	return toon.Sections(summary, recentTable, hints), nil
}

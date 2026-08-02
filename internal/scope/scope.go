// Package scope resolves a note or tag selection shared by several commands:
// note-scope.ts's --notebook/--tag/--query/--task ID-intersection logic
// (used by notes list and tags add/remove) and tag-lookup.ts's tag
// title-to-ID resolution (used by notes list --tag and every tags command).
package scope

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/mapfield"
)

// CheckScopesHint is the shared empty-match hint for any scope-filtered
// command (notes list, tags add/remove).
const CheckScopesHint = "Run `joplin-axi notebooks list` or `joplin-axi tags list` to check available scopes."

// Cap on the /search-derived candidate set when combined with other filters.
const defaultSearchCap = 500

// Set is a minimal generic set, used here to intersect note-ID candidate
// sets from multiple active filters. Go's stdlib has no generic Set type
// (unlike JS's built-in Set<string>, which note-scope.ts relies on).
type Set[T comparable] map[T]struct{}

// NewSet builds a Set from the given items.
func NewSet[T comparable](items ...T) Set[T] {
	s := make(Set[T], len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

// Has reports whether item is in the set.
func (s Set[T]) Has(item T) bool {
	_, ok := s[item]
	return ok
}

// Options mirrors note-scope.ts's NoteScopeOptions.
type Options struct {
	NotebookID string
	TagID      string
	Query      string
	Task       bool
	Fields     []string
	// SearchCap caps the /search-derived candidate set when combined with
	// other filters. Zero means "use the default" (500).
	SearchCap int
}

func byRecency(notes []map[string]any) {
	sort.SliceStable(notes, func(i, j int) bool {
		return mapfield.Int64(notes[i], "updated_time") > mapfield.Int64(notes[j], "updated_time")
	})
}

// ResolveNoteScope resolves the set of notes matching any combination of
// notebook/tag/query/task filters. NotebookID and TagID each contribute a
// *full* candidate set fetched from their own ID-scoped endpoint (no title
// ever touches a query string — see commands/notes.go's list command for
// why that matters). Query/Task contribute a capped candidate set from
// Joplin's real full-text search. When more than one filter is active, the
// result is their intersection by note ID.
//
// With zero filters, returns *everything* (unbounded) — safe for a
// read-only listing (notes list), but callers doing a mutation should
// validate that at least one filter (or an explicit ID list) was given
// before calling this, rather than relying on this function to refuse an
// unscoped "everything".
func ResolveNoteScope(ctx context.Context, c client.Client, opts Options) ([]map[string]any, error) {
	searchCap := opts.SearchCap
	if searchCap == 0 {
		searchCap = defaultSearchCap
	}

	var sourceFuncs []func() ([]map[string]any, error)
	if opts.NotebookID != "" {
		sourceFuncs = append(sourceFuncs, func() ([]map[string]any, error) {
			return c.ListNotes(ctx, client.ListNotesOptions{NotebookID: opts.NotebookID, Fields: opts.Fields, Limit: client.Unlimited})
		})
	}
	if opts.TagID != "" {
		sourceFuncs = append(sourceFuncs, func() ([]map[string]any, error) {
			return c.ListNotes(ctx, client.ListNotesOptions{TagID: opts.TagID, Fields: opts.Fields, Limit: client.Unlimited})
		})
	}
	if opts.Query != "" || opts.Task {
		searchQuery := opts.Query
		if searchQuery == "" {
			searchQuery = "*"
		}
		if opts.Task {
			searchQuery += " type:todo"
		}
		sourceFuncs = append(sourceFuncs, func() ([]map[string]any, error) {
			return c.ListNotes(ctx, client.ListNotesOptions{Query: searchQuery, Fields: opts.Fields, Limit: searchCap})
		})
	}

	if len(sourceFuncs) == 0 {
		items, err := c.ListNotes(ctx, client.ListNotesOptions{Fields: opts.Fields, Limit: client.Unlimited})
		if err != nil {
			return nil, err
		}
		byRecency(items)
		return items, nil
	}

	if len(sourceFuncs) == 1 {
		items, err := sourceFuncs[0]()
		if err != nil {
			return nil, err
		}
		byRecency(items)
		return items, nil
	}

	// Fetch every active filter's candidate set concurrently, matching the
	// TS version's Promise.all — these are independent reads.
	results := make([][]map[string]any, len(sourceFuncs))
	errs := make([]error, len(sourceFuncs))
	var wg sync.WaitGroup
	wg.Add(len(sourceFuncs))
	for i, fn := range sourceFuncs {
		go func(i int, fn func() ([]map[string]any, error)) {
			defer wg.Done()
			results[i], errs[i] = fn()
		}(i, fn)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	idSets := make([]Set[string], len(results))
	for i, items := range results {
		ids := make(Set[string], len(items))
		for _, n := range items {
			ids[mapfield.String(n, "id")] = struct{}{}
		}
		idSets[i] = ids
	}
	rest := idSets[1:]

	// Iterate results[0] itself (not idSets[0]) to keep a deterministic base
	// order — map iteration order is unspecified, and this order feeds a
	// *stable* sort below, so ties in updated_time must come from something
	// deterministic (the first source's own returned order), exactly like
	// TS's `[...first]` (a Set's iteration order = its insertion order =
	// the order results[0].items was appended in).
	var common []map[string]any
	for _, n := range results[0] {
		id := mapfield.String(n, "id")
		inAll := true
		for _, s := range rest {
			if !s.Has(id) {
				inAll = false
				break
			}
		}
		if inAll {
			common = append(common, n)
		}
	}
	byRecency(common)
	return common, nil
}

// ResolveTagID resolves a tag *title* to its ID — shared by notes list --tag
// and every tags command.
func ResolveTagID(ctx context.Context, c client.Client, title string) (string, error) {
	allTags, err := c.ListTags(ctx, nil)
	if err != nil {
		return "", err
	}
	for _, t := range allTags {
		if mapfield.String(t, "title") == title {
			return mapfield.String(t, "id"), nil
		}
	}
	return "", &args.UsageError{
		Message:   fmt.Sprintf("no tag titled `%s`", title),
		HelpLines: []string{"run `joplin-axi tags list` to see available tags, or `joplin-axi tags create <title>` to make one"},
	}
}

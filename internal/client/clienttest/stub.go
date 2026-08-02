// Package clienttest provides a hand-written test double for client.Client,
// the direct replacement for the TS test suite's fakeClientFactory pattern.
// Mirrors stdlib's own net/http/httptest split: test-only code lives in its
// own package so it never pulls into the production build graph.
package clienttest

import (
	"context"
	"sync"

	"github.com/gcerar/joplin-axi/internal/client"
)

// StubClient is a test double for client.Client. Only methods with their
// corresponding *Func field set are actually implemented — any other method
// call falls through to the embedded nil client.Client interface and
// panics. That's the Go analogue of the TS fake silently returning
// `undefined` for an un-stubbed method: a nil-interface panic fails loudly
// at the exact call site instead, which is easier to debug, not harder.
//
// Every call is also recorded as a raw argument slice in the matching
// *Calls field, replacing vi.fn() call-argument assertions. Recording is
// mutex-protected: production code (e.g. scope.ResolveNoteScope, HomeView)
// legitimately calls the same Client method concurrently from multiple
// goroutines, and appending to a plain slice from more than one goroutine
// at once is a real data race, not just a theoretical one — go test -race
// catches it immediately without this.
type StubClient struct {
	client.Client
	mu sync.Mutex

	PingCalls [][]any
	PingFunc  func() bool

	ListNotebooksCalls [][]any
	ListNotebooksFunc  func(fields []string) ([]map[string]any, error)

	ListTagsCalls [][]any
	ListTagsFunc  func(fields []string) ([]map[string]any, error)

	GetTagsByNoteCalls [][]any
	GetTagsByNoteFunc  func(noteID string, fields []string) ([]map[string]any, error)

	GetNoteCalls [][]any
	GetNoteFunc  func(id string, fields []string) (map[string]any, error)

	GetNoteResourcesCalls [][]any
	GetNoteResourcesFunc  func(noteID string, fields []string) ([]map[string]any, error)

	CreateNoteCalls [][]any
	CreateNoteFunc  func(fields map[string]any) (map[string]any, error)

	UpdateNoteCalls [][]any
	UpdateNoteFunc  func(id string, fields map[string]any) (map[string]any, error)

	DeleteNoteCalls [][]any
	DeleteNoteFunc  func(id string) error

	RestoreNoteCalls [][]any
	RestoreNoteFunc  func(id string) error

	CreateNotebookCalls [][]any
	CreateNotebookFunc  func(fields map[string]any) (map[string]any, error)

	UpdateNotebookCalls [][]any
	UpdateNotebookFunc  func(id string, fields map[string]any) (map[string]any, error)

	DeleteNotebookCalls [][]any
	DeleteNotebookFunc  func(id string) error

	RestoreNotebookCalls [][]any
	RestoreNotebookFunc  func(id string) error

	CreateTagCalls [][]any
	CreateTagFunc  func(title string) (map[string]any, error)

	UpdateTagCalls [][]any
	UpdateTagFunc  func(id, title string) (map[string]any, error)

	DeleteTagCalls [][]any
	DeleteTagFunc  func(id string) error

	AddTagToNoteCalls [][]any
	AddTagToNoteFunc  func(tagID, noteID string) error

	RemoveTagFromNoteCalls [][]any
	RemoveTagFromNoteFunc  func(tagID, noteID string) error

	CreateResourceCalls [][]any
	CreateResourceFunc  func(data []byte, filename, mimeType string) (map[string]any, error)

	ListNotesCalls [][]any
	ListNotesFunc  func(opts client.ListNotesOptions) ([]map[string]any, error)
}

func (s *StubClient) recordCall(calls *[][]any, callArgs []any) {
	s.mu.Lock()
	*calls = append(*calls, callArgs)
	s.mu.Unlock()
}

func (s *StubClient) Ping(ctx context.Context) bool {
	s.recordCall(&s.PingCalls, nil)
	if s.PingFunc != nil {
		return s.PingFunc()
	}
	return s.Client.Ping(ctx)
}

func (s *StubClient) ListNotebooks(ctx context.Context, fields []string) ([]map[string]any, error) {
	s.recordCall(&s.ListNotebooksCalls, []any{fields})
	if s.ListNotebooksFunc != nil {
		return s.ListNotebooksFunc(fields)
	}
	return s.Client.ListNotebooks(ctx, fields)
}

func (s *StubClient) ListTags(ctx context.Context, fields []string) ([]map[string]any, error) {
	s.recordCall(&s.ListTagsCalls, []any{fields})
	if s.ListTagsFunc != nil {
		return s.ListTagsFunc(fields)
	}
	return s.Client.ListTags(ctx, fields)
}

func (s *StubClient) GetTagsByNote(ctx context.Context, noteID string, fields []string) ([]map[string]any, error) {
	s.recordCall(&s.GetTagsByNoteCalls, []any{noteID, fields})
	if s.GetTagsByNoteFunc != nil {
		return s.GetTagsByNoteFunc(noteID, fields)
	}
	return s.Client.GetTagsByNote(ctx, noteID, fields)
}

func (s *StubClient) GetNote(ctx context.Context, id string, fields []string) (map[string]any, error) {
	s.recordCall(&s.GetNoteCalls, []any{id, fields})
	if s.GetNoteFunc != nil {
		return s.GetNoteFunc(id, fields)
	}
	return s.Client.GetNote(ctx, id, fields)
}

func (s *StubClient) GetNoteResources(ctx context.Context, noteID string, fields []string) ([]map[string]any, error) {
	s.recordCall(&s.GetNoteResourcesCalls, []any{noteID, fields})
	if s.GetNoteResourcesFunc != nil {
		return s.GetNoteResourcesFunc(noteID, fields)
	}
	return s.Client.GetNoteResources(ctx, noteID, fields)
}

func (s *StubClient) CreateNote(ctx context.Context, fields map[string]any) (map[string]any, error) {
	s.recordCall(&s.CreateNoteCalls, []any{fields})
	if s.CreateNoteFunc != nil {
		return s.CreateNoteFunc(fields)
	}
	return s.Client.CreateNote(ctx, fields)
}

func (s *StubClient) UpdateNote(ctx context.Context, id string, fields map[string]any) (map[string]any, error) {
	s.recordCall(&s.UpdateNoteCalls, []any{id, fields})
	if s.UpdateNoteFunc != nil {
		return s.UpdateNoteFunc(id, fields)
	}
	return s.Client.UpdateNote(ctx, id, fields)
}

func (s *StubClient) DeleteNote(ctx context.Context, id string) error {
	s.recordCall(&s.DeleteNoteCalls, []any{id})
	if s.DeleteNoteFunc != nil {
		return s.DeleteNoteFunc(id)
	}
	return s.Client.DeleteNote(ctx, id)
}

func (s *StubClient) RestoreNote(ctx context.Context, id string) error {
	s.recordCall(&s.RestoreNoteCalls, []any{id})
	if s.RestoreNoteFunc != nil {
		return s.RestoreNoteFunc(id)
	}
	return s.Client.RestoreNote(ctx, id)
}

func (s *StubClient) CreateNotebook(ctx context.Context, fields map[string]any) (map[string]any, error) {
	s.recordCall(&s.CreateNotebookCalls, []any{fields})
	if s.CreateNotebookFunc != nil {
		return s.CreateNotebookFunc(fields)
	}
	return s.Client.CreateNotebook(ctx, fields)
}

func (s *StubClient) UpdateNotebook(ctx context.Context, id string, fields map[string]any) (map[string]any, error) {
	s.recordCall(&s.UpdateNotebookCalls, []any{id, fields})
	if s.UpdateNotebookFunc != nil {
		return s.UpdateNotebookFunc(id, fields)
	}
	return s.Client.UpdateNotebook(ctx, id, fields)
}

func (s *StubClient) DeleteNotebook(ctx context.Context, id string) error {
	s.recordCall(&s.DeleteNotebookCalls, []any{id})
	if s.DeleteNotebookFunc != nil {
		return s.DeleteNotebookFunc(id)
	}
	return s.Client.DeleteNotebook(ctx, id)
}

func (s *StubClient) RestoreNotebook(ctx context.Context, id string) error {
	s.recordCall(&s.RestoreNotebookCalls, []any{id})
	if s.RestoreNotebookFunc != nil {
		return s.RestoreNotebookFunc(id)
	}
	return s.Client.RestoreNotebook(ctx, id)
}

func (s *StubClient) CreateTag(ctx context.Context, title string) (map[string]any, error) {
	s.recordCall(&s.CreateTagCalls, []any{title})
	if s.CreateTagFunc != nil {
		return s.CreateTagFunc(title)
	}
	return s.Client.CreateTag(ctx, title)
}

func (s *StubClient) UpdateTag(ctx context.Context, id, title string) (map[string]any, error) {
	s.recordCall(&s.UpdateTagCalls, []any{id, title})
	if s.UpdateTagFunc != nil {
		return s.UpdateTagFunc(id, title)
	}
	return s.Client.UpdateTag(ctx, id, title)
}

func (s *StubClient) DeleteTag(ctx context.Context, id string) error {
	s.recordCall(&s.DeleteTagCalls, []any{id})
	if s.DeleteTagFunc != nil {
		return s.DeleteTagFunc(id)
	}
	return s.Client.DeleteTag(ctx, id)
}

func (s *StubClient) AddTagToNote(ctx context.Context, tagID, noteID string) error {
	s.recordCall(&s.AddTagToNoteCalls, []any{tagID, noteID})
	if s.AddTagToNoteFunc != nil {
		return s.AddTagToNoteFunc(tagID, noteID)
	}
	return s.Client.AddTagToNote(ctx, tagID, noteID)
}

func (s *StubClient) RemoveTagFromNote(ctx context.Context, tagID, noteID string) error {
	s.recordCall(&s.RemoveTagFromNoteCalls, []any{tagID, noteID})
	if s.RemoveTagFromNoteFunc != nil {
		return s.RemoveTagFromNoteFunc(tagID, noteID)
	}
	return s.Client.RemoveTagFromNote(ctx, tagID, noteID)
}

func (s *StubClient) CreateResource(ctx context.Context, data []byte, filename, mimeType string) (map[string]any, error) {
	s.recordCall(&s.CreateResourceCalls, []any{data, filename, mimeType})
	if s.CreateResourceFunc != nil {
		return s.CreateResourceFunc(data, filename, mimeType)
	}
	return s.Client.CreateResource(ctx, data, filename, mimeType)
}

func (s *StubClient) ListNotes(ctx context.Context, opts client.ListNotesOptions) ([]map[string]any, error) {
	s.recordCall(&s.ListNotesCalls, []any{opts})
	if s.ListNotesFunc != nil {
		return s.ListNotesFunc(opts)
	}
	return s.Client.ListNotes(ctx, opts)
}

var _ client.Client = (*StubClient)(nil)

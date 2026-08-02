package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client/clienttest"
)

func TestPingCommand(t *testing.T) {
	ctx := context.Background()
	noArgs := args.ParsedArgs{}

	t.Run("reports unreachable and failed auth without attempting an authenticated call", func(t *testing.T) {
		stub := &clienttest.StubClient{
			PingFunc:          func() bool { return false },
			ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		}

		result, err := PingCommand.Run(ctx, noArgs, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "clipper: unreachable") {
			t.Errorf("output %q does not contain 'clipper: unreachable'", result.Output)
		}
		if !strings.Contains(result.Output, "auth: failed") {
			t.Errorf("output %q does not contain 'auth: failed'", result.Output)
		}
		if len(stub.ListNotebooksCalls) != 0 {
			t.Error("ListNotebooks was called despite the clipper being unreachable")
		}
	})

	t.Run("reports reachable and auth ok when an authenticated call succeeds", func(t *testing.T) {
		stub := &clienttest.StubClient{
			PingFunc:          func() bool { return true },
			ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, nil },
		}

		result, err := PingCommand.Run(ctx, noArgs, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "clipper: reachable") {
			t.Errorf("output %q does not contain 'clipper: reachable'", result.Output)
		}
		if !strings.Contains(result.Output, "auth: ok") {
			t.Errorf("output %q does not contain 'auth: ok'", result.Output)
		}
	})

	t.Run("reports reachable but failed auth when the token is rejected", func(t *testing.T) {
		stub := &clienttest.StubClient{
			PingFunc:          func() bool { return true },
			ListNotebooksFunc: func([]string) ([]map[string]any, error) { return nil, errors.New("401") },
		}

		result, err := PingCommand.Run(ctx, noArgs, stub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Output, "clipper: reachable") {
			t.Errorf("output %q does not contain 'clipper: reachable'", result.Output)
		}
		if !strings.Contains(result.Output, "auth: failed") {
			t.Errorf("output %q does not contain 'auth: failed'", result.Output)
		}
	})
}

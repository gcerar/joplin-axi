package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// noEnv simulates an environment with no JOPLIN_TOKEN/JOPLIN_BASE_URL set —
// every case below that reaches requireEnv fails before any network call,
// so these stay fast, deterministic unit tests. Exercising a full command's
// successful execution through Run (a real network round-trip) is covered
// by the commands package's own stub-backed tests plus this project's
// established live-verification-against-real-Joplin discipline, not here.
func noEnv(string) string { return "" }

func TestRun(t *testing.T) {
	ctx := context.Background()

	t.Run("--help prints top-level help and exits 0", func(t *testing.T) {
		var out bytes.Buffer
		code := Run(ctx, []string{"--help"}, &out, noEnv)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "joplin-axi — AXI-style CLI for Joplin") {
			t.Errorf("output %q does not contain the top-level help banner", out.String())
		}
	})

	t.Run("-h is equivalent to --help", func(t *testing.T) {
		var out bytes.Buffer
		code := Run(ctx, []string{"-h"}, &out, noEnv)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("no args with no JOPLIN_TOKEN set reports the error and exits 1 without touching the network", func(t *testing.T) {
		var out bytes.Buffer
		code := Run(ctx, nil, &out, noEnv)
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if !strings.Contains(out.String(), "JOPLIN_TOKEN environment variable is required") {
			t.Errorf("output %q does not report the missing token", out.String())
		}
	})

	t.Run("a known top-level command with no JOPLIN_TOKEN set fails before any network call", func(t *testing.T) {
		var out bytes.Buffer
		code := Run(ctx, []string{"ping"}, &out, noEnv)
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if !strings.Contains(out.String(), "JOPLIN_TOKEN environment variable is required") {
			t.Errorf("output %q does not report the missing token", out.String())
		}
	})

	t.Run("unknown top-level command reports an error listing valid commands and exits 2", func(t *testing.T) {
		var out bytes.Buffer
		code := Run(ctx, []string{"bogus"}, &out, noEnv)
		if code != 2 {
			t.Errorf("exit code = %d, want 2", code)
		}
		if !strings.Contains(out.String(), "unknown command `bogus`") {
			t.Errorf("output %q does not report the unknown command", out.String())
		}
		if !strings.Contains(out.String(), "ping") {
			t.Errorf("output %q does not list `ping` as a valid command", out.String())
		}
	})

	// "known group, unknown subcommand within it" (the sub != "" branch of
	// the unknown-command check) has no test yet — groups is empty until
	// Phase 2 ports the first real one (notebooks). Add that case there,
	// against the real group, rather than a fake one registered just for
	// this test.
}

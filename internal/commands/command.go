// Package commands implements each joplin-axi subcommand.
package commands

import (
	"context"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
)

// CommandResult is a command's successful outcome. ExitCode is always
// explicit (unlike the TS string | CommandOutput union, where its absence
// meant 0) — Go has no union return, so this is one extra field on the
// common case rather than a special-cased shorthand.
type CommandResult struct {
	Output   string
	ExitCode int
}

// Ok builds a normal, exit-0 result — the common case.
func Ok(output string) CommandResult { return CommandResult{Output: output} }

// Failed builds a result that still prints its full report but signals
// exit 1 — used by batch operations where at least one item failed but the
// operation as a whole still produced a valid, worth-showing report.
func Failed(output string) CommandResult { return CommandResult{Output: output, ExitCode: 1} }

// Command is the contract every subcommand implements. Run's error return is
// reserved for failures that prevent producing any report at all (bad flags,
// an unreachable client) — a *args.UsageError specifically maps to exit code
// 2, anything else to exit code 1. Per-item failure within a batch operation
// is data, not control flow: it must accumulate into a CommandResult (via
// Failed), never propagate out as this error.
type Command struct {
	Spec args.CommandSpec
	Run  func(ctx context.Context, parsed args.ParsedArgs, c client.Client) (CommandResult, error)
	// NoClient marks a command that never touches Joplin (e.g. skill) — cli.Run
	// skips the JOPLIN_TOKEN/JOPLIN_BASE_URL check for it and passes a nil
	// client, which Run must not dereference.
	NoClient bool
}

// FailedItem records one failed element of a batch operation, for reporting
// (id/title plus the error), not for programmatic retry.
type FailedItem[T any] struct {
	Item  T
	Error string
}

// ApplyToEach runs fn over every item sequentially, isolating each error
// instead of aborting the batch — so one failure doesn't hide what already
// succeeded. Sequential, not concurrent: Joplin's local server has no
// documented concurrency guarantee, and introducing goroutines here would be
// a behavior change, not just a translation.
func ApplyToEach[T any](items []T, fn func(T) error) (succeeded []T, failed []FailedItem[T]) {
	for _, item := range items {
		if err := fn(item); err != nil {
			failed = append(failed, FailedItem[T]{Item: item, Error: err.Error()})
			continue
		}
		succeeded = append(succeeded, item)
	}
	return succeeded, failed
}

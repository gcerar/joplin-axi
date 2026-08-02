// Package cli is joplin-axi's entry-point dispatcher — routes argv to
// top-level commands (ping, import) or <group> <command> (notes/notebooks/
// tags), prints the result, and reports the process exit code.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/commands"
	"github.com/gcerar/joplin-axi/internal/toon"
)

// groups holds each <group> <command> command set.
var groups = map[string]map[string]commands.Command{
	"notebooks": commands.NotebooksCommands,
	"tags":      commands.TagsCommands,
	"notes":     commands.NotesCommands,
}

// topLevelCommands holds single-verb commands that don't fit the <group>
// <command> shape.
var topLevelCommands = map[string]commands.Command{
	"ping":   commands.PingCommand,
	"import": commands.ImportCommand,
	"skill":  commands.SkillCommand,
}

// Version is set via -ldflags at build time (see .goreleaser.yaml) —
// "dev" identifies a local/unreleased build.
var Version = "dev"

const topLevelHelp = `joplin-axi — AXI-style CLI for Joplin

usage: joplin-axi <group> <command> [flags]
       joplin-axi ping
       joplin-axi import <path> [flags]
       joplin-axi skill [--output <path>]
       joplin-axi --version
       joplin-axi

groups:
  notebooks   list, create, update, delete, restore
  tags        list, of, create, update, delete, add, remove
  notes       list, get, find-in, links, resources, create, update, edit, delete, restore

Run ` + "`joplin-axi <group> <command> --help`" + ` for details on a specific command.`

// sortedKeys returns m's keys sorted alphabetically — used only for
// validation-error hints ("valid commands: ..."), where a stable, scannable
// listing is what matters; unlike TOON's object/table field order, there's
// no declared order here worth preserving (Go maps have none to begin with).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func requireEnv(getenv func(string) string, stdout io.Writer) (client.Options, bool) {
	token := getenv("JOPLIN_TOKEN")
	if token == "" {
		fmt.Fprintln(stdout, toon.ErrorOut("JOPLIN_TOKEN environment variable is required", []string{
			"export JOPLIN_TOKEN=<token from Joplin → Options → Web Clipper>",
		}))
		return client.Options{}, false
	}
	baseURL := getenv("JOPLIN_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:41184"
	}
	return client.Options{BaseURL: baseURL, Token: token}, true
}

func reportError(stdout io.Writer, err error) int {
	var usageErr *args.UsageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(stdout, toon.ErrorOut(usageErr.Message, usageErr.HelpLines))
		return 2
	}
	fmt.Fprintln(stdout, toon.ErrorOut(err.Error(), nil))
	return 1
}

func runCommand(ctx context.Context, cmd commands.Command, argv []string, getenv func(string) string, stdout io.Writer) int {
	parsed, err := args.ParseArgs(argv, cmd.Spec)
	if err != nil {
		return reportError(stdout, err)
	}
	if parsed.Help {
		fmt.Fprintln(stdout, args.HelpText(cmd.Spec))
		return 0
	}

	var c client.Client
	if !cmd.NoClient {
		opts, ok := requireEnv(getenv, stdout)
		if !ok {
			return 1
		}
		c = client.New(opts)
	}

	result, err := cmd.Run(ctx, parsed, c)
	if err != nil {
		return reportError(stdout, err)
	}
	fmt.Fprintln(stdout, result.Output)
	return result.ExitCode
}

// Run dispatches argv (already stripped of the program name) and returns the
// process exit code. Takes stdout/getenv as parameters rather than touching
// os.Stdout/os.Getenv directly, so it's unit-testable without a subprocess —
// closing a gap the original cli.ts had (no test file of its own).
func Run(ctx context.Context, argv []string, stdout io.Writer, getenv func(string) string) int {
	if len(argv) == 0 {
		opts, ok := requireEnv(getenv, stdout)
		if !ok {
			return 1
		}
		c := client.New(opts)
		out, err := commands.HomeView(ctx, c)
		if err != nil {
			return reportError(stdout, err)
		}
		fmt.Fprintln(stdout, out)
		return 0
	}

	first, rest := argv[0], argv[1:]

	if first == "--help" || first == "-h" {
		fmt.Fprintln(stdout, topLevelHelp)
		return 0
	}

	if first == "--version" || first == "-v" {
		fmt.Fprintln(stdout, "joplin-axi "+Version)
		return 0
	}

	if cmd, ok := topLevelCommands[first]; ok {
		return runCommand(ctx, cmd, rest, getenv, stdout)
	}

	group, ok := groups[first]
	if !ok {
		valid := append(sortedKeys(groups), sortedKeys(topLevelCommands)...)
		sort.Strings(valid)
		fmt.Fprintln(stdout, toon.ErrorOut(fmt.Sprintf("unknown command `%s`", first), []string{
			"valid commands: " + strings.Join(valid, ", "),
		}))
		return 2
	}

	var sub string
	var cmdArgv []string
	if len(rest) > 0 {
		sub, cmdArgv = rest[0], rest[1:]
	}

	cmd, ok := group[sub]
	if !ok {
		attempted := first
		if sub != "" {
			attempted = first + " " + sub
		}
		fmt.Fprintln(stdout, toon.ErrorOut(fmt.Sprintf("unknown command `%s`", attempted), []string{
			fmt.Sprintf("valid subcommands for `%s`: %s", first, strings.Join(sortedKeys(group), ", ")),
		}))
		return 2
	}

	return runCommand(ctx, cmd, cmdArgv, getenv, stdout)
}

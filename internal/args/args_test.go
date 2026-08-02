package args

import (
	"errors"
	"strings"
	"testing"
)

func testSpec() CommandSpec {
	return CommandSpec{
		Name:    "notes list",
		Summary: "List notes.",
		Usage:   "joplin-axi notes list [--query <text>] [--limit <n>]",
		Flags: []FlagSpec{
			{Name: "query", Type: FlagString, Description: "Free-text search"},
			{Name: "limit", Type: FlagNumber, Description: "Max results", Default: float64(20)},
			{Name: "task", Type: FlagBoolean, Description: "Restrict to to-dos", Default: false},
		},
		Examples: []string{"joplin-axi notes list --task"},
	}
}

func mustUsageError(t *testing.T, err error) *UsageError {
	t.Helper()
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("got error %v (%T), want *UsageError", err, err)
	}
	return usageErr
}

func TestParseArgs(t *testing.T) {
	spec := testSpec()

	t.Run("applies defaults when a flag is omitted", func(t *testing.T) {
		parsed, err := ParseArgs(nil, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := parsed.NumberFlag("limit"); got != 20 {
			t.Errorf("limit = %v, want 20", got)
		}
		if parsed.BoolFlag("task") != false {
			t.Errorf("task = true, want false")
		}
	})

	t.Run("parses --flag value pairs", func(t *testing.T) {
		parsed, err := ParseArgs([]string{"--query", "annual report"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := parsed.StringFlag("query"); got != "annual report" {
			t.Errorf("query = %q, want %q", got, "annual report")
		}
	})

	t.Run("parses --flag=value form", func(t *testing.T) {
		parsed, err := ParseArgs([]string{"--limit=5"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := parsed.NumberFlag("limit"); got != 5 {
			t.Errorf("limit = %v, want 5", got)
		}
	})

	t.Run("rejects a non-numeric value for a number flag instead of producing NaN", func(t *testing.T) {
		if _, err := ParseArgs([]string{"--limit", "abc"}, spec); err == nil {
			t.Error("expected an error for --limit abc")
		} else {
			mustUsageError(t, err)
		}
		if _, err := ParseArgs([]string{"--limit=abc"}, spec); err == nil {
			t.Error("expected an error for --limit=abc")
		} else {
			mustUsageError(t, err)
		}
	})

	t.Run("treats a boolean flag with no value as true", func(t *testing.T) {
		parsed, err := ParseArgs([]string{"--task"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.BoolFlag("task") {
			t.Error("task = false, want true")
		}
	})

	t.Run("parses --flag=true/--flag=false explicitly", func(t *testing.T) {
		parsedTrue, err := ParseArgs([]string{"--task=true"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsedTrue.BoolFlag("task") {
			t.Error("task = false, want true")
		}

		parsedFalse, err := ParseArgs([]string{"--task=false"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsedFalse.BoolFlag("task") {
			t.Error("task = true, want false")
		}
	})

	t.Run("rejects a non-boolean value for a boolean flag instead of silently treating it as true", func(t *testing.T) {
		if _, err := ParseArgs([]string{"--task=0"}, spec); err == nil {
			t.Error("expected an error for --task=0")
		} else {
			mustUsageError(t, err)
		}
		if _, err := ParseArgs([]string{"--task=no"}, spec); err == nil {
			t.Error("expected an error for --task=no")
		} else {
			mustUsageError(t, err)
		}
	})

	t.Run("collects positionals separately from flags", func(t *testing.T) {
		parsed, err := ParseArgs([]string{"abc123", "--limit", "5"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(parsed.Positionals) != 1 || parsed.Positionals[0] != "abc123" {
			t.Errorf("positionals = %v, want [abc123]", parsed.Positionals)
		}
		if got, _ := parsed.NumberFlag("limit"); got != 5 {
			t.Errorf("limit = %v, want 5", got)
		}
	})

	t.Run("sets help and stops treating it as a positional", func(t *testing.T) {
		parsed, err := ParseArgs([]string{"--help"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.Help {
			t.Error("help = false, want true")
		}
		if len(parsed.Positionals) != 0 {
			t.Errorf("positionals = %v, want none", parsed.Positionals)
		}
	})

	t.Run("throws UsageError on an unknown flag, listing the valid ones", func(t *testing.T) {
		_, err := ParseArgs([]string{"--bogus"}, spec)
		usageErr := mustUsageError(t, err)
		if !strings.Contains(usageErr.Message, "unknown flag --bogus") {
			t.Errorf("message = %q, want to contain %q", usageErr.Message, "unknown flag --bogus")
		}
		if !strings.Contains(strings.Join(usageErr.HelpLines, " "), "--query") {
			t.Errorf("help lines = %v, want to contain --query", usageErr.HelpLines)
		}
	})

	t.Run("throws UsageError when a string flag is missing its value", func(t *testing.T) {
		if _, err := ParseArgs([]string{"--query"}, spec); err == nil {
			t.Error("expected an error")
		} else {
			mustUsageError(t, err)
		}
	})

	t.Run("lets --help short-circuit past a later invalid flag", func(t *testing.T) {
		// notes list --help --bogus: must show help, not error on --bogus.
		parsed, err := ParseArgs([]string{"--help", "--bogus"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.Help {
			t.Error("help = false, want true")
		}
	})

	t.Run(`does not treat a flag VALUE of "--help" as a help request`, func(t *testing.T) {
		// --query is a string flag, so the token right after it is consumed as
		// its value — "--help" here should become the query text, not trigger help.
		parsed, err := ParseArgs([]string{"--query", "--help"}, spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := parsed.StringFlag("query"); got != "--help" {
			t.Errorf("query = %q, want %q", got, "--help")
		}
		if parsed.Help {
			t.Error("help = true, want false")
		}
	})
}

func TestSplitList(t *testing.T) {
	t.Run("splits, trims, and drops empty entries", func(t *testing.T) {
		got := SplitList("a, b ,, c")
		want := []string{"a", "b", "c"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("returns an empty slice for an empty string", func(t *testing.T) {
		got := SplitList("")
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestHelpText(t *testing.T) {
	t.Run("includes usage, flags with defaults, and examples", func(t *testing.T) {
		text := HelpText(testSpec())
		for _, want := range []string{"usage: joplin-axi notes list", "--limit <number>", "(default: 20)", "joplin-axi notes list --task"} {
			if !strings.Contains(text, want) {
				t.Errorf("help text %q does not contain %q", text, want)
			}
		}
	})
}

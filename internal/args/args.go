// Package args is a minimal flag parser. No dependency, full control over
// AXI-style usage errors (exit code 2, unknown flags rejected with the valid
// set inlined). Go's stdlib flag package doesn't fit AXI's --flag=value +
// --flag value + typed-flag + help-shortcircuit + subcommand shape, so this
// is a hand port rather than a stdlib swap.
package args

import (
	"fmt"
	"strconv"
	"strings"
)

// UsageError signals a usage/validation problem (unknown flag, missing
// value, bad flag type) — the caller maps this to exit code 2.
type UsageError struct {
	Message   string
	HelpLines []string
}

func (e *UsageError) Error() string { return e.Message }

// FlagType is a closed set (string/boolean/number) modeled as typed string
// constants — Go has no union types, so ParseArgs and any future
// flag-value construction must go through FlagSpec/ParseArgs rather than
// constructing a FlagType value directly.
type FlagType string

const (
	FlagString  FlagType = "string"
	FlagBoolean FlagType = "boolean"
	FlagNumber  FlagType = "number"
)

// FlagSpec describes one flag. Default is nil for "no default" (mirrors the
// TS `default?`); otherwise a string, bool, or float64 matching Type.
type FlagSpec struct {
	Name        string
	Type        FlagType
	Description string
	Default     any
}

// CommandSpec drives both parsing and --help rendering. Flags is an ordered
// slice, not a map — flag declaration order matters for help text and error
// messages (an unknown-flag error lists valid flags in this order), and Go
// map iteration order is unspecified.
type CommandSpec struct {
	Name     string
	Summary  string
	Usage    string
	Flags    []FlagSpec
	Examples []string
}

func (s CommandSpec) flag(name string) (FlagSpec, bool) {
	for _, f := range s.Flags {
		if f.Name == name {
			return f, true
		}
	}
	return FlagSpec{}, false
}

// ParsedArgs is the result of a successful parse. Flags values are string,
// bool, or float64, matching FlagSpec.Type.
type ParsedArgs struct {
	Positionals []string
	Flags       map[string]any
	Help        bool
}

func (p ParsedArgs) StringFlag(name string) (string, bool) {
	s, ok := p.Flags[name].(string)
	return s, ok
}

func (p ParsedArgs) BoolFlag(name string) bool {
	b, _ := p.Flags[name].(bool)
	return b
}

func (p ParsedArgs) NumberFlag(name string) (float64, bool) {
	n, ok := p.Flags[name].(float64)
	return n, ok
}

// ParseArgs parses argv against spec.
func ParseArgs(argv []string, spec CommandSpec) (ParsedArgs, error) {
	flags := map[string]any{}
	var positionals []string

	for i := 0; i < len(argv); i++ {
		arg := argv[i]

		// Short-circuits rather than just setting a flag and continuing, so a
		// bare --help always wins over flag validation later in the same argv
		// — e.g. `notes list --help --bogus` shows help instead of erroring on
		// --bogus. (A token consumed as a preceding string flag's *value*
		// never reaches this check, so `--title --help` correctly treats
		// "--help" as the title, not a help request — see args_test.go.)
		if arg == "--help" || arg == "-h" {
			return ParsedArgs{Positionals: positionals, Flags: flags, Help: true}, nil
		}

		if strings.HasPrefix(arg, "--") {
			eq := strings.IndexByte(arg, '=')
			var name string
			if eq == -1 {
				name = arg[2:]
			} else {
				name = arg[2:eq]
			}

			flagSpec, ok := spec.flag(name)
			if !ok {
				valid := make([]string, len(spec.Flags))
				for i, f := range spec.Flags {
					valid[i] = "--" + f.Name
				}
				validStr := "(none)"
				if len(valid) > 0 {
					validStr = strings.Join(valid, ", ")
				}
				return ParsedArgs{}, &UsageError{
					Message:   fmt.Sprintf("unknown flag --%s for `%s`", name, spec.Name),
					HelpLines: []string{fmt.Sprintf("valid flags for `%s`: %s", spec.Name, validStr)},
				}
			}

			if flagSpec.Type == FlagBoolean {
				if eq == -1 {
					flags[name] = true
				} else {
					value := arg[eq+1:]
					if value != "true" && value != "false" {
						return ParsedArgs{}, &UsageError{
							Message:   fmt.Sprintf("--%s must be `true` or `false`, got `%s`", name, value),
							HelpLines: []string{spec.Usage},
						}
					}
					flags[name] = value == "true"
				}
				continue
			}

			var value string
			if eq != -1 {
				value = arg[eq+1:]
			} else {
				i++
				if i >= len(argv) {
					return ParsedArgs{}, &UsageError{
						Message:   fmt.Sprintf("--%s requires a value", name),
						HelpLines: []string{spec.Usage},
					}
				}
				value = argv[i]
			}

			if flagSpec.Type == FlagNumber {
				// strconv.ParseFloat is stricter than JS's Number() (no
				// whitespace trimming, no empty-string-means-0) — an
				// acceptable, documented difference; nothing in this CLI's
				// numeric flags (--limit, etc.) relies on those JS quirks.
				num, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return ParsedArgs{}, &UsageError{
						Message:   fmt.Sprintf("--%s must be a number, got `%s`", name, value),
						HelpLines: []string{spec.Usage},
					}
				}
				flags[name] = num
			} else {
				flags[name] = value
			}
			continue
		}

		positionals = append(positionals, arg)
	}

	for _, f := range spec.Flags {
		if _, exists := flags[f.Name]; !exists && f.Default != nil {
			flags[f.Name] = f.Default
		}
	}

	return ParsedArgs{Positionals: positionals, Flags: flags, Help: false}, nil
}

// SplitList turns a comma-separated flag value into trimmed, non-empty
// entries. Shared by every command that takes a --fields/--notes-style list
// flag.
func SplitList(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// RequirePositional fetches positionals[index], returning a consistent
// UsageError if missing. Shared by every command with a required
// <id>/<title>-style argument.
func RequirePositional(parsed ParsedArgs, index int, name string, usage string) (string, error) {
	if index >= len(parsed.Positionals) || parsed.Positionals[index] == "" {
		return "", &UsageError{
			Message:   fmt.Sprintf("missing required argument <%s>", name),
			HelpLines: []string{usage},
		}
	}
	return parsed.Positionals[index], nil
}

// HelpText renders a command's --help output.
func HelpText(spec CommandSpec) string {
	lines := []string{spec.Summary, "", "usage: " + spec.Usage}

	if len(spec.Flags) > 0 {
		lines = append(lines, "", "flags:")
		for _, f := range spec.Flags {
			def := ""
			if f.Default != nil {
				def = fmt.Sprintf(" (default: %v)", f.Default)
			}
			lines = append(lines, fmt.Sprintf("  --%s <%s>  %s%s", f.Name, f.Type, f.Description, def))
		}
	}

	if len(spec.Examples) > 0 {
		lines = append(lines, "", "examples:")
		for _, ex := range spec.Examples {
			lines = append(lines, "  "+ex)
		}
	}

	return strings.Join(lines, "\n")
}

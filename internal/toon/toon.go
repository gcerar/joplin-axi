// Package toon is a minimal TOON (Token-Oriented Object Notation) writer.
// Spec: https://toonformat.dev — this covers the subset AXI tools need:
// tables, scalars, objects, help hints, and structured errors.
package toon

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FmtTime formats a Joplin epoch-ms timestamp field, shared by every command
// that displays one. Zero (Joplin's "unset" sentinel, same as JS's falsy 0)
// renders as an empty string, matching the original's ternary fallback.
func FmtTime(ms int64) string {
	if ms == 0 {
		return ""
	}
	// Fixed-width millisecond precision to match JS's Date.toISOString()
	// exactly (e.g. "2023-01-01T00:00:00.000Z"), not Go's variable-precision
	// RFC3339Nano.
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

var needsQuote = regexp.MustCompile(`["\n,]`)

func quoteIfNeeded(value string) string {
	if needsQuote.MatchString(value) {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func cell(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return quoteIfNeeded(v)
	default:
		return fmt.Sprint(v)
	}
}

// Table renders a named, counted collection of rows with a fixed field order
// (the row maps themselves need no ordering — fields dictates column order).
func Table(name string, fields []string, rows []map[string]any) string {
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, fmt.Sprintf("%s[%d]{%s}:", name, len(rows), strings.Join(fields, ",")))
	for _, row := range rows {
		cells := make([]string, len(fields))
		for i, f := range fields {
			cells[i] = cell(row[f])
		}
		lines = append(lines, "  "+strings.Join(cells, ","))
	}
	return strings.Join(lines, "\n")
}

// Scalar renders a single "key: value" line.
func Scalar(key string, value any) string {
	return fmt.Sprintf("%s: %v", key, value)
}

func indentMultiline(value string) string {
	lines := strings.Split(value, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// Field is one key/value entry in an Object — a slice, not a map, because
// TOON's object output is order-sensitive (mirrors an object literal's
// declared field order in the original TS) and Go map iteration order is
// unspecified.
type Field struct {
	Key   string
	Value any
}

// Object renders a named block of ordered key: value lines.
func Object(name string, fields []Field) string {
	lines := make([]string, 0, len(fields)+1)
	lines = append(lines, name+":")
	for _, f := range fields {
		lines = append(lines, "  "+f.Key+": "+indentMultiline(fmt.Sprint(f.Value)))
	}
	return strings.Join(lines, "\n")
}

// Help renders a numbered help block, or an empty string for zero lines.
func Help(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, fmt.Sprintf("help[%d]:", len(lines)))
	for _, l := range lines {
		out = append(out, "  "+l)
	}
	return strings.Join(out, "\n")
}

// ErrorOut renders a structured error line plus zero or more help lines —
// AXI errors print to stdout in this same format, never to stderr.
func ErrorOut(message string, helpLines []string) string {
	parts := make([]string, 0, len(helpLines)+1)
	parts = append(parts, "error: "+message)
	for _, l := range helpLines {
		parts = append(parts, "help: "+l)
	}
	return strings.Join(parts, "\n")
}

// TruncateResult is truncate's output — the shown text plus whether/how much
// was cut, so callers can render a "N chars total, run --full" hint.
type TruncateResult struct {
	Text      string
	Truncated bool
	Total     int
}

// Truncate cuts text to at most limit runes. Rune count (not byte length) is
// used deliberately: byte-slicing arbitrary UTF-8 (note bodies can contain
// multi-byte characters) risks cutting mid-character and producing invalid
// text, which raw len()-based slicing in Go would silently do.
func Truncate(text string, limit int) TruncateResult {
	runes := []rune(text)
	total := len(runes)
	if total <= limit {
		return TruncateResult{Text: text, Truncated: false, Total: total}
	}
	return TruncateResult{Text: string(runes[:limit]), Truncated: true, Total: total}
}

// Sections joins non-empty parts with a blank line between each, dropping
// any empty ones — used to compose a command's body + help block cleanly
// whether or not the help block ended up empty.
func Sections(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

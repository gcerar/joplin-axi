package importer

import (
	"regexp"
	"slices"
	"strings"
)

// ParsedFrontmatter is the result of parsing a `---\nkey: value\n---` block.
// No YAML dependency (matching this project's "no dependency, full control"
// style) — only handles the small field shapes markdown import actually
// needs: flat scalars, `[a, b]`/comma-separated inline lists, and `- item`
// block lists. Anything fancier (nested maps, multiline scalars, YAML
// anchors) is out of scope; such a file just gets treated as having no
// frontmatter (the `---` block, if any, is left in the body untouched).
type ParsedFrontmatter struct {
	Fields map[string]string
	Lists  map[string][]string
	Body   string
}

var (
	lineSplitRE      = regexp.MustCompile(`\r?\n`)
	delimiterRE      = regexp.MustCompile(`^---\s*$`)
	frontmatterKeyRE = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.*)$`)
	blockListItemRE  = regexp.MustCompile(`^\s*-\s*`)
)

func stripQuotes(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && strings.HasSuffix(trimmed, `"`)) || (trimmed[0] == '\'' && strings.HasSuffix(trimmed, "'")) {
			return trimmed[1 : len(trimmed)-1]
		}
	}
	return trimmed
}

func splitInlineList(value string) []string {
	inner := strings.TrimSpace(value)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	var result []string
	for _, part := range strings.Split(inner, ",") {
		if s := stripQuotes(part); s != "" {
			result = append(result, s)
		}
	}
	return result
}

// DefaultListKeys are keys treated as comma-separated lists when written as
// a bare scalar (no brackets) — e.g. `tags: work, urgent`. Deliberately NOT
// a generic "any comma-containing value is a list" rule: a `title: Hello,
// World` would wrongly split otherwise. Bracket syntax (`key: [a, b]`) is
// unambiguous and applies to any key regardless of this list.
var DefaultListKeys = []string{"tags", "categories", "keywords"}

// ParseFrontmatter parses content's leading `---`-delimited frontmatter
// block, if any, using listKeys to decide which bare-scalar keys split on
// comma. Pass nil for listKeys to use DefaultListKeys.
func ParseFrontmatter(content string, listKeys []string) ParsedFrontmatter {
	if listKeys == nil {
		listKeys = DefaultListKeys
	}
	noFrontmatter := ParsedFrontmatter{Fields: map[string]string{}, Lists: map[string][]string{}, Body: content}

	lines := lineSplitRE.Split(content, -1)
	if len(lines) == 0 || !delimiterRE.MatchString(lines[0]) {
		return noFrontmatter
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if delimiterRE.MatchString(lines[i]) {
			end = i
			break
		}
	}
	if end == -1 {
		return noFrontmatter
	}

	fields := map[string]string{}
	lists := map[string][]string{}

	i := 1
	for i < end {
		line := lines[i]
		m := frontmatterKeyRE.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		key, rawValue := m[1], m[2]

		if strings.HasPrefix(strings.TrimSpace(rawValue), "[") {
			lists[key] = splitInlineList(rawValue)
			i++
			continue
		}

		if strings.TrimSpace(rawValue) == "" {
			// Possible YAML block list on following indented `- item` lines.
			var items []string
			j := i + 1
			for j < end && blockListItemRE.MatchString(lines[j]) {
				items = append(items, stripQuotes(blockListItemRE.ReplaceAllString(lines[j], "")))
				j++
			}
			if len(items) > 0 {
				lists[key] = items
				i = j
				continue
			}
			fields[key] = ""
			i++
			continue
		}

		if slices.Contains(listKeys, key) && strings.Contains(rawValue, ",") {
			// A bare comma-separated list scalar (common for `tags: a, b, c`
			// without brackets).
			var l []string
			for _, part := range strings.Split(rawValue, ",") {
				if s := stripQuotes(part); s != "" {
					l = append(l, s)
				}
			}
			lists[key] = l
			i++
			continue
		}

		fields[key] = stripQuotes(rawValue)
		i++
	}

	body := strings.Join(lines[end+1:], "\n")
	body = strings.TrimLeft(body, "\n")
	return ParsedFrontmatter{Fields: fields, Lists: lists, Body: body}
}

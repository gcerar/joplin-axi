// Package mapfield extracts typed values from a Joplin API response map
// (map[string]any, from encoding/json's default unmarshal target) — shared
// by every package that reads a note/notebook/tag object back from the
// client, rather than repeating the same type assertions in each one.
//
// Named mapfield rather than the more obvious "fields" deliberately: the
// command layer's own local variables are conventionally named `fields`
// (the API request payload being built, matching the TS source's own
// naming) — importing a package literally called `fields` would shadow
// that in almost every function that needs both.
package mapfield

// Int64 reads an int64 field, defaulting to 0 if absent or the wrong type.
// JSON numbers decode into float64 via encoding/json's default `any` target.
func Int64(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

// String reads a string field, defaulting to "" if absent or the wrong type.
func String(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// StringOr reads a string field, falling back to fallback only if the field
// is genuinely absent or not a string — mirrors JS's ?? nullish-coalescing
// semantics (e.g. `notebook.id ?? id`): an empty-string value is present and
// NOT treated as "missing", unlike a falsy-check (||) would treat it.
func StringOr(m map[string]any, key, fallback string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return fallback
}

// Bool reads a bool-ish field. Joplin's boolean fields (is_todo, etc.) are
// actually 0/1 integers over the wire, so a nonzero float64 counts as true
// too, matching a JS truthy check on the same JSON number.
func Bool(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	}
	return false
}

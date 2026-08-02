package commands

// fieldInt64 safely reads an int64 field from a Joplin API response map.
// JSON numbers decode into float64 via encoding/json's default `any` target,
// so this centralizes that conversion (and the "missing/wrong type -> 0"
// fallback) rather than repeating a type assertion at every call site.
func fieldInt64(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

// fieldString safely reads a string field from a Joplin API response map,
// defaulting to "" if absent or the wrong type.
func fieldString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// fieldStringOr reads a string field from m, falling back to fallback only
// if the field is genuinely absent or not a string — mirrors JS's ??
// nullish-coalescing semantics (notebook.id ?? id): an empty-string value is
// present and NOT treated as "missing", unlike a falsy-check (||) would.
func fieldStringOr(m map[string]any, key, fallback string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return fallback
}

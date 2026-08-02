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

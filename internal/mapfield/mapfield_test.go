package mapfield

import "testing"

func TestInt64(t *testing.T) {
	if got := Int64(map[string]any{"n": float64(42)}, "n"); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	if got := Int64(map[string]any{}, "missing"); got != 0 {
		t.Errorf("got %d, want 0 for a missing key", got)
	}
	if got := Int64(map[string]any{"n": "not a number"}, "n"); got != 0 {
		t.Errorf("got %d, want 0 for the wrong type", got)
	}
}

func TestString(t *testing.T) {
	if got := String(map[string]any{"s": "hi"}, "s"); got != "hi" {
		t.Errorf("got %q, want hi", got)
	}
	if got := String(map[string]any{}, "missing"); got != "" {
		t.Errorf("got %q, want empty for a missing key", got)
	}
}

func TestStringOr(t *testing.T) {
	t.Run("returns the field value when present, even if empty", func(t *testing.T) {
		if got := StringOr(map[string]any{"id": ""}, "id", "fallback"); got != "" {
			t.Errorf("got %q, want empty string (present, not missing)", got)
		}
	})

	t.Run("falls back only when the field is absent", func(t *testing.T) {
		if got := StringOr(map[string]any{}, "id", "fallback"); got != "fallback" {
			t.Errorf("got %q, want fallback", got)
		}
	})

	t.Run("falls back when the field is the wrong type", func(t *testing.T) {
		if got := StringOr(map[string]any{"id": 42}, "id", "fallback"); got != "fallback" {
			t.Errorf("got %q, want fallback", got)
		}
	})
}

func TestBool(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"true bool", map[string]any{"b": true}, true},
		{"false bool", map[string]any{"b": false}, false},
		{"nonzero float64 (Joplin's is_todo: 1)", map[string]any{"b": float64(1)}, true},
		{"zero float64 (Joplin's is_todo: 0)", map[string]any{"b": float64(0)}, false},
		{"missing key", map[string]any{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Bool(c.m, "b"); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

package commands

import (
	"errors"
	"testing"
)

func TestApplyToEach(t *testing.T) {
	t.Run("isolates a failure instead of aborting the batch", func(t *testing.T) {
		succeeded, failed := ApplyToEach([]string{"a", "bad", "c"}, func(item string) error {
			if item == "bad" {
				return errors.New("boom")
			}
			return nil
		})

		if len(succeeded) != 2 || succeeded[0] != "a" || succeeded[1] != "c" {
			t.Errorf("succeeded = %v, want [a c]", succeeded)
		}
		if len(failed) != 1 || failed[0].Item != "bad" || failed[0].Error != "boom" {
			t.Errorf("failed = %v, want [{bad boom}]", failed)
		}
	})

	t.Run("all succeed", func(t *testing.T) {
		succeeded, failed := ApplyToEach([]int{1, 2, 3}, func(int) error { return nil })
		if len(succeeded) != 3 || len(failed) != 0 {
			t.Errorf("succeeded = %v, failed = %v", succeeded, failed)
		}
	})

	t.Run("all fail", func(t *testing.T) {
		succeeded, failed := ApplyToEach([]int{1, 2}, func(int) error { return errors.New("nope") })
		if len(succeeded) != 0 || len(failed) != 2 {
			t.Errorf("succeeded = %v, failed = %v", succeeded, failed)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		succeeded, failed := ApplyToEach[string](nil, func(string) error { return nil })
		if len(succeeded) != 0 || len(failed) != 0 {
			t.Errorf("succeeded = %v, failed = %v, want both empty", succeeded, failed)
		}
	})
}

func TestOkAndFailed(t *testing.T) {
	if got := Ok("hi"); got != (CommandResult{Output: "hi", ExitCode: 0}) {
		t.Errorf("Ok(%q) = %+v", "hi", got)
	}
	if got := Failed("oops"); got != (CommandResult{Output: "oops", ExitCode: 1}) {
		t.Errorf("Failed(%q) = %+v", "oops", got)
	}
}

package chapters

import (
	"strings"
	"testing"
)

func mk(line int, start int64, title string) Chapter {
	return Chapter{Start: start, Title: title, Line: line}
}

func TestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		ch := []Chapter{
			mk(1, 0, "Intro"),
			mk(2, 330000, "Part 2"),
			mk(3, 6150000, "Final"),
		}
		if err := Validate(ch); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		err := Validate(nil)
		if err == nil {
			t.Fatal("expected error for empty chapters")
		}
		if !strings.Contains(err.Error(), "at least one") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("first not at zero", func(t *testing.T) {
		ch := []Chapter{mk(1, 1000, "Intro")}
		err := Validate(ch)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "00:00:00") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("not sorted", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, "a"), mk(2, 5000, "b"), mk(3, 2000, "c")}
		err := Validate(ch)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "chronological") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("duplicate timestamp", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, "a"), mk(2, 5000, "b"), mk(3, 5000, "c")}
		err := Validate(ch)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("title with newline", func(t *testing.T) {
		ch := []Chapter{{Start: 0, Title: "bad\ntitle", Line: 1}}
		err := Validate(ch)
		if err == nil {
			t.Fatal("expected error for title with newline")
		}
	})

	t.Run("title ending in backslash rejected", func(t *testing.T) {
		ch := []Chapter{{Start: 0, Title: `Trail\`, Line: 1}}
		err := Validate(ch)
		if err == nil {
			t.Fatal("expected error for title ending in a backslash")
		}
		if !strings.Contains(err.Error(), "must not end with a backslash") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("mid-string backslash allowed", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, `Back\slash`), mk(2, 5000, "b")}
		if err := Validate(ch); err != nil {
			t.Fatalf("mid-string backslash should be allowed: %v", err)
		}
	})
}

func TestValidateDuration(t *testing.T) {
	t.Run("within duration ok", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, "a"), mk(2, 1000, "b")}
		if err := ValidateDuration(ch, 2000); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("exactly at duration ok", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, "a"), mk(2, 5000, "b")}
		if err := ValidateDuration(ch, 5000); err != nil {
			t.Fatalf("expected == duration to be allowed, got error: %v", err)
		}
	})

	t.Run("beyond duration fails", func(t *testing.T) {
		ch := []Chapter{mk(1, 0, "a"), mk(2, 5001, "b")}
		err := ValidateDuration(ch, 5000)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("unexpected message: %v", err)
		}
	})
}

func TestFormatMilliseconds(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "00:00:00"},
		{1000, "00:00:01"},
		{330000, "00:05:30"},
		{6150000, "01:42:30"},
		{500, "00:00:00.500"},
		{1250, "00:00:01.250"},
		{3600000, "01:00:00"},
		{359999000, "99:59:59"},
	}
	for _, c := range cases {
		if got := FormatMilliseconds(c.in); got != c.want {
			t.Errorf("FormatMilliseconds(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

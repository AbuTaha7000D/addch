package chapters

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	t.Run("single chapter", func(t *testing.T) {
		got := Format([]Chapter{{Start: 0, Title: "Intro", Line: 1}})
		want := "00:00:00 Intro\n"
		if got != want {
			t.Errorf("Format = %q, want %q", got, want)
		}
	})

	t.Run("multi chapter", func(t *testing.T) {
		got := Format([]Chapter{
			{Start: 0, Title: "Intro", Line: 1},
			{Start: 330000, Title: "Part 2", Line: 2},
			{Start: 6150000, Title: "Final", Line: 3},
		})
		want := "00:00:00 Intro\n00:05:30 Part 2\n01:42:30 Final\n"
		if got != want {
			t.Errorf("Format = %q, want %q", got, want)
		}
	})

	t.Run("fractional timestamps", func(t *testing.T) {
		got := Format([]Chapter{
			{Start: 0, Title: "Start", Line: 1},
			{Start: 1250, Title: "Getting Started", Line: 2},
			{Start: 5000, Title: "Whole Second", Line: 3},
		})
		want := "00:00:00 Start\n00:00:01.250 Getting Started\n00:00:05 Whole Second\n"
		if got != want {
			t.Errorf("Format = %q, want %q", got, want)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		if got := Format(nil); got != "" {
			t.Errorf("Format(nil) = %q, want empty", got)
		}
		if got := Format([]Chapter{}); got != "" {
			t.Errorf("Format([]) = %q, want empty", got)
		}
	})

	t.Run("title written verbatim with LF endings", func(t *testing.T) {
		got := Format([]Chapter{
			{Start: 0, Title: "مقدمة With Spaces", Line: 1},
			{Start: 1000, Title: `Back\slash`, Line: 2},
		})
		want := "00:00:00 مقدمة With Spaces\n00:00:01 Back\\slash\n"
		if got != want {
			t.Errorf("Format = %q, want %q", got, want)
		}
		if strings.Contains(got, "\r") {
			t.Error("Format must use LF-only line endings")
		}
	})
}

func TestWrite(t *testing.T) {
	t.Run("writes canonical form", func(t *testing.T) {
		ch := []Chapter{
			{Start: 0, Title: "Intro", Line: 1},
			{Start: 500, Title: "Half Second", Line: 2},
			{Start: 1500, Title: "End", Line: 3},
		}
		var buf bytes.Buffer
		if err := Write(&buf, ch); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "00:00:00 Intro\n00:00:00.500 Half Second\n00:00:01.500 End\n"
		if buf.String() != want {
			t.Errorf("Write = %q, want %q", buf.String(), want)
		}
	})

	t.Run("empty slice writes nothing", func(t *testing.T) {
		var buf bytes.Buffer
		if err := Write(&buf, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("Write(nil) wrote %d bytes, want 0", buf.Len())
		}
	})
}

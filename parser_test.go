package main

import (
	"strings"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"00:00:00", 0, false},
		{"00:00:01", 1000, false},
		{"00:05:30", 330000, false},
		{"01:00:00", 3600000, false},
		{"01:42:30", 6150000, false},
		{"99:59:59", 359999000, false},
		{"00:00:00.500", 500, false},
		{"00:00:00.5", 500, false},
		{"00:00:00.05", 50, false},
		{"00:00:00.005", 5, false},
		{"00:00:01.999", 1999, false},
		{"00:00:00.9999", 999, false}, // extra precision truncated
		{"00:00:00.1234x", 0, true},   // non-digit beyond ms precision must be rejected
		{"00:00:00.9999a", 0, true},   // trailing garbage after ms precision rejected
		{"05:30", 0, true},
		{"00:00", 0, true},
		{"abc:12:34", 0, true},
		{"00:65:00", 0, true},
		{"00:00:65", 0, true},
		{"a0:00:00", 0, true},
		{"00:00:00.", 0, true},  // missing digits after dot
		{"00:00:00.a", 0, true}, // non-digit fractional
		{"0:00:00", 0, true},    // single digit hour
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := parseTimestamp(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseTimestamp(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTimestamp(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseTimestamp(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseChapters(t *testing.T) {
	t.Run("valid basic", func(t *testing.T) {
		input := "00:00:00 Intro\n00:05:30 Chapter 2\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 2 {
			t.Fatalf("expected 2 chapters, got %d", len(ch))
		}
		if ch[0].Start != 0 || ch[0].Title != "Intro" || ch[0].Line != 1 {
			t.Errorf("unexpected first chapter: %+v", ch[0])
		}
		if ch[1].Start != 330000 || ch[1].Title != "Chapter 2" || ch[1].Line != 2 {
			t.Errorf("unexpected second chapter: %+v", ch[1])
		}
	})

	t.Run("ignores blank lines", func(t *testing.T) {
		input := "00:00:00 Intro\n\n\n00:05:30 Part 2\n  \n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 2 {
			t.Fatalf("expected 2 chapters, got %d", len(ch))
		}
	})

	t.Run("handles BOM", func(t *testing.T) {
		input := "\uFEFF00:00:00 Intro\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 1 || ch[0].Title != "Intro" {
			t.Errorf("unexpected chapters: %+v", ch)
		}
	})

	t.Run("handles CRLF", func(t *testing.T) {
		input := "00:00:00 Intro\r\n00:05:30 Part 2\r\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 2 {
			t.Fatalf("expected 2 chapters, got %d", len(ch))
		}
		if ch[1].Title != "Part 2" {
			t.Errorf("title should not contain CR: %q", ch[1].Title)
		}
	})

	t.Run("trims surrounding whitespace in title", func(t *testing.T) {
		input := "00:00:00   Spaced Title  \n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch[0].Title != "Spaced Title" {
			t.Errorf("expected trimmed title, got %q", ch[0].Title)
		}
	})

	t.Run("mixed precision allowed", func(t *testing.T) {
		input := "00:00:00 Intro\n00:00:01.250 Part\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch[1].Start != 1250 {
			t.Errorf("expected 1250ms, got %d", ch[1].Start)
		}
	})

	t.Run("unicode title", func(t *testing.T) {
		input := "00:00:00 مقدمة\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch[0].Title != "مقدمة" {
			t.Errorf("expected unicode title, got %q", ch[0].Title)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := parseChapters(strings.NewReader("not a timestamp\n"))
		if err == nil {
			t.Fatal("expected an error for invalid format")
		}
	})

	t.Run("missing title", func(t *testing.T) {
		_, err := parseChapters(strings.NewReader("00:00:00\n"))
		if err == nil {
			t.Fatal("expected an error for missing title")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		ch, err := parseChapters(strings.NewReader(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 0 {
			t.Fatalf("expected 0 chapters, got %d", len(ch))
		}
	})

	t.Run("title containing colon", func(t *testing.T) {
		input := "00:00:00 Module 3: Advanced\n"
		ch, err := parseChapters(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch[0].Title != "Module 3: Advanced" {
			t.Errorf("expected title with colon, got %q", ch[0].Title)
		}
	})
}

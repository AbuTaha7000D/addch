package chapters

import (
	"strings"
	"testing"
)

// TestParseTimestampAdversarial pins the remaining boundary and hostile inputs
// for parseTimestamp beyond the main table: negative timestamps, zero-valued
// fractional parts, and fraction lengths that decode to zero milliseconds.
func TestParseTimestampAdversarial(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"-00:00:01", 0, true},   // negative timestamps are not a valid format
		{"00:00:00.0", 0, false}, // single zero fractional digit
		{"00:00:00.00", 0, false},
		{"00:00:00.000", 0, false},
		{"00:00:00.0000", 0, false}, // extra precision truncated; still zero
		{"00:00:00.0009", 0, false}, // sub-millisecond digits are ignored
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

// TestParseChaptersAdversarial exercises the parser against sidecar-format
// variants that are "valid enough" to parse but hostile in the details: raw
// non-UTF-8 bytes, embedded whitespace that must be preserved or trimmed, a BOM
// on a non-first line, and a very long title that forces the scanner buffer to
// grow.
func TestParseChaptersAdversarial(t *testing.T) {
	t.Run("non-UTF8 title bytes pass through verbatim", func(t *testing.T) {
		// Raw ISO-8859-1 byte 0xE9 ("é"). The parser has no charset concept, so
		// the byte must survive byte-for-byte (UTF-8 normalization, if any, is
		// FFmpeg's concern and is exercised end-to-end elsewhere).
		raw := []byte("00:00:00 caf\xe9 latin1\n")
		ch, err := Parse(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 1 || ch[0].Title != "caf\xe9 latin1" {
			t.Errorf("latin-1 title not preserved verbatim: %+v", ch)
		}
	})

	t.Run("BOM stripped only on the first line", func(t *testing.T) {
		// A UTF-8 BOM at the very start of the file is stripped so the first
		// timestamp parses normally.
		ch, err := Parse(strings.NewReader("\xef\xbb\xbf00:00:00 First\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 1 || ch[0].Title != "First" {
			t.Fatalf("first-line BOM not stripped: %+v", ch)
		}

		// A mid-file BOM at the start of a later line is ordinary content: it is
		// not stripped, so it lands in the timestamp and the line must be
		// rejected rather than silently accepting a mangled timestamp.
		_, err = Parse(strings.NewReader("00:00:00 First\n\xef\xbb\xbf00:00:01 Second\n"))
		if err == nil {
			t.Fatal("mid-file BOM before a timestamp must be rejected")
		}
		if !strings.Contains(err.Error(), `hours must be two digits`) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("BOM inside a title is preserved as content", func(t *testing.T) {
		// A BOM after the timestamp separator is an ordinary title character.
		ch, err := Parse(strings.NewReader("00:00:00 \xef\xbb\xbfTitle\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 1 || ch[0].Title != "\ufeffTitle" {
			t.Errorf("BOM in title not preserved verbatim: %+v", ch)
		}
	})

	t.Run("inner whitespace kept, edges trimmed", func(t *testing.T) {
		input := "00:00:00\tLead\ttrail\t  \n"
		ch, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch[0].Title != "Lead\ttrail" {
			t.Errorf("expected inner tab preserved and edges trimmed, got %q", ch[0].Title)
		}
	})

	t.Run("very long title forces scanner buffer growth", func(t *testing.T) {
		// 70KB exceeds the parser's initial 64KB buffer, so the scanner must
		// grow to its 1MB cap for this line to parse.
		long := strings.Repeat("x", 70*1024)
		input := "00:00:00 " + long + "\n"
		ch, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ch) != 1 || ch[0].Title != long {
			t.Error("long title not preserved")
		}
	})
}

// TestFormatMillisecondsAdversarial pins zero-padding of sub-second
// timestamps: a millisecond value below 100 must render as a three-digit .mmm
// fraction, and rounded values must not collide.
func TestFormatMillisecondsAdversarial(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{5, "00:00:00.005"},
		{50, "00:00:00.050"},
		{500, "00:00:00.500"},
		{0, "00:00:00"},
		{999, "00:00:00.999"},
	}
	for _, c := range cases {
		if got := FormatMilliseconds(c.in); got != c.want {
			t.Errorf("FormatMilliseconds(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

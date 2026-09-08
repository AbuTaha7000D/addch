package chapters

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseFile reads and parses a chapters definition file.
//
// The expected format per non-empty line is:
//
//	HH:MM:SS Chapter title
//	HH:MM:SS[.mmm] Chapter title
//
// Blank lines are skipped. A UTF-8 BOM at the start of the file is stripped.
// Both LF ("\n") and CRLF ("\r\n") line endings are accepted.
func ParseFile(path string) ([]Chapter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open chapters file %q: %w", path, err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads and parses chapters from r.
func Parse(r io.Reader) ([]Chapter, error) {
	scanner := bufio.NewScanner(r)
	// Bump the buffer to allow long titles.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var chapters []Chapter
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if lineNum == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		line = strings.TrimRight(line, "\r")

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		ts, title, ok := splitTimestampTitle(line)
		if !ok {
			return nil, fmt.Errorf("line %d: invalid chapter format: expected \"HH:MM:SS Chapter title\"", lineNum)
		}
		ms, err := parseTimestamp(ts)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", lineNum, err)
		}
		chapters = append(chapters, Chapter{
			Start: ms,
			Title: title,
			Line:  lineNum,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading chapters file: %w", err)
	}
	return chapters, nil
}

// splitTimestampTitle splits a line into its timestamp and title components.
// It returns the raw timestamp string, the trimmed title, and whether the line
// was structurally valid (timestamp followed by whitespace and a non-empty title).
func splitTimestampTitle(line string) (ts, title string, ok bool) {
	// The timestamp is everything up to the first run of whitespace.
	idx := strings.IndexAny(line, " \t")
	if idx <= 0 {
		return "", "", false
	}
	ts = line[:idx]
	title = strings.TrimSpace(line[idx:])
	if title == "" {
		return "", "", false
	}
	return ts, title, true
}

// parseTimestamp parses an HH:MM:SS or HH:MM:SS.mmm timestamp into milliseconds
// using integer arithmetic only.
func parseTimestamp(s string) (int64, error) {
	orig := s

	// Split optional fractional part.
	frac := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		frac = s[i+1:]
		s = s[:i]
		if frac == "" {
			return 0, fmt.Errorf("invalid timestamp %q: missing digits after '.'", orig)
		}
		// Validate the full fractional part before truncating, so that trailing
		// non-digit characters beyond millisecond precision are rejected rather
		// than silently dropped.
		for i := 0; i < len(frac); i++ {
			if c := frac[i]; c < '0' || c > '9' {
				return 0, fmt.Errorf("invalid timestamp %q: fractional part must contain only digits", orig)
			}
		}
		if len(frac) > 3 {
			frac = frac[:3] // ignore precision beyond milliseconds
		}
	}

	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp %q: expected format HH:MM:SS", orig)
	}

	h, ok := parseTwoDigits(parts[0])
	if !ok {
		return 0, fmt.Errorf("invalid timestamp %q: hours must be two digits (00-99)", orig)
	}
	m, ok := parseTwoDigits(parts[1])
	if !ok || m > 59 {
		return 0, fmt.Errorf("invalid timestamp %q: minutes must be two digits (00-59)", orig)
	}
	sec, ok := parseTwoDigits(parts[2])
	if !ok || sec > 59 {
		return 0, fmt.Errorf("invalid timestamp %q: seconds must be two digits (00-59)", orig)
	}
	ms := 0
	if frac != "" {
		// frac is non-empty and digit-only (validated above), so only the length
		// determines the millisecond value.
		switch len(frac) {
		case 1:
			ms = int(frac[0]-'0') * 100
		case 2:
			ms = int(frac[0]-'0')*100 + int(frac[1]-'0')*10
		case 3:
			ms = int(frac[0]-'0')*100 + int(frac[1]-'0')*10 + int(frac[2]-'0')
		}
	}
	return ((int64(h)*60+int64(m))*60+int64(sec))*1000 + int64(ms), nil
}

// parseTwoDigits parses exactly two ASCII digits [0-9] into an integer.
func parseTwoDigits(s string) (int, bool) {
	if len(s) != 2 {
		return 0, false
	}
	a, b := s[0], s[1]
	if a < '0' || a > '9' || b < '0' || b > '9' {
		return 0, false
	}
	return int(a-'0')*10 + int(b-'0'), true
}

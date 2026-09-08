package chapters

import (
	"io"
	"strings"
)

// Format renders chapters in the canonical TXT form defined by GRAMMAR.md §9:
// one "HH:MM:SS[.mmm] Title" line per chapter, LF-terminated, with the title
// written verbatim. It returns an empty string for an empty slice.
func Format(chapters []Chapter) string {
	if len(chapters) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range chapters {
		b.WriteString(FormatMilliseconds(chapters[i].Start))
		b.WriteByte(' ')
		b.WriteString(chapters[i].Title)
		b.WriteByte('\n')
	}
	return b.String()
}

// Write writes the canonical serialization of chapters to w.
func Write(w io.Writer, chapters []Chapter) error {
	_, err := io.WriteString(w, Format(chapters))
	return err
}

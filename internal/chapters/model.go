// Package chapters defines the shared chapter data model, TXT parser,
// validator, and canonical TXT serializer used by the addch toolkit.
package chapters

import "fmt"

// Chapter is a single parsed chapter entry.
type Chapter struct {
	// Start is the start timestamp in milliseconds.
	Start int64
	// Title is the chapter title.
	Title string
	// Line is the 1-based line number in the input file where this chapter appeared.
	Line int
}

// FormatMilliseconds renders a millisecond count as HH:MM:SS[.mmm].
func FormatMilliseconds(ms int64) string {
	neg := ms < 0
	if neg {
		ms = -ms
	}
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	rem := ms % 1000
	base := fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	if rem != 0 {
		base += fmt.Sprintf(".%03d", rem)
	}
	if neg {
		return "-" + base
	}
	return base
}

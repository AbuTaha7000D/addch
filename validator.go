package main

import (
	"fmt"
	"strings"
)

// ErrValidation is returned when chapter validation fails. The message is
// human-readable and actionable.
type ErrValidation struct{ msg string }

func (e *ErrValidation) Error() string { return e.msg }

func validationErrorf(format string, args ...interface{}) error {
	return &ErrValidation{msg: fmt.Sprintf(format, args...)}
}

// ValidateChapters performs all structural validation that does not depend on
// the video. It returns an error describing the first problem found.
func ValidateChapters(chapters []Chapter) error {
	if len(chapters) == 0 {
		return validationErrorf("no chapters found: the chapters file must contain at least one chapter")
	}

	prevLine, prevStart := -1, int64(-1)
	for i := range chapters {
		c := &chapters[i]

		// Title must be non-empty (handled by parser) and contain no newlines.
		if strings.ContainsAny(c.Title, "\r\n") {
			return validationErrorf("line %d: chapter title must not contain newline characters", c.Line)
		}

		// A title ending in a backslash cannot be represented in FFmpeg's
		// FFMETADATA format: FFmpeg's reader treats a trailing backslash as an
		// escape of the line's terminating newline, so the newline (and any
		// following content) is absorbed into the title. This makes the title
		// impossible to round-trip exactly. Reject it explicitly so the user gets a
		// clear, actionable error instead of a corrupt chapter or a confusing
		// verification failure.
		if strings.HasSuffix(c.Title, `\`) {
			return validationErrorf(
				"line %d: chapter title must not end with a backslash ('\\') character — "+
					"FFmpeg's FFMETADATA format cannot represent it", c.Line)
		}

		if i > 0 && c.Start == prevStart {
			return validationErrorf(
				"lines %d and %d: duplicate timestamp %s — chapters must have distinct start times",
				prevLine, c.Line, formatMilliseconds(c.Start))
		}
		if i > 0 && c.Start < prevStart {
			return validationErrorf(
				"line %d: timestamp %s is before the previous chapter at %s — chapters must be in chronological order",
				c.Line, formatMilliseconds(c.Start), formatMilliseconds(prevStart))
		}
		prevLine, prevStart = c.Line, c.Start
	}

	if chapters[0].Start != 0 {
		return validationErrorf(
			"first chapter must start at 00:00:00, found %s",
			formatMilliseconds(chapters[0].Start))
	}

	return nil
}

// ValidateDuration checks that no chapter starts after the video ends.
// A chapter exactly at the video duration is allowed (it forms a zero-length
// trailing chapter, which FFmpeg handles).
func ValidateAgainstDuration(chapters []Chapter, durationMs int64) error {
	for i := range chapters {
		if chapters[i].Start > durationMs {
			return validationErrorf(
				"line %d: timestamp %s exceeds the video duration %s",
				chapters[i].Line, formatMilliseconds(chapters[i].Start), formatMilliseconds(durationMs))
		}
	}
	return nil
}

// formatMilliseconds renders a millisecond count as HH:MM:SS[.mmm].
func formatMilliseconds(ms int64) string {
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

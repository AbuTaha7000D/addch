package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/abutaha/addch/internal/chapters"
)

// probeJSON runs ffprobe with the given arguments and returns its stdout.
func probeJSON(args ...string) ([]byte, error) {
	full := append([]string{"-v", "quiet", "-print_format", "json"}, args...)
	out, err := exec.Command("ffprobe", full...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg != "" {
				return nil, fmt.Errorf("ffprobe failed: %s", msg)
			}
			return nil, fmt.Errorf("ffprobe could not process the file")
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	return out, nil
}

// probeFormat mimics the relevant subset of ffprobe's -show_format output.
type probeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// getVideoDurationMs returns the duration of a video in milliseconds using
// ffprobe. Zero or negative durations and unparseable values result in an error.
func getVideoDurationMs(videoPath string) (int64, error) {
	out, err := probeJSON("-show_format", videoPath)
	if err != nil {
		return 0, fmt.Errorf("could not read video %q: %v — is it a valid video file?", videoPath, err)
	}
	var pf probeFormat
	if err := json.Unmarshal(out, &pf); err != nil {
		return 0, fmt.Errorf("could not parse ffprobe output for %q: %v", videoPath, err)
	}
	if pf.Format.Duration == "" || pf.Format.Duration == "N/A" {
		return 0, fmt.Errorf("could not determine the duration of %q — is it a valid video file?", videoPath)
	}
	ms, err := parseDurationToMs(pf.Format.Duration)
	if err != nil {
		return 0, fmt.Errorf("could not parse duration %q for %q: %v", pf.Format.Duration, videoPath, err)
	}
	if ms <= 0 {
		return 0, fmt.Errorf("could not determine the duration of %q — is it a valid video file?", videoPath)
	}
	return ms, nil
}

// parseDurationToMs converts an FFprobe duration string like "5", "5.1",
// "5.123", "5.999" or "20.000000" into milliseconds using integer/decimal
// parsing only (no floating-point arithmetic). If more than three fractional
// digits are present, the value is truncated deterministically to millisecond
// precision (consistent with the chapter timestamp parser). Zero is allowed
// here; callers may reject it separately. Negative or non-numeric values are
// errors.
func parseDurationToMs(s string) (int64, error) {
	if s == "" || s == "N/A" {
		return 0, fmt.Errorf("missing duration")
	}
	// A single dot splits integer and fractional parts; a bare '-' is invalid (negative).
	intPart, frac := s, ""
	hasDot := strings.ContainsRune(s, '.')
	if hasDot {
		i := strings.IndexByte(s, '.')
		intPart = s[:i]
		frac = s[i+1:]
	}
	if !allDigits(intPart) {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	secs, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %v", s, err)
	}
	if secs < 0 {
		return 0, fmt.Errorf("invalid duration %q: negative", s)
	}
	// Guard against overflow when converting seconds to milliseconds (and adding
	// up to 999ms of fractional precision). A value this large is never a real
	// media duration and is rejected rather than silently wrapping to a wrong or
	// negative result.
	if secs > (math.MaxInt64-999)/1000 {
		return 0, fmt.Errorf("invalid duration %q: value too large", s)
	}
	ms := secs * 1000
	// If a '.' was present, the fractional part must be non-empty and digit-only.
	if hasDot {
		if !allDigits(frac) {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		// Normalize the fractional part to exactly three digits (truncate extra).
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		fv, _ := strconv.ParseInt(frac, 10, 64)
		ms += fv
	}
	return ms, nil
}

// allDigits reports whether the given string is non-empty and consists entirely
// of ASCII decimal digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// probeChapter mirrors one chapter entry from ffprobe's -show_chapters.
type probeChapter struct {
	ID        int    `json:"id"`
	TimeBase  string `json:"time_base"`
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Tags      struct {
		Title string `json:"title"`
	} `json:"tags"`
}

type probeChapters struct {
	Chapters []probeChapter `json:"chapters"`
}

// toleranceMs is the maximum acceptable difference between expected and
// verified chapter timestamps.
const toleranceMs = 1

// timebaseToMillis converts a raw clock value to milliseconds given an
// FFmpeg time_base string such as "1/1000" or "1/1000000000". It returns false
// if the time_base cannot be parsed. Formula: ms = value * 1000 / denom.
func timebaseToMillis(value int64, tb string) (int64, bool) {
	parts := strings.SplitN(tb, "/", 2)
	if len(parts) != 2 {
		return 0, false
	}
	num, err1 := strconv.ParseInt(parts[0], 10, 64)
	den, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, false
	}
	// ms = value * (num/den) * 1000 = value * num * 1000 / den
	return (value * num * 1000) / den, true
}

// verifyChapters uses ffprobe to confirm that the output file contains exactly
// the chapters we expected, with matching titles and start/end times.
func verifyChapters(outputPath string, expected []chapters.Chapter, durationMs int64) error {
	out, err := probeJSON("-show_chapters", outputPath)
	if err != nil {
		return fmt.Errorf("verification failed for %q: %v", outputPath, err)
	}
	var pc probeChapters
	if err := json.Unmarshal(out, &pc); err != nil {
		return fmt.Errorf("could not parse ffprobe chapter output for %q: %v", outputPath, err)
	}
	return compareChapters(pc.Chapters, expected, durationMs)
}

// compareChapters verifies that the probeChapter list matches the expected
// chapters in count, order, titles, and start/end times (within toleranceMs).
// It is pure and unit-testable without a real file.
func compareChapters(got []probeChapter, expected []chapters.Chapter, durationMs int64) error {
	if len(got) != len(expected) {
		return fmt.Errorf("verification failed: expected %d chapters, found %d",
			len(expected), len(got))
	}

	// Build the expected end timestamps (next start, or duration for the last).
	expectedEnds := make([]int64, len(expected))
	for i := range expected {
		if i+1 < len(expected) {
			expectedEnds[i] = expected[i+1].Start
		} else {
			expectedEnds[i] = durationMs
		}
	}

	for i := range expected {
		gotCh := got[i]
		if gotCh.Tags.Title != expected[i].Title {
			return fmt.Errorf("verification failed: chapter %d has title %q, expected %q",
				i+1, gotCh.Tags.Title, expected[i].Title)
		}
		gotStart, ok := timebaseToMillis(gotCh.Start, gotCh.TimeBase)
		if !ok {
			return fmt.Errorf("verification failed: could not interpret time_base %q for chapter %d",
				gotCh.TimeBase, i+1)
		}
		gotEnd, ok := timebaseToMillis(gotCh.End, gotCh.TimeBase)
		if !ok {
			return fmt.Errorf("verification failed: could not interpret time_base %q for chapter %d",
				gotCh.TimeBase, i+1)
		}
		if diff(expected[i].Start, gotStart) > toleranceMs {
			return fmt.Errorf("verification failed: chapter %d start is %s, expected %s",
				i+1, chapters.FormatMilliseconds(gotStart), chapters.FormatMilliseconds(expected[i].Start))
		}
		if diff(expectedEnds[i], gotEnd) > toleranceMs {
			return fmt.Errorf("verification failed: chapter %d end is %s, expected %s",
				i+1, chapters.FormatMilliseconds(gotEnd), chapters.FormatMilliseconds(expectedEnds[i]))
		}
	}
	return nil
}

func diff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

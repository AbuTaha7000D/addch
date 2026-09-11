package media

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/abutaha/addch/internal/chapters"
)

// ProbeJSON runs ffprobe with the given arguments and returns its stdout.
func ProbeJSON(args ...string) ([]byte, error) {
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

// GetVideoDurationMs returns the duration of a video in milliseconds using
// ffprobe. Zero or negative durations and unparseable values result in an error.
func GetVideoDurationMs(videoPath string) (int64, error) {
	out, err := ProbeJSON("-show_format", videoPath)
	if err != nil {
		return 0, fmt.Errorf("could not read video \"%s\": %v — is it a valid video file?", videoPath, err)
	}
	var pf probeFormat
	if err := json.Unmarshal(out, &pf); err != nil {
		return 0, fmt.Errorf("could not parse ffprobe output for %q: %v", videoPath, err)
	}
	if pf.Format.Duration == "" || pf.Format.Duration == "N/A" {
		return 0, fmt.Errorf("could not determine the duration of %q — is it a valid video file?", videoPath)
	}
	ms, err := ParseDurationToMs(pf.Format.Duration)
	if err != nil {
		return 0, fmt.Errorf("could not parse duration %q for %q: %v", pf.Format.Duration, videoPath, err)
	}
	if ms <= 0 {
		return 0, fmt.Errorf("could not determine the duration of %q — is it a valid video file?", videoPath)
	}
	return ms, nil
}

// ParseDurationToMs converts an FFprobe duration string like "5", "5.1",
// "5.123", "5.999" or "20.000000" into milliseconds using integer/decimal
// parsing only (no floating-point arithmetic). If more than three fractional
// digits are present, the value is truncated deterministically to millisecond
// precision (consistent with the chapter timestamp parser). Zero is allowed
// here; callers may reject it separately. Negative or non-numeric values are
// errors.
func ParseDurationToMs(s string) (int64, error) {
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

// ProbeChapter mirrors one chapter entry from ffprobe's -show_chapters.
type ProbeChapter struct {
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
	Chapters []ProbeChapter `json:"chapters"`
}

// ToleranceMs is the maximum acceptable difference between expected and
// verified chapter timestamps.
const ToleranceMs = 1

// TimebaseToMillis converts a raw clock value to milliseconds given an
// FFmpeg time_base string such as "1/1000" or "1/1000000000". It returns false
// if the time_base cannot be parsed. Formula: ms = value * 1000 / denom.
// The result is rounded to the nearest millisecond (rather than truncated)
// because FFmpeg writes timestamps with round-to-nearest when a non-millisecond
// time_base is in use (for example the 1/12800 movie timescale some encoders
// choose), so reading must round identically to recover the original value.
// Exact divisions (1/1000, 1/1000000000, ...) are unaffected by the rounding.
func TimebaseToMillis(value int64, tb string) (int64, bool) {
	parts := strings.SplitN(tb, "/", 2)
	if len(parts) != 2 {
		return 0, false
	}
	num, err1 := strconv.ParseInt(parts[0], 10, 64)
	den, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, false
	}
	// ms = value * (num/den) * 1000 = value * num * 1000 / den, rounded.
	return (value*num*1000 + den/2) / den, true
}

// VerifyChapters uses ffprobe to confirm that the output file contains exactly
// the chapters we expected, with matching titles and start/end times.
func VerifyChapters(outputPath string, expected []chapters.Chapter, durationMs int64) error {
	out, err := ProbeJSON("-show_chapters", outputPath)
	if err != nil {
		return fmt.Errorf("verification failed for %q: %v", outputPath, err)
	}
	var pc probeChapters
	if err := json.Unmarshal(out, &pc); err != nil {
		return fmt.Errorf("could not parse ffprobe chapter output for %q: %v", outputPath, err)
	}
	return CompareChapters(pc.Chapters, expected, durationMs)
}

// CompareChapters verifies that the ProbeChapter list matches the expected
// chapters in count, order, titles, and start/end times (within ToleranceMs).
// It is pure and unit-testable without a real file.
func CompareChapters(got []ProbeChapter, expected []chapters.Chapter, durationMs int64) error {
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
		gotStart, ok := TimebaseToMillis(gotCh.Start, gotCh.TimeBase)
		if !ok {
			return fmt.Errorf("verification failed: could not interpret time_base %q for chapter %d",
				gotCh.TimeBase, i+1)
		}
		gotEnd, ok := TimebaseToMillis(gotCh.End, gotCh.TimeBase)
		if !ok {
			return fmt.Errorf("verification failed: could not interpret time_base %q for chapter %d",
				gotCh.TimeBase, i+1)
		}
		if diff(expected[i].Start, gotStart) > ToleranceMs {
			return fmt.Errorf("verification failed: chapter %d start is %s, expected %s",
				i+1, chapters.FormatMilliseconds(gotStart), chapters.FormatMilliseconds(expected[i].Start))
		}
		if diff(expectedEnds[i], gotEnd) > ToleranceMs {
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

// ProbeChapters extracts the chapter markers of a media file exactly as ffprobe
// reports them (in probe order, converted to milliseconds). It returns an empty
// slice for a file with no chapters. Unlike CountChapters, which only cares how
// many there are, this is the faithful getch analogue: chapters are picked up
// verbatim with no normalization, validation, reordering, zero-first insertion,
// or title truncation.
func ProbeChapters(path string) ([]chapters.Chapter, error) {
	out, err := ProbeJSON("-show_chapters", path)
	if err != nil {
		return nil, fmt.Errorf("could not read chapters for %q: %v", path, err)
	}
	var pc probeChapters
	if err := json.Unmarshal(out, &pc); err != nil {
		return nil, fmt.Errorf("could not parse ffprobe chapter output for %q: %v", path, err)
	}
	return ChaptersFromProbe(pc.Chapters)
}

// ChaptersFromProbe converts ffprobe chapter entries into the toolkit's chapter
// model, preserving probe order and each chapter's verbatim title. Only the
// start time (converted from the container's own time_base) and title are
// carried over; the end time is deliberately dropped because a faithful extract
// reproduces chapters from their start markers alone. A chapter whose start
// cannot be interpreted (unparseable time_base) is an error: a probe result
// that cannot be turned into milliseconds must fail loudly rather than guess.
func ChaptersFromProbe(chs []ProbeChapter) ([]chapters.Chapter, error) {
	out := make([]chapters.Chapter, 0, len(chs))
	for _, c := range chs {
		start, ok := TimebaseToMillis(c.Start, c.TimeBase)
		if !ok {
			return nil, fmt.Errorf("could not interpret time_base %q at start %d", c.TimeBase, c.Start)
		}
		out = append(out, chapters.Chapter{Start: start, Title: c.Tags.Title})
	}
	return out, nil
}

// CountChapters returns the number of chapter markers in a media file as
// reported by ffprobe -show_chapters. This is the chapter-stripping (rmch)
// analogue of the embed-side probe; it is also used up front to decide whether
// an input has any chapters to remove.
func CountChapters(path string) (int, error) {
	out, err := ProbeJSON("-show_chapters", path)
	if err != nil {
		return 0, fmt.Errorf("could not read chapters for %q: %v", path, err)
	}
	var pc probeChapters
	if err := json.Unmarshal(out, &pc); err != nil {
		return 0, fmt.Errorf("could not parse ffprobe chapter output for %q: %v", path, err)
	}
	return len(pc.Chapters), nil
}

// VerifyNoChapters confirms that a chapter-stripped output has zero chapters,
// as reported by ffprobe -show_chapters. The contract is zero chapters only; a
// residual MP4 bin_data/data stream (a chapter-track artifact) is acceptable.
func VerifyNoChapters(outputPath string) error {
	n, err := CountChapters(outputPath)
	if err != nil {
		return fmt.Errorf("verification failed for %q: %v", outputPath, err)
	}
	return CompareNoChapters(n)
}

// CompareNoChapters is the pure counterpart of VerifyNoChapters: it returns an
// error unless the reported chapter count is zero. Unit-testable without a real
// file or ffprobe.
func CompareNoChapters(count int) error {
	if count != 0 {
		return fmt.Errorf("verification failed: expected 0 chapters, found %d", count)
	}
	return nil
}

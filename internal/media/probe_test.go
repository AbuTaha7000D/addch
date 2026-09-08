package media

import (
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

func TestParseDurationToMs(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"5", 5000, false},          // integer seconds
		{"5.1", 5100, false},        // one fractional digit
		{"5.12", 5120, false},       // two fractional digits
		{"5.123", 5123, false},      // three fractional digits
		{"5.999", 5999, false},      // boundary ms
		{"20.000000", 20000, false}, // many fractional digits, truncation
		{"5.123456", 5123, false},   // >3 digits truncated to ms
		{"0", 0, false},             // zero allowed at parse level
		{"0.000", 0, false},
		{"3599.99", 3599990, false}, // boundary large value
		{"3600", 3600000, false},
		{"123.456", 123456, false},
		{"", 0, true},                    // empty
		{"N/A", 0, true},                 // missing
		{"-5", 0, true},                  // negative
		{"-0.5", 0, true},                // negative fraction
		{"abc", 0, true},                 // non-numeric
		{"5.x", 0, true},                 // invalid fraction
		{"5.", 0, true},                  // missing fraction digits
		{".5", 0, true},                  // missing integer part
		{"5..5", 0, true},                // multiple dots
		{"1e2", 0, true},                 // scientific notation rejected
		{"5.12.3", 0, true},              // extra dot
		{"9223372036854775807", 0, true}, // int64-max seconds overflows ms
		{"9223372036854775", 0, true},    // overflow via the ms multiplication
	}
	for _, c := range cases {
		got, err := ParseDurationToMs(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseDurationToMs(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDurationToMs(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDurationToMs(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTimebaseToMillis(t *testing.T) {
	cases := []struct {
		value int64
		tb    string
		want  int64
		ok    bool
	}{
		{0, "1/1000", 0, true},                   // MP4
		{5000, "1/1000", 5000, true},             // MP4 ms unchanged
		{2000000000, "1/1000000000", 2000, true}, // MKV ns -> ms
		{60500000000, "1/1000000000", 60500, true},
		{1000000, "1/1000000", 1000, true}, // us -> ms
		{0, "garbage", 0, false},
		{5, "1/0", 0, false},
	}
	for _, c := range cases {
		got, ok := TimebaseToMillis(c.value, c.tb)
		if ok != c.ok {
			t.Errorf("TimebaseToMillis(%d, %q) ok = %v, want %v", c.value, c.tb, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("TimebaseToMillis(%d, %q) = %d, want %d", c.value, c.tb, got, c.want)
		}
	}
}

func TestGetVideoDurationInvalidFile(t *testing.T) {
	// A nonexistent file should produce an error (ffprobe path failure).
	if _, err := GetVideoDurationMs("/nonexistent/file.mp4"); err == nil {
		t.Error("expected error for nonexistent video file")
	}
}

// TestChaptersFromProbe covers the pure probe-to-model conversion used by getch
// (and ProbeChapters): faithful ms conversion, verbatim titles, probe order, an
// empty-result empty slice, and a hard error on an unparseable time_base.
func TestChaptersFromProbe(t *testing.T) {
	probe := func(start int64, tb, title string) ProbeChapter {
		p := ProbeChapter{ID: 1, TimeBase: tb, Start: start}
		p.Tags.Title = title
		return p
	}

	t.Run("converts ms and everything is carried faithfully", func(t *testing.T) {
		in := []ProbeChapter{
			probe(0, "1/1000", "Intro"),
			probe(3000, "1/1000", "Middle"),
		}
		got, err := ChaptersFromProbe(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []chapters.Chapter{{Start: 0, Title: "Intro"}, {Start: 3000, Title: "Middle"}}
		if len(got) != len(want) {
			t.Fatalf("got %d chapters, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i].Start != want[i].Start || got[i].Title != want[i].Title {
				t.Errorf("chapter %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("ns time_base is converted to ms", func(t *testing.T) {
		got, err := ChaptersFromProbe([]ProbeChapter{probe(60500000000, "1/1000000000", "Late")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[0].Start != 60500 {
			t.Errorf("Start = %d, want 60500", got[0].Start)
		}
	})

	t.Run("order is preserved verbatim, no zero-first or dedup", func(t *testing.T) {
		in := []ProbeChapter{
			probe(5000, "1/1000", "Late"),
			probe(1000, "1/1000", "Early"),
			probe(1000, "1/1000", "DupStart"),
			probe(3000, "1/1000", ""),
		}
		got, err := ChaptersFromProbe(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []chapters.Chapter{
			{Start: 5000, Title: "Late"},
			{Start: 1000, Title: "Early"},
			{Start: 1000, Title: "DupStart"},
			{Start: 3000, Title: ""},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d chapters, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i].Start != want[i].Start || got[i].Title != want[i].Title {
				t.Errorf("chapter %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("empty probe result yields an empty non-nil slice", func(t *testing.T) {
		got, err := ChaptersFromProbe(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("got %#v, want an empty non-nil slice", got)
		}
	})

	t.Run("unparseable time_base is an error", func(t *testing.T) {
		_, err := ChaptersFromProbe([]ProbeChapter{probe(5, "garbage", "Bad")})
		if err == nil {
			t.Fatal("expected error for unparseable time_base")
		}
		if _, ok := TimebaseToMillis(5, "garbage"); ok {
			t.Fatal("sanity: TimebaseToMillis should reject garbage")
		}
	})
}

// TestProbeChaptersNonexistent verifies that the ffprobe-backed extractor fails
// on a path that cannot be read (the same contract as GetVideoDurationMs).
func TestProbeChaptersNonexistent(t *testing.T) {
	if _, err := ProbeChapters("/nonexistent/file.mp4"); err == nil {
		t.Error("expected error for nonexistent video file")
	}
}

func TestCompareChapters(t *testing.T) {
	expected := []chapters.Chapter{
		{Start: 0, Title: "Intro", Line: 1},
		{Start: 5000, Title: "Part", Line: 2},
	}
	duration := int64(10000)

	chapter := func(id int, startMs, endMs int64, title string) ProbeChapter {
		// time_base 1/1000 so raw start/end == milliseconds.
		p := ProbeChapter{ID: id, TimeBase: "1/1000", Start: startMs, End: endMs}
		p.Tags.Title = title
		return p
	}

	// Expected ends: chapter 0 ends at 5000 (next start), chapter 1 ends at duration.
	match := []ProbeChapter{chapter(0, 0, 5000, "Intro"), chapter(1, 5000, 10000, "Part")}
	if err := CompareChapters(match, expected, duration); err != nil {
		t.Errorf("expected match to pass, got: %v", err)
	}

	// Wrong count.
	if err := CompareChapters(match[:1], expected, duration); err == nil {
		t.Error("expected error for wrong chapter count")
	}

	// Wrong title.
	wrongTitle := []ProbeChapter{chapter(0, 0, 5000, "Wrong"), chapter(1, 5000, 10000, "Part")}
	if err := CompareChapters(wrongTitle, expected, duration); err == nil {
		t.Error("expected error for wrong title")
	}
	if err := CompareChapters(wrongTitle, expected, duration); !strings.Contains(err.Error(), "title") {
		t.Errorf("title error should mention title: %v", err)
	}

	// Start off by more than tolerance.
	badStart := []ProbeChapter{chapter(0, 0, 5000, "Intro"), chapter(1, 5010, 10000, "Part")}
	if err := CompareChapters(badStart, expected, duration); err == nil {
		t.Error("expected error for start outside tolerance")
	}

	// Start within tolerance passes.
	withinTol := []ProbeChapter{chapter(0, 0, 5000, "Intro"), chapter(1, 5001, 10000, "Part")}
	if err := CompareChapters(withinTol, expected, duration); err != nil {
		t.Errorf("expected within-tolerance start to pass, got: %v", err)
	}

	// Invalid time_base.
	badTB := []ProbeChapter{{ID: 0, TimeBase: "garbage", Start: 0}, chapter(1, 5000, 10000, "Part")}
	if err := CompareChapters(badTB, expected, duration); err == nil {
		t.Error("expected error for invalid time_base")
	}
}

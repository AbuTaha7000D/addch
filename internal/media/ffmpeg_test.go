package media

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

func TestBuildMetadata(t *testing.T) {
	chs := []chapters.Chapter{
		{Start: 0, Title: "Intro", Line: 1},
		{Start: 330000, Title: "Chapter 2", Line: 2},
		{Start: 6150000, Title: "Final", Line: 3},
	}
	got := BuildMetadata(chs, 6150000)

	want := `;FFMETADATA1

[CHAPTER]
TIMEBASE=1/1000
START=0
END=330000
title=Intro

[CHAPTER]
TIMEBASE=1/1000
START=330000
END=6150000
title=Chapter 2

[CHAPTER]
TIMEBASE=1/1000
START=6150000
END=6150000
title=Final
`
	if got != want {
		t.Errorf("BuildMetadata mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMetadataLastEndsAtDuration(t *testing.T) {
	chs := []chapters.Chapter{
		{Start: 0, Title: "Intro", Line: 1},
		{Start: 3000, Title: "Second", Line: 2},
	}
	// Duration is longer than the last chapter start.
	got := BuildMetadata(chs, 10000)
	if !strings.Contains(got, "END=10000\n") {
		t.Errorf("expected last chapter END to equal duration 10000:\n%s", got)
	}
}

func TestBuildMetadataSingleChapter(t *testing.T) {
	chs := []chapters.Chapter{{Start: 0, Title: "Whole", Line: 1}}
	got := BuildMetadata(chs, 50000)
	if !strings.Contains(got, "START=0\n") || !strings.Contains(got, "END=50000\n") {
		t.Errorf("single chapter metadata incorrect:\n%s", got)
	}
	if strings.Count(got, "[CHAPTER]") != 1 {
		t.Errorf("expected exactly 1 chapter block, got:\n%s", got)
	}
}

func TestBuildMetadataDoesNotMutateInput(t *testing.T) {
	chs := []chapters.Chapter{
		{Start: 5000, Title: "b", Line: 2},
		{Start: 0, Title: "a", Line: 1},
	}
	_ = BuildMetadata(chs, 10000)
	// Ensure original slice ordering is unchanged.
	if chs[0].Start != 5000 || chs[1].Start != 0 {
		t.Error("BuildMetadata mutated the input slice")
	}
}

func TestBuildMetadataSorts(t *testing.T) {
	chs := []chapters.Chapter{
		{Start: 5000, Title: "later", Line: 2},
		{Start: 0, Title: "earlier", Line: 1},
	}
	got := BuildMetadata(chs, 10000)
	iLater := strings.Index(got, "title=later")
	iEarlier := strings.Index(got, "title=earlier")
	if iEarlier == -1 || iLater == -1 || iEarlier > iLater {
		t.Errorf("chapters not sorted:\n%s", got)
	}
}

func TestEscapeMetadataTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain title", "plain title"},
		{`Back\slash`, `Back\\slash`},
		{`a\b c`, `a\\b c`}, // a literal backslash-b must survive
		{`Win\FilePath`, `Win\\FilePath`},
		{`trailing\`, `trailing\\`},
		{"emoji 🎬 arab اختبار", "emoji 🎬 arab اختبار"},
		{`eq=bracket[ok] &% ;semi #hash`, `eq=bracket[ok] &% ;semi #hash`},
	}
	for _, c := range cases {
		if got := EscapeMetadataTitle(c.in); got != c.want {
			t.Errorf("EscapeMetadataTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildMetadataEscapesBackslash(t *testing.T) {
	chs := []chapters.Chapter{{Start: 0, Title: `Back\slash`, Line: 1}}
	got := BuildMetadata(chs, 1000)
	if !strings.Contains(got, `title=Back\\slash`) {
		t.Errorf("expected escaped backslash in metadata:\n%s", got)
	}
	if strings.Contains(got, "title=Back\\slash\n") {
		t.Errorf("raw (unescaped) backslash must not be written:\n%s", got)
	}
}

func TestBuildMetadataSpecialChars(t *testing.T) {
	// '=', ';' and '#' must pass through unchanged; only backslash is escaped.
	chs := []chapters.Chapter{
		{Start: 0, Title: "eq=a ;semi #hash", Line: 1},
		{Start: 5000, Title: `back\slash`, Line: 2},
	}
	got := BuildMetadata(chs, 10000)
	if !strings.Contains(got, "title=eq=a ;semi #hash\n") {
		t.Errorf("expected = ; # unchanged:\n%s", got)
	}
	if !strings.Contains(got, `title=back\\slash`+"\n") {
		t.Errorf("expected backslash escaped:\n%s", got)
	}
}

func TestIsMP4Family(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".mp4", true},
		{".m4v", true},
		{".mov", true},
		{".3gp", true},
		{".3g2", true},
		{".MP4", true},
		{".mkv", false},
		{".webm", false},
		{".avi", false},
		{"", false},
		{".mp3", false},
	}
	for _, c := range cases {
		if got := IsMP4Family(c.ext); got != c.want {
			t.Errorf("IsMP4Family(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}

func TestBuildFFmpegArgs(t *testing.T) {
	has := func(args []string, needle string) bool {
		for _, a := range args {
			if a == needle {
				return true
			}
		}
		return false
	}

	// Overwrite => -y present; no overwrite => -y absent.
	argsOver := BuildFFmpegArgs("in.mp4", "meta.txt", "out.mp4", ".mp4", true)
	if !has(argsOver, "-y") {
		t.Errorf("expected -y when overwrite is true: %v", argsOver)
	}
	argsNo := BuildFFmpegArgs("in.mp4", "meta.txt", "out.mp4", ".mp4", false)
	if has(argsNo, "-y") {
		t.Errorf("-y must NOT be present without --overwrite: %v", argsNo)
	}

	// Core flags always present.
	for _, flag := range []string{"-map", "0", "-map_metadata", "0", "-map_chapters", "1", "-c", "copy"} {
		if !has(argsNo, flag) {
			t.Errorf("missing required flag %q in %v", flag, argsNo)
		}
	}

	// faststart only for MP4-family.
	if !has(argsOver, "+faststart") {
		t.Errorf("expected +faststart for .mp4: %v", argsOver)
	}
	mkvArgs := BuildFFmpegArgs("in.mp4", "meta.txt", "out.mkv", ".mkv", false)
	if has(mkvArgs, "+faststart") {
		t.Errorf("+faststart must NOT be added for .mkv: %v", mkvArgs)
	}
	// Case-insensitive extension handling.
	if !has(BuildFFmpegArgs("in", "m", "out.MP4", ".MP4", false), "+faststart") {
		t.Error("expected +faststart for .MP4 (case-insensitive)")
	}
}

func TestBuildStripArgs(t *testing.T) {
	has := func(args []string, needle string) bool {
		for _, a := range args {
			if a == needle {
				return true
			}
		}
		return false
	}

	// Overwrite => -y present; no overwrite => -y absent.
	argsOver := BuildStripArgs("in.mp4", "out.mp4", ".mp4", true)
	if !has(argsOver, "-y") {
		t.Errorf("expected -y when overwrite is true: %v", argsOver)
	}
	argsNo := BuildStripArgs("in.mp4", "out.mp4", ".mp4", false)
	if has(argsNo, "-y") {
		t.Errorf("-y must NOT be present without --overwrite: %v", argsNo)
	}

	// The strip contract: all streams, metadata preserved, chapters disabled.
	for _, flag := range []string{"-map", "0", "-map_metadata", "0", "-map_chapters", "-1", "-c", "copy"} {
		if !has(argsNo, flag) {
			t.Errorf("missing required flag %q in %v", flag, argsNo)
		}
	}
	if got := stripChaptersValue(argsNo); got != "-1" {
		t.Errorf("-map_chapters value = %q, want -1: %v", got, argsNo)
	}

	// faststart only for MP4-family.
	if !has(argsOver, "+faststart") {
		t.Errorf("expected +faststart for .mp4: %v", argsOver)
	}
	mkvArgs := BuildStripArgs("in.mp4", "out.mkv", ".mkv", false)
	if has(mkvArgs, "+faststart") {
		t.Errorf("+faststart must NOT be added for .mkv: %v", mkvArgs)
	}
	if !has(BuildStripArgs("in", "out.MP4", ".MP4", false), "+faststart") {
		t.Error("expected +faststart for .MP4 (case-insensitive)")
	}
}

// stripChaptersValue returns the value paired with "-map_chapters" in args,
// used by TestBuildStripArgs to pin the -1 requirement.
func stripChaptersValue(args []string) string {
	for i, a := range args {
		if a == "-map_chapters" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestVerifyNoChaptersPure(t *testing.T) {
	if err := CompareNoChapters(0); err != nil {
		t.Errorf("expected count 0 to pass, got %v", err)
	}
	for _, n := range []int{1, 3} {
		err := CompareNoChapters(n)
		if err == nil {
			t.Errorf("expected count %d to fail", n)
			continue
		}
		if !strings.Contains(err.Error(), "expected 0 chapters") {
			t.Errorf("unexpected error wording: %v", err)
		}
	}
}

func TestCountChaptersInvalidFile(t *testing.T) {
	if _, err := CountChapters("/nonexistent/file.mp4"); err == nil {
		t.Error("expected error counting chapters of a nonexistent file")
	}
}

func TestRemuxProcessInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix 'sleep' command")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	rp := NewRemuxProcess(cmd)
	// Interrupt() must kill the child and block until it is fully reaped.
	// Exited() is true for a normal exit; a signal-killed child reports ExitCode -1.
	rp.Interrupt()
	ps := rp.cmd.ProcessState
	if ps == nil || (!ps.Exited() && ps.ExitCode() != -1) {
		t.Fatal("child process did not exit after Interrupt()")
	}
}

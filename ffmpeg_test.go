package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

func TestBuildMetadata(t *testing.T) {
	chapters := []chapters.Chapter{
		{Start: 0, Title: "Intro", Line: 1},
		{Start: 330000, Title: "Chapter 2", Line: 2},
		{Start: 6150000, Title: "Final", Line: 3},
	}
	got := buildMetadata(chapters, 6150000)

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
		t.Errorf("buildMetadata mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMetadataLastEndsAtDuration(t *testing.T) {
	chapters := []chapters.Chapter{
		{Start: 0, Title: "Intro", Line: 1},
		{Start: 3000, Title: "Second", Line: 2},
	}
	// Duration is longer than the last chapter start.
	got := buildMetadata(chapters, 10000)
	if !strings.Contains(got, "END=10000\n") {
		t.Errorf("expected last chapter END to equal duration 10000:\n%s", got)
	}
}

func TestBuildMetadataSingleChapter(t *testing.T) {
	chapters := []chapters.Chapter{{Start: 0, Title: "Whole", Line: 1}}
	got := buildMetadata(chapters, 50000)
	if !strings.Contains(got, "START=0\n") || !strings.Contains(got, "END=50000\n") {
		t.Errorf("single chapter metadata incorrect:\n%s", got)
	}
	if strings.Count(got, "[CHAPTER]") != 1 {
		t.Errorf("expected exactly 1 chapter block, got:\n%s", got)
	}
}

func TestBuildMetadataDoesNotMutateInput(t *testing.T) {
	chapters := []chapters.Chapter{
		{Start: 5000, Title: "b", Line: 2},
		{Start: 0, Title: "a", Line: 1},
	}
	_ = buildMetadata(chapters, 10000)
	// Ensure original slice ordering is unchanged.
	if chapters[0].Start != 5000 || chapters[1].Start != 0 {
		t.Error("buildMetadata mutated the input slice")
	}
}

func TestBuildMetadataSorts(t *testing.T) {
	chapters := []chapters.Chapter{
		{Start: 5000, Title: "later", Line: 2},
		{Start: 0, Title: "earlier", Line: 1},
	}
	got := buildMetadata(chapters, 10000)
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
		if got := escapeMetadataTitle(c.in); got != c.want {
			t.Errorf("escapeMetadataTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildMetadataEscapesBackslash(t *testing.T) {
	chapters := []chapters.Chapter{{Start: 0, Title: `Back\slash`, Line: 1}}
	got := buildMetadata(chapters, 1000)
	if !strings.Contains(got, `title=Back\\slash`) {
		t.Errorf("expected escaped backslash in metadata:\n%s", got)
	}
	if strings.Contains(got, "title=Back\\slash\n") {
		t.Errorf("raw (unescaped) backslash must not be written:\n%s", got)
	}
}

func TestBuildMetadataSpecialChars(t *testing.T) {
	// '=', ';' and '#' must pass through unchanged; only backslash is escaped.
	chapters := []chapters.Chapter{
		{Start: 0, Title: "eq=a ;semi #hash", Line: 1},
		{Start: 5000, Title: `back\slash`, Line: 2},
	}
	got := buildMetadata(chapters, 10000)
	if !strings.Contains(got, "title=eq=a ;semi #hash\n") {
		t.Errorf("expected = ; # unchanged:\n%s", got)
	}
	if !strings.Contains(got, `title=back\\slash`+"\n") {
		t.Errorf("expected backslash escaped:\n%s", got)
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
	argsOver := buildFFmpegArgs("in.mp4", "meta.txt", "out.mp4", ".mp4", true)
	if !has(argsOver, "-y") {
		t.Errorf("expected -y when overwrite is true: %v", argsOver)
	}
	argsNo := buildFFmpegArgs("in.mp4", "meta.txt", "out.mp4", ".mp4", false)
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
	mkvArgs := buildFFmpegArgs("in.mp4", "meta.txt", "out.mkv", ".mkv", false)
	if has(mkvArgs, "+faststart") {
		t.Errorf("+faststart must NOT be added for .mkv: %v", mkvArgs)
	}
	// Case-insensitive extension handling.
	if !has(buildFFmpegArgs("in", "m", "out.MP4", ".MP4", false), "+faststart") {
		t.Error("expected +faststart for .MP4 (case-insensitive)")
	}
}

func TestFFmpegRemuxInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix 'sleep' command")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	rp := newFFmpegRemux(cmd)
	// interrupt() must kill the child and block until it is fully reaped.
	// Exited() is true for a normal exit; a signal-killed child reports ExitCode -1.
	rp.interrupt()
	ps := rp.cmd.ProcessState
	if ps == nil || (!ps.Exited() && ps.ExitCode() != -1) {
		t.Fatal("child process did not exit after interrupt()")
	}
}

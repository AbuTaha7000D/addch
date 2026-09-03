package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireTools skips integration tests if ffmpeg/ffprobe are unavailable.
func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("integration test skipped: %s not found in PATH", tool)
		}
	}
}

// makeTestVideo creates a small, real video file and returns its path.
func makeTestVideo(t *testing.T, dir string, durationSec string) string {
	t.Helper()
	path := filepath.Join(dir, "source.mp4")
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=" + durationSec + ":size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=" + durationSec,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p",
		"-shortest",
		path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not create test video: %v\n%s", err, out)
	}
	return path
}

// runEmbedForTest drives the full embed pipeline for real file paths, capturing
// output into string buffers and returning the exit code.
func runEmbedForTest(chaptersFile, video, output string, overwrite bool) (string, string, int) {
	pa := &parsedArgs{chapters: chaptersFile, video: video, output: output, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runEmbed(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestEndToEnd(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "20") // 20 second video

	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00 Intro\n00:00:05.500 Precise Start\n00:00:10 Second\n"
	if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	expectedOut := defaultOutputPath(video)
	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("runEmbed failed with code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(expectedOut); err != nil {
		t.Fatalf("expected output file %q not created", expectedOut)
	}

	chapters := probeChaptersRaw(t, expectedOut)
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, found %d", len(chapters))
	}
	if chapters[0].Tags.Title != "Intro" || chapters[0].Start != 0 {
		t.Errorf("chapter 0 mismatch: %+v", chapters[0])
	}
	if chapters[1].Start != 5500 {
		t.Errorf("chapter 1 start %d, want 5500 (ms precision)", chapters[1].Start)
	}
	if chapters[2].Start != 10000 || chapters[2].End != 20000 {
		t.Errorf("chapter 2 start/end mismatch: %+v (want start 10000, end 20000)", chapters[2])
	}
}

func TestEndToEndDoesNotModifySource(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	before, err := os.Stat(video)
	if err != nil {
		t.Fatal(err)
	}

	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("runEmbed failed: %s", errStr)
	}
	after, err := os.Stat(video)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		t.Error("source video was modified")
	}
}

func TestEndToEndRejectsOverwriteWithoutFlag(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	if _, _, code := runEmbedForTest(chaptersFile, video, "", false); code != 0 {
		t.Fatalf("first run should succeed, got code %d", code)
	}
	// Second run without --overwrite should fail, and existing output remains.
	if _, _, code := runEmbedForTest(chaptersFile, video, "", false); code == 0 {
		t.Fatal("second run should fail without --overwrite")
	}
}

func TestEndToEndOverwriteFlag(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	if _, _, code := runEmbedForTest(chaptersFile, video, "", false); code != 0 {
		t.Fatalf("first run failed with code %d", code)
	}
	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", true); code != 0 {
		t.Fatalf("overwrite run failed with code %d:\n%s", code, errStr)
	}
}

func TestEndToEndBackslashInTitle(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "15")

	// Titles containing a literal backslash must survive the FFMETADATA1
	// round-trip unchanged. FFmpeg treats '\' as an escape in the metadata file,
	// so we must double it when writing; verify the user's original string comes
	// back intact (and special characters still work).
	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00 Back\\slash test\n" +
		"00:00:05 Win\\File\\Path & eq=b\n" +
		"00:00:10 Emoji 🎬\n"
	if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("runEmbed failed with code %d:\n%s", code, errStr)
	}

	outPath := defaultOutputPath(video)
	got := probeChaptersRaw(t, outPath)
	want := []string{`Back\slash test`, `Win\File\Path & eq=b`, "Emoji 🎬"}
	if len(got) != len(want) {
		t.Fatalf("expected %d chapters, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].Tags.Title != w {
			t.Errorf("chapter %d title = %q, want %q", i, got[i].Tags.Title, w)
		}
	}
}

func TestEndToEndTitleEndingInBackslashFails(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "15")

	// FFmpeg's FFMETADATA format cannot represent a value ending in a backslash:
	// the trailing '\' escapes the line's newline, which is then absorbed into the
	// title, so the intended string never round-trips. addch must reject such a
	// title up front with a clear message rather than writing a corrupt chapter.
	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00 Intro\n" +
		"00:00:05 EndsWith\\Backslash\\\n" +
		"00:00:10 Final\n"
	if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code == 0 {
		t.Fatal("expected a title ending in a backslash to be rejected")
	}
	if !strings.Contains(errStr, "must not end with a backslash") {
		t.Errorf("unexpected error message: %s", errStr)
	}

	// The failed run must not leave any output or temp metadata behind.
	if _, err := os.Stat(defaultOutputPath(video)); !os.IsNotExist(err) {
		t.Error("no output file should exist after the rejected run")
	}
	if countTempMetadata() != 0 {
		t.Error("temporary metadata file was left behind after the rejected run")
	}
}

func TestEndToEndChapterBeyondDurationFails(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	// 11 seconds > 10 second video.
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:11 B\n"), 0o644)

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code == 0 {
		t.Fatal("expected failure for chapter beyond duration")
	}
	if !strings.Contains(errStr, "exceeds the video duration") {
		t.Errorf("unexpected error message: %s", errStr)
	}
}

func TestEndToEndMKVContainer(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	mp4 := makeTestVideo(t, dir, "15")
	mkv := filepath.Join(dir, "source.mkv")
	// Transmux the mp4 into an mkv so we exercise the non-MP4 code path
	// (time_base in nanoseconds + no -movflags +faststart).
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-i", mp4, "-c", "copy", mkv).CombinedOutput(); err != nil {
		t.Fatalf("could not create mkv: %v\n%s", err, out)
	}

	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05.250 B\n00:00:10 C\n"), 0o644)

	_, errStr, code := runEmbedForTest(chaptersFile, mkv, "", false)
	if code != 0 {
		t.Fatalf("mkv run failed with code %d:\n%s", code, errStr)
	}

	outPath := defaultOutputPath(mkv)
	chapters := probeChaptersRaw(t, outPath)
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if got := millisecondsOf(t, chapters[1]); got != 5250 {
		t.Errorf("chapter 1 start %d ms, want 5250", got)
	}
}

// millisecondsOf converts a probeChapter's start to milliseconds using its
// time_base, mirroring the production verification logic.
func millisecondsOf(t *testing.T, c probeChapter) int64 {
	t.Helper()
	ms, ok := timebaseToMillis(c.Start, c.TimeBase)
	if !ok {
		t.Fatalf("could not convert time_base %q", c.TimeBase)
	}
	return ms
}

// probeChaptersRaw runs ffprobe -show_chapters and returns the parsed chapters.
func probeChaptersRaw(t *testing.T, path string) []probeChapter {
	t.Helper()
	out, err := probeJSON("-show_chapters", path)
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	var pc probeChapters
	if err := json.Unmarshal(out, &pc); err != nil {
		t.Fatalf("parse chapters: %v", err)
	}
	return pc.Chapters
}

// countTempMetadata returns the number of addch temp metadata files currently
// in the system temp directory.
func countTempMetadata() int {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "addch-metadata-*"))
	return len(matches)
}

func TestEndToEndOverwriteNeverTouchesInput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	before, _ := os.Stat(video)

	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	// First run produces output, then an overwrite run replaces the output.
	if _, _, code := runEmbedForTest(chaptersFile, video, "", false); code != 0 {
		t.Fatalf("first run failed, code %d", code)
	}
	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", true); code != 0 {
		t.Fatalf("overwrite run failed with code %d:\n%s", code, errStr)
	}

	after, _ := os.Stat(video)
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		t.Error("input video was modified by --overwrite run")
	}
}

func TestEndToEndPreservesChapterFileAndCleansTemp(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00 Intro\n00:00:05.250 Precise\n00:00:09 End\n"
	os.WriteFile(chaptersFile, []byte(content), 0o644)
	before, _ := os.ReadFile(chaptersFile)

	tempBefore := countTempMetadata()

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("run failed with code %d:\n%s", code, errStr)
	}

	// The user's chapter file is untouched, byte-for-byte.
	after, err := os.ReadFile(chaptersFile)
	if err != nil {
		t.Fatalf("chapter file missing after run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("user chapter file contents changed")
	}

	// Output contains the expected chapters.
	outPath := defaultOutputPath(video)
	chapters := probeChaptersRaw(t, outPath)
	if len(chapters) != 3 || chapters[1].Tags.Title != "Precise" {
		t.Errorf("output chapters wrong: %+v", chapters)
	}

	// No temp metadata file was left behind.
	if countTempMetadata() > tempBefore {
		t.Error("temporary metadata file was not cleaned up")
	}
}

func TestEndToEndSameBaseNameVariants(t *testing.T) {
	requireTools(t)

	cases := []struct {
		inVideo string
		content string
		wantOut string
	}{
		{"video.mp4", "00:00:00 A\n", "video-chapters.mp4"},
		{"My Course.mp4", "00:00:00 A\n", "My Course-chapters.mp4"},
		{"course.part1.mp4", "00:00:00 A\n", "course.part1-chapters.mp4"},
		{"فيديو.mp4", "00:00:00 A\n", "فيديو-chapters.mp4"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "8")
		// Rename the generated source to the case-specific video name.
		videoPath := filepath.Join(dir, c.inVideo)
		if err := os.Rename(video, videoPath); err != nil {
			t.Fatalf("rename video: %v", err)
		}
		// Same base name chapter file.
		chaptersFile := filepath.Join(dir, "video.txt")
		os.WriteFile(chaptersFile, []byte(c.content), 0o644)
		videoBytes, _ := os.ReadFile(videoPath)
		chapterBytes, _ := os.ReadFile(chaptersFile)

		wantOut := filepath.Join(dir, c.wantOut)
		_, errStr, code := runEmbedForTest(chaptersFile, videoPath, "", false)
		if code != 0 {
			t.Fatalf("[%s] run failed code %d:\n%s", c.inVideo, code, errStr)
		}
		if _, err := os.Stat(wantOut); err != nil {
			t.Fatalf("[%s] expected output %q: %v", c.inVideo, wantOut, err)
		}
		// Both originals untouched.
		if v, _ := os.ReadFile(videoPath); !bytes.Equal(v, videoBytes) {
			t.Errorf("[%s] input video changed", c.inVideo)
		}
		if ch, _ := os.ReadFile(chaptersFile); !bytes.Equal(ch, chapterBytes) {
			t.Errorf("[%s] chapter file changed", c.inVideo)
		}
	}
}

func TestEndToEndCustomOutputCannotOverwriteChapterFile(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)
	orig, _ := os.ReadFile(chaptersFile)

	// Point the output at the chapter file itself: must be rejected and must not
	// modify the chapter file.
	_, errStr, code := runEmbedForTest(chaptersFile, video, chaptersFile, true)
	if code == 0 {
		t.Fatal("expected run with output == chapter file to fail")
	}
	if got, _ := os.ReadFile(chaptersFile); !bytes.Equal(got, orig) {
		t.Error("chapter file was modified even though output==input was refused")
	}
	_ = errStr

	// Point the output at the input video: must be rejected too (never overwrite input).
	if _, _, code := runEmbedForTest(chaptersFile, video, video, true); code == 0 {
		t.Fatal("expected run with output == video to fail")
	}
}

func TestEndToEndSpecialCharacterTitles(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "15")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00 eq=a ;semi #hash\n" +
		"00:00:05 Back\\slash test\n" +
		"00:00:10 arab اختبار emoji 🎬\n"
	os.WriteFile(chaptersFile, []byte(content), 0o644)

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("run failed code %d:\n%s", code, errStr)
	}

	got := probeChaptersRaw(t, defaultOutputPath(video))
	want := []string{"eq=a ;semi #hash", `Back\slash test`, "arab اختبار emoji 🎬"}
	if len(got) != len(want) {
		t.Fatalf("expected %d chapters, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].Tags.Title != w {
			t.Errorf("chapter %d title = %q, want %q", i, got[i].Tags.Title, w)
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
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

	expectedOut := fsutil.DefaultOutputPath(video)
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

	// ffprobe reports start/end as raw values in the container's time_base units
	// (e.g. 1/1000 for MP4 -> raw == ms, but other time_bases, e.g. on Windows,
	// differ). Convert to milliseconds via time_base so these assertions are
	// platform-independent. This mirrors the production verification in compareChapters.
	start := func(i int) int64 {
		ms, ok := media.TimebaseToMillis(chapters[i].Start, chapters[i].TimeBase)
		if !ok {
			t.Fatalf("could not convert time_base %q for chapter %d", chapters[i].TimeBase, i)
		}
		return ms
	}
	end := func(i int) int64 {
		ms, ok := media.TimebaseToMillis(chapters[i].End, chapters[i].TimeBase)
		if !ok {
			t.Fatalf("could not convert time_base %q for chapter %d", chapters[i].TimeBase, i)
		}
		return ms
	}

	if chapters[0].Tags.Title != "Intro" || start(0) != 0 {
		t.Errorf("chapter 0 mismatch: %+v", chapters[0])
	}
	if got := start(1); got != 5500 {
		t.Errorf("chapter 1 start %d, want 5500 (ms precision)", got)
	}
	if gotStart, gotEnd := start(2), end(2); gotStart != 10000 || gotEnd != 20000 {
		t.Errorf("chapter 2 start/end mismatch: start=%d end=%d (want start 10000, end 20000)", gotStart, gotEnd)
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

	outPath := fsutil.DefaultOutputPath(video)
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
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); !os.IsNotExist(err) {
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

	outPath := fsutil.DefaultOutputPath(mkv)
	chapters := probeChaptersRaw(t, outPath)
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if got := millisecondsOf(t, chapters[1]); got != 5250 {
		t.Errorf("chapter 1 start %d ms, want 5250", got)
	}
}

// millisecondsOf converts a media.ProbeChapter's start to milliseconds using its
// time_base, mirroring the production verification logic.
func millisecondsOf(t *testing.T, c media.ProbeChapter) int64 {
	t.Helper()
	ms, ok := media.TimebaseToMillis(c.Start, c.TimeBase)
	if !ok {
		t.Fatalf("could not convert time_base %q", c.TimeBase)
	}
	return ms
}

// probeChaptersRaw runs ffprobe -show_chapters and returns the parsed chapters.
func probeChaptersRaw(t *testing.T, path string) []media.ProbeChapter {
	t.Helper()
	out, err := media.ProbeJSON("-show_chapters", path)
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	var pc struct {
		Chapters []media.ProbeChapter `json:"chapters"`
	}
	if err := json.Unmarshal(out, &pc); err != nil {
		t.Fatalf("parse chapters: %v", err)
	}
	return pc.Chapters
}

// countTempMetadata returns the number of addch temp metadata files currently
// in the metadata temp directory. When ADDCH_METADATA_TMPDIR is set the count
// is scoped to that private directory (see media.WriteTempMetadata); otherwise
// the system temp directory is used.
func countTempMetadata() int {
	dir := os.Getenv("ADDCH_METADATA_TMPDIR")
	if dir == "" {
		dir = os.TempDir()
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "addch-metadata-*"))
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
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

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
	outPath := fsutil.DefaultOutputPath(video)
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

	got := probeChaptersRaw(t, fsutil.DefaultOutputPath(video))
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

// makeTestContainer creates a small, real video fixture in the requested
// container ("mp4" or "mkv") and returns its path. The mp4 is produced by the
// shared makeTestVideo helper; other containers are transmuxed from it with
// stream copy, exactly as the existing MKV coverage does.
func makeTestContainer(t *testing.T, dir, container, durationSec string) string {
	t.Helper()
	mp4 := makeTestVideo(t, dir, durationSec)
	if container == "mp4" {
		return mp4
	}
	out := filepath.Join(dir, "source."+container)
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", mp4, "-c", "copy", out}
	if o, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not create %s source fixture: %v\n%s", container, err, o)
	}
	return out
}

// endMsOf converts a probeChapter's end to milliseconds using its time_base,
// mirroring the production verification logic (same as millisecondsOf but for End).
func endMsOf(t *testing.T, c media.ProbeChapter) int64 {
	t.Helper()
	ms, ok := media.TimebaseToMillis(c.End, c.TimeBase)
	if !ok {
		t.Fatalf("could not convert end time_base %q", c.TimeBase)
	}
	return ms
}

// diff returns the absolute difference between two values.
func diff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestChapterRoundTripMP4AndMKV is the Phase 2A empirical verification that
// chapter metadata round-trips through real FFmpeg/FFprobe in both the MP4 and
// Matroska (MKV) containers. It embeds an adversarial chapter set (Arabic and
// Unicode, spaces, `=`, `;`, `#`, double quotes, non-final literal backslashes,
// and the literal two-character text sequences `\n`, `\t`, `\r`) and, via
// ffprobe, asserts the exact chapter count, exact titles, start timestamps
// converted from each chapter's reported time_base, and the end-chain
// invariant (each end equals the next chapter's start; the final end equals the
// source media duration). Copy-out safety is also re-asserted here: the source
// must remain byte-identical.
//
// The test skips only when ffmpeg/ffprobe are unavailable; any remux or
// verification failure while the tools are installed fails the test.
func TestChapterRoundTripMP4AndMKV(t *testing.T) {
	for _, container := range []string{"mp4", "mkv"} {
		t.Run(container, func(t *testing.T) {
			requireTools(t)

			dir := t.TempDir()
			video := makeTestContainer(t, dir, container, "10")

			wantStarts := []int64{0, 2500, 5250, 8000}
			wantTitles := []string{
				"مقدمة With Spaces and حروف",
				`eq=a ;semi #hash "double quoted"`,
				`Back\slash literal\nsequence`,
				`literal\tsequence literal\rreturn mixed العربية mixed`,
			}

			chaptersFile := filepath.Join(dir, "chapters.txt")
			content := "00:00:00.000 مقدمة With Spaces and حروف\n" +
				"00:00:02.500 eq=a ;semi #hash \"double quoted\"\n" +
				"00:00:05.250 Back\\slash literal\\nsequence\n" +
				"00:00:08.000 literal\\tsequence literal\\rreturn mixed العربية mixed\n"
			if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
				t.Fatalf("write chapters: %v", err)
			}

			srcBefore, err := os.ReadFile(video)
			if err != nil {
				t.Fatalf("read source before embed: %v", err)
			}

			_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
			if code != 0 {
				t.Fatalf("[%s] embed failed with code %d:\n%s", container, code, errStr)
			}

			// Copy-out safety: the source media must be untouched.
			if srcAfter, err := os.ReadFile(video); err != nil {
				t.Fatalf("read source after embed: %v", err)
			} else if !bytes.Equal(srcBefore, srcAfter) {
				t.Errorf("[%s] source video was modified", container)
			}

			durationMs, err := media.GetVideoDurationMs(video)
			if err != nil {
				t.Fatalf("[%s] could not read source duration: %v", container, err)
			}

			out := fsutil.DefaultOutputPath(video)
			chapters := probeChaptersRaw(t, out)

			if len(chapters) != len(wantTitles) {
				t.Fatalf("[%s] chapter count = %d, want %d", container, len(chapters), len(wantTitles))
			}
			t.Logf("[%s] chapter time_base = %q; source duration = %d ms",
				container, chapters[0].TimeBase, durationMs)

			for i, wantStart := range wantStarts {
				if got := millisecondsOf(t, chapters[i]); got != wantStart {
					t.Errorf("[%s] chapter %d start = %d ms, want %d ms", container, i, got, wantStart)
				}
				if chapters[i].Tags.Title != wantTitles[i] {
					t.Errorf("[%s] chapter %d title = %q, want %q", container, i, chapters[i].Tags.Title, wantTitles[i])
				}
			}

			// End-chain invariant: each chapter ends where the next one starts,
			// converted through the same time-base logic as the production verifier.
			for i := 0; i < len(chapters)-1; i++ {
				got := endMsOf(t, chapters[i])
				want := millisecondsOf(t, chapters[i+1])
				if got != want {
					t.Errorf("[%s] chapter %d end = %d ms, want next chapter start %d ms",
						container, i, got, want)
				}
			}

			// Final chapter end equals the target media (source) duration. A
			// 1 ms tolerance mirrors the production verifier's media.ToleranceMs and
			// absorbs container-level rounding of the duration probe.
			finalEnd := endMsOf(t, chapters[len(chapters)-1])
			if diff(finalEnd, durationMs) > media.ToleranceMs {
				t.Errorf("[%s] final chapter end = %d ms, want source duration %d ms (within %d ms)",
					container, finalEnd, durationMs, media.ToleranceMs)
			}
		})
	}
}

// makeTestAudio creates a small, real M4A fixture containing AAC audio only
// (no video), synthesized deterministically from a lavfi sine input.
func makeTestAudio(t *testing.T, dir, durationSec string) string {
	t.Helper()
	path := filepath.Join(dir, "source.m4a")
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=" + durationSec,
		"-c:a", "aac",
		path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not create test audio: %v\n%s", err, out)
	}
	return path
}

// TestChapterRoundTripM4A is the Phase 2C empirical verification that chapter
// metadata round-trips through an M4A/AAC audio container. It embeds a valid
// chapter set (Arabic/Unicode, punctuation, quotes, and a non-final literal
// backslash, with a non-zero millisecond timestamp) and, via FFprobe, asserts
// the exact chapter count, exact titles, start timestamps converted through
// each returned chapter's time_base, the end-chain invariant (each end equals
// the next start; the final end equals the probed source duration within
// media.ToleranceMs), and that the source M4A remains byte-identical.
//
// The test skips only when ffmpeg/ffprobe are unavailable; if the tools exist
// but M4A remuxing or verification fails, the test fails rather than skipping.
func TestChapterRoundTripM4A(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	m4a := makeTestAudio(t, dir, "10")

	wantStarts := []int64{0, 2500, 5250, 8000}
	wantTitles := []string{
		"مقدمة مقطع صوتي",
		`eq=a ;semi #hash "double quoted"`,
		`Back\slash literal`,
		"Final",
	}

	chaptersFile := filepath.Join(dir, "chapters.txt")
	content := "00:00:00.000 مقدمة مقطع صوتي\n" +
		"00:00:02.500 eq=a ;semi #hash \"double quoted\"\n" +
		"00:00:05.250 Back\\slash literal\n" +
		"00:00:08.000 Final\n"
	if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	srcBefore, err := os.ReadFile(m4a)
	if err != nil {
		t.Fatalf("read source before embed: %v", err)
	}

	_, errStr, code := runEmbedForTest(chaptersFile, m4a, "", false)
	if code != 0 {
		t.Fatalf("m4a embed failed with code %d:\n%s", code, errStr)
	}

	// Copy-out safety: the source audio must be untouched.
	if srcAfter, err := os.ReadFile(m4a); err != nil {
		t.Fatalf("read source after embed: %v", err)
	} else if !bytes.Equal(srcBefore, srcAfter) {
		t.Errorf("source m4a was modified")
	}

	durationMs, err := media.GetVideoDurationMs(m4a)
	if err != nil {
		t.Fatalf("could not read source duration: %v", err)
	}

	out := fsutil.DefaultOutputPath(m4a)
	chapters := probeChaptersRaw(t, out)

	if len(chapters) != len(wantTitles) {
		t.Fatalf("chapter count = %d, want %d", len(chapters), len(wantTitles))
	}

	t.Logf("m4a chapter time_base = %q; source duration = %d ms",
		chapters[0].TimeBase, durationMs)

	for i, wantStart := range wantStarts {
		if got := millisecondsOf(t, chapters[i]); got != wantStart {
			t.Errorf("chapter %d start = %d ms, want %d ms", i, got, wantStart)
		}
		if chapters[i].Tags.Title != wantTitles[i] {
			t.Errorf("chapter %d title = %q, want %q", i, chapters[i].Tags.Title, wantTitles[i])
		}
	}

	// End-chain invariant: each chapter ends where the next one starts.
	for i := 0; i < len(chapters)-1; i++ {
		got := endMsOf(t, chapters[i])
		want := millisecondsOf(t, chapters[i+1])
		if got != want {
			t.Errorf("chapter %d end = %d ms, want next chapter start %d ms", i, got, want)
		}
	}

	// Final chapter end equals the probed source-media duration within media.ToleranceMs.
	finalEnd := endMsOf(t, chapters[len(chapters)-1])
	if diff(finalEnd, durationMs) > media.ToleranceMs {
		t.Errorf("final chapter end = %d ms, want source duration %d ms (within %d ms)",
			finalEnd, durationMs, media.ToleranceMs)
	}
}

// probeToModel converts ffprobe's chapter output back into the project's chapter
// model, the reverse of the embed step. Start timestamps are normalized through
// each chapter's container time_base to the project's millisecond precision, so
// Parse(TXT) and FFprobe(FFmpeg(TXT)) can be compared directly. The Line field
// has no probe equivalent and is left as its zero value.
func probeToModel(t *testing.T, container string, probed []media.ProbeChapter) []chapters.Chapter {
	t.Helper()
	model := make([]chapters.Chapter, len(probed))
	for i, c := range probed {
		start, ok := media.TimebaseToMillis(c.Start, c.TimeBase)
		if !ok {
			t.Fatalf("[%s] chapter %d: cannot convert start time_base %q", container, i, c.TimeBase)
		}
		model[i] = chapters.Chapter{Start: start, Title: c.Tags.Title}
	}
	return model
}

// probeEndsMs converts each probed chapter's stored end to milliseconds via its
// time_base, so the end-chain (each end equals the next chapter's start; the
// final end equals the media duration) can be verified against the derived ends
// of the parsed model.
func probeEndsMs(t *testing.T, container string, chapters []media.ProbeChapter) []int64 {
	t.Helper()
	ends := make([]int64, len(chapters))
	for i, c := range chapters {
		end, ok := media.TimebaseToMillis(c.End, c.TimeBase)
		if !ok {
			t.Fatalf("[%s] chapter %d: cannot convert end time_base %q", container, i, c.TimeBase)
		}
		ends[i] = end
	}
	return ends
}

// requireRoundTripEqual proves the semantic equality Parse(TXT) ==
// FFprobe(FFmpeg(TXT)) for one container: the probed model must match the parsed
// model in count, order, start (within media.ToleranceMs), and exact title, and each
// probed end must equal the derived end (the next chapter's start, or the media
// duration for the final chapter) within media.ToleranceMs. Failures identify the
// container and chapter index with both expected and actual values.
func requireRoundTripEqual(t *testing.T, container string, parsed []chapters.Chapter, durationMs int64, probed []chapters.Chapter, probedEnds []int64) {
	t.Helper()
	if len(probed) != len(parsed) {
		t.Errorf("[%s] chapter count = %d, want %d", container, len(probed), len(parsed))
		return
	}
	for i := range parsed {
		if diff(probed[i].Start, parsed[i].Start) > media.ToleranceMs {
			t.Errorf("[%s] chapter %d start = %d ms, want %d ms", container, i, probed[i].Start, parsed[i].Start)
		}
		if probed[i].Title != parsed[i].Title {
			t.Errorf("[%s] chapter %d title = %q, want %q", container, i, probed[i].Title, parsed[i].Title)
		}
		wantEnd := durationMs
		if i+1 < len(parsed) {
			wantEnd = parsed[i+1].Start
		}
		if diff(probedEnds[i], wantEnd) > media.ToleranceMs {
			t.Errorf("[%s] chapter %d end = %d ms, want %d ms", container, i, probedEnds[i], wantEnd)
		}
	}
}

// TestRoundTripParseEqualsProbe is the Phase 2E round-trip proof. For every
// currently supported container (MP4, MKV, M4A/AAC) it establishes, using real
// FFmpeg/FFprobe:
//
//	Parse(TXT) == FFprobe(FFmpeg(TXT))
//
// A single adversarial chapter fixture (Arabic/Unicode, spaces, `=`, `;`, `#`,
// quotes, literal backslash sequences, an exact-zero first chapter at 00:00:00,
// and fractional timestamps through the supported duration boundary) is parsed
// with the production parser (step 1), embedded by the production addch pipeline
// (step 2), probed with real ffprobe (step 3), converted back into the project's
// chapter model inside this test (step 4), and compared at the project's
// millisecond precision (step 5). Only media produced through addch's own
// supported path is proven; arbitrary foreign chapter structures are out of
// scope. The test skips only when ffmpeg/ffprobe are unavailable; while the
// tools are installed, any mismatch fails.
func TestRoundTripParseEqualsProbe(t *testing.T) {
	for _, container := range []string{"mp4", "mkv", "m4a"} {
		t.Run(container, func(t *testing.T) {
			requireTools(t)

			dir := t.TempDir()
			var video string
			if container == "m4a" {
				video = makeTestAudio(t, dir, "10")
			} else {
				video = makeTestContainer(t, dir, container, "10")
			}

			// One fixture for every container, reusing the titles already proven
			// in the Phase 2 MP4/MKV/M4A integration coverage.
			chaptersFile := filepath.Join(dir, "chapters.txt")
			content := "00:00:00.000 مقدمة With Spaces and حروف\n" +
				"00:00:02.500 eq=a ;semi #hash \"double quoted\"\n" +
				"00:00:05.250 Back\\slash literal\\nsequence\n" +
				"00:00:08.000 literal\\tsequence literal\\rreturn mixed العربية mixed\n"
			if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
				t.Fatalf("write chapters: %v", err)
			}

			// Step 1: parse the fixture with the production parser/model.
			parsed, err := chapters.ParseFile(chaptersFile)
			if err != nil {
				t.Fatalf("[%s] parse chapters: %v", container, err)
			}
			if len(parsed) != 4 {
				t.Fatalf("[%s] parsed chapter count = %d, want 4", container, len(parsed))
			}

			// Step 2: embed with the production addch pipeline (copy-out).
			_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
			if code != 0 {
				t.Fatalf("[%s] embed failed with code %d:\n%s", container, code, errStr)
			}

			durationMs, err := media.GetVideoDurationMs(video)
			if err != nil {
				t.Fatalf("[%s] could not read source duration: %v", container, err)
			}

			// Steps 3-4: probe the output and convert back into the project model.
			probed := probeChaptersRaw(t, fsutil.DefaultOutputPath(video))
			got := probeToModel(t, container, probed)
			gotEnds := probeEndsMs(t, container, probed)

			t.Logf("[%s] chapter time_base = %q; source duration = %d ms",
				container, probed[0].TimeBase, durationMs)

			// Step 5: normalized semantic comparison.
			requireRoundTripEqual(t, container, parsed, durationMs, got, gotEnds)
		})
	}
}

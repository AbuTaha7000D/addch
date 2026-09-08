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

// makeTestContainer creates a small, real video fixture in the requested
// container ("mp4" or "mkv") and returns its path. The mp4 is produced by
// makeTestVideo; other containers are transmuxed from it with stream copy.
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
	// Drop the intermediate mp4 so it never leaks into rmch discovery (which,
	// unlike addch, has no sidecar filter).
	_ = os.Remove(mp4)
	return out
}

// makeNamedContainer creates a small real media fixture and renames it to name
// so several distinct files can coexist in one directory.
func makeNamedContainer(t *testing.T, dir, name, container, durationSec string) string {
	t.Helper()
	src := makeTestContainer(t, dir, container, durationSec)
	dest := filepath.Join(dir, name)
	if err := os.Rename(src, dest); err != nil {
		t.Fatalf("rename %s to %s: %v", src, name, err)
	}
	return dest
}

// seedChapters are the chapter markers embedded into rmch fixtures. The seeds
// start at 0/3/6s, so fixtures must be at least ~6s long.
var seedChapters = []chapters.Chapter{
	{Start: 0, Title: "Intro"},
	{Start: 3000, Title: "Middle"},
	{Start: 6000, Title: "End"},
}

// makeChapteredFixture creates a real media file in dir with chapters embedded
// (via the production media core), named name, and returns its path. The plain
// seed is removed afterwards so the directory only contains the chaptered file.
func makeChapteredFixture(t *testing.T, dir, name, container, durationSec string) string {
	t.Helper()
	src := makeNamedContainer(t, dir, "seed."+container, container, durationSec)
	durationMs, err := media.GetVideoDurationMs(src)
	if err != nil {
		t.Fatalf("[%s] probe seed: %v", container, err)
	}
	meta := media.BuildMetadata(seedChapters, durationMs)
	metaPath, err := media.WriteTempMetadata(meta)
	if err != nil {
		t.Fatalf("[%s] write metadata: %v", container, err)
	}
	defer os.Remove(metaPath)

	dest := filepath.Join(dir, name)
	ext := fsutil.OutputExtension(dest)
	rp, err := media.StartRemux(src, metaPath, dest, ext, false)
	if err != nil {
		t.Fatalf("[%s] start remux: %v", container, err)
	}
	if err := rp.Wait(); err != nil {
		t.Fatalf("[%s] remux failed: %v", container, err)
	}
	if err := media.VerifyChapters(dest, seedChapters, durationMs); err != nil {
		t.Fatalf("[%s] seed verification failed: %v", container, err)
	}
	_ = os.Remove(src)
	return dest
}

// runRemovalForTest drives the full single-file removal pipeline for real file
// paths, capturing output into string buffers and returning the exit code.
func runRemovalForTest(video, output string, overwrite bool) (string, string, int) {
	pa := &parsedArgs{video: video, output: output, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runRemoval(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
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

// streamInfo is the subset of ffprobe -show_streams used for preservation
// assertions.
type streamInfo struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
}

// probeStreamsRaw runs ffprobe -show_streams and returns the parsed streams.
func probeStreamsRaw(t *testing.T, path string) []streamInfo {
	t.Helper()
	out, err := media.ProbeJSON("-show_streams", path)
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	var sf struct {
		Streams []streamInfo `json:"streams"`
	}
	if err := json.Unmarshal(out, &sf); err != nil {
		t.Fatalf("parse streams: %v", err)
	}
	return sf.Streams
}

// countTempMetadata returns the number of addch temp metadata files currently
// in the system temp directory (rmch never creates them, so any change signals
// a leak from its fixtures or pipeline).
func countTempMetadata() int {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "addch-metadata-*"))
	return len(matches)
}

// diff returns the absolute difference between two values.
func diff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

// requireZeroChapters asserts the file reports exactly zero chapters via ffprobe
// and returns the count for callers that want to inspect it.
func requireZeroChapters(t *testing.T, path string) {
	t.Helper()
	if got := probeChaptersRaw(t, path); len(got) != 0 {
		t.Errorf("expected 0 chapters in %s, found %d", path, len(got))
	}
}

// requirePreservedStreams asserts every video/audio stream of the source is
// still present in the output with an unchanged codec (stream copy). Extra
// residual streams (e.g. MP4 chapter-track bin_data) are permitted; they carry
// no chapters.
func requirePreservedStreams(t *testing.T, src, out string) {
	t.Helper()
	srcStreams := probeStreamsRaw(t, src)
	outStreams := probeStreamsRaw(t, out)

	srcHasVideo, srcHasAudio := false, false
	outHas := func(typ, codec string) bool {
		for _, s := range outStreams {
			if s.CodecType == typ && s.CodecName == codec {
				return true
			}
		}
		return false
	}
	for _, s := range srcStreams {
		switch s.CodecType {
		case "video":
			srcHasVideo = true
		case "audio":
			srcHasAudio = true
		default:
			continue // subtitles/data are permitted to be absent after remux
		}
		if !outHas(s.CodecType, s.CodecName) {
			t.Errorf("stream %s/%s lost or re-encoded: not found in output", s.CodecType, s.CodecName)
		}
	}
	if !srcHasVideo || !srcHasAudio {
		t.Fatalf("fixture should carry video and audio: %+v", srcStreams)
	}
}

func TestRemoveMP4(t *testing.T) {
	removeContainer(t, "mp4")
}

func TestRemoveMKV(t *testing.T) {
	removeContainer(t, "mkv")
}

// removeContainer is the shared MP4+MKV removal proof: copy-out safety, zero
// chapters in the output, duration unchanged, video+audio preserved with
// unchanged codecs, and no temp metadata leak.
func removeContainer(t *testing.T, container string) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered."+container, container, "10")

	srcBefore, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("[%s] read source: %v", container, err)
	}
	srcMs, err := media.GetVideoDurationMs(input)
	if err != nil {
		t.Fatalf("[%s] probe source: %v", container, err)
	}
	if n := probeChaptersRaw(t, input); len(n) != 3 {
		t.Fatalf("[%s] fixture must start with chapters, found %d", container, len(n))
	}
	tempBefore := countTempMetadata()

	_, errStr, code := runRemovalForTest(input, "", false)
	if code != 0 {
		t.Fatalf("[%s] runRemoval failed code %d:\n%s", container, code, errStr)
	}
	output := fsutil.DefaultNoChaptersOutputPath(input)
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("[%s] expected output %q: %v", container, output, err)
	}

	if srcAfter, err := os.ReadFile(input); err != nil {
		t.Fatalf("[%s] read source after removal: %v", container, err)
	} else if !bytes.Equal(srcBefore, srcAfter) {
		t.Errorf("[%s] source video was modified", container)
	}

	requireZeroChapters(t, output)
	requirePreservedStreams(t, input, output)

	outMs, err := media.GetVideoDurationMs(output)
	if err != nil {
		t.Fatalf("[%s] probe output: %v", container, err)
	}
	if diff(outMs, srcMs) > media.ToleranceMs {
		t.Errorf("[%s] output duration %d ms, want source %d ms (within %d ms)", container, outMs, srcMs, media.ToleranceMs)
	}

	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("[%s] temp metadata leaked: before=%d after=%d", container, tempBefore, got)
	}
}

func TestNoChaptersInputSingleFile(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	srcBefore, _ := os.ReadFile(video)

	out, errStr, code := runRemovalForTest(video, "", false)
	if code == 0 {
		t.Fatalf("expected a no-chapters input to fail:\n%s", out)
	}
	if !strings.Contains(errStr, "has no chapters to remove") {
		t.Errorf("unexpected error message: %s", errStr)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(video)); !os.IsNotExist(err) {
		t.Error("no output file should be created for a no-chapters input")
	}
	if got, _ := os.ReadFile(video); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the refused run")
	}
}

func TestOverwriteRefusal(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	if _, errStr, code := runRemovalForTest(input, "", false); code != 0 {
		t.Fatalf("first remove should succeed, got code %d:\n%s", code, errStr)
	}
	output := fsutil.DefaultNoChaptersOutputPath(input)
	orig, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	// Second run without --overwrite must fail and leave the output untouched.
	_, errStr, code := runRemovalForTest(input, "", false)
	if code == 0 {
		t.Fatal("second remove should fail without --overwrite")
	}
	if !strings.Contains(errStr, "already exists") {
		t.Errorf("unexpected error message: %s", errStr)
	}
	if got, _ := os.ReadFile(output); !bytes.Equal(got, orig) {
		t.Error("existing output was replaced despite no --overwrite")
	}
}

func TestOverwriteFlag(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	srcBefore, _ := os.ReadFile(input)

	if _, errStr, code := runRemovalForTest(input, "", false); code != 0 {
		t.Fatalf("first remove failed code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runRemovalForTest(input, "", true); code != 0 {
		t.Fatalf("overwrite remove failed code %d:\n%s", code, errStr)
	}
	output := fsutil.DefaultNoChaptersOutputPath(input)
	requireZeroChapters(t, output)
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("input was modified by the --overwrite run")
	}
}

func TestRemoveFailureCleansUp(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	srcBefore, _ := os.ReadFile(input)
	tempBefore := countTempMetadata()

	// An output path in a missing directory makes FFmpeg fail after the strip
	// starts; the partial output (nothing could be written) and any temp state
	// must be cleaned and the run must exit 1.
	output := filepath.Join(dir, "absent", "out.mp4")
	_, errStr, code := runRemovalForTest(input, output, false)
	if code == 0 {
		t.Fatal("expected the strip to fail when the output directory is missing")
	}
	if errStr == "" {
		t.Fatal("expected an error message")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("partial output must not remain: %v", err)
	}
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the failed run")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked after failure: before=%d after=%d", tempBefore, got)
	}
}

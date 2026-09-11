package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

// dirWritable probes dir by creating and removing a marker file in it, so a
// read-only directory can be distinguished from one whose chmod was ineffective
// (root on Unix, the entire concept on Windows). A directory that still accepts
// writes after chmod must not be treated as read-only by the test.
func dirWritable(dir string) bool {
	marker := filepath.Join(dir, ".writetest-probe")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(marker)
	return true
}

// TestAddchInvalidPayloads proves the embed front door rejects media that is
// empty, pure garbage, or a truncated real file, identifying the file and
// refusing to leave an output behind. ffprobe cannot compute a duration from
// any of these, so GetVideoDurationMs must fail loudly.
func TestAddchInvalidPayloads(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	realFixture := makeTestVideo(t, dir, "5")
	full, err := os.ReadFile(realFixture)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, fileName string
		payload        []byte
	}{
		{"zero-byte", "empty.mp4", nil},
		{"garbage", "garbage.mp4", []byte("this is not a real media file, not even close")},
		{"truncated", "truncated.mp4", full[:4096]}, // moov tail is cut off: no usable container metadata
	}

	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:02 B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			video := filepath.Join(dir, c.fileName)
			os.WriteFile(video, c.payload, 0o644)

			out, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
			if code == 0 {
				t.Fatalf("embed of %s must fail\nstdout:\n%s", c.fileName, out)
			}
			if !strings.Contains(errStr, "is it a valid video file") {
				t.Errorf("expected a valid-video diagnostic, got:\n%s", errStr)
			}
			if !strings.Contains(errStr, video) {
				t.Errorf("the failed input must be named, got:\n%s", errStr)
			}
			if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
				t.Error("failed embed must not leave an output file behind")
			}
		})
	}
}

// TestAddchBatchCorruptSiblingContinues proves batch mode isolates a broken
// candidate: a corrupt media file with a valid sidecar fails on its own while
// a healthy sibling completes, the summary counts both, and nothing is left in
// the temp metadata directory.
func TestAddchBatchCorruptSiblingContinues(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	good := makeNamedContainer(t, dir, "good.mp4", "mp4", "8")
	writeSidecar(t, good, batchChapters)
	bad := makeNamedContainer(t, dir, "bad.mp4", "mp4", "8")
	writeSidecar(t, bad, batchChapters)
	if err := os.WriteFile(bad, []byte("garbage pretending to be a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	tempBefore := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("expected exit 1 (one failed item), got %d:\n%s\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if err := media.VerifyChapters(fsutil.DefaultOutputPath(good), mustParseBatchChapters(t), mustBatchDurationMs(t, good)); err != nil {
		t.Errorf("healthy sibling output failed verification: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(bad)); err == nil {
		t.Error("corrupt sibling must not leave an output file behind")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
	}
}

// mustParseBatchChapters parses the shared batchChapters fixture content into
// the toolkit model for verification.
func mustParseBatchChapters(t *testing.T) []chapters.Chapter {
	t.Helper()
	chs, err := chapters.Parse(strings.NewReader(batchChapters))
	if err != nil {
		t.Fatalf("parse batchChapters fixture: %v", err)
	}
	return chs
}

// mustBatchDurationMs probes a fixture's real duration for verification.
func mustBatchDurationMs(t *testing.T, mediaPath string) int64 {
	t.Helper()
	ms, err := media.GetVideoDurationMs(mediaPath)
	if err != nil {
		t.Fatalf("probe %s for verification: %v", mediaPath, err)
	}
	return ms
}

// TestAddchOutputDirNonexistentFails proves embedding into an output directory
// that does not exist surfaces the FFmpeg failure through the enriched error
// and leaves nothing littered — FFmpeg itself rejects the unopenable path.
func TestAddchOutputDirNonexistentFails(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "5")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:02 B\n"), 0o644)

	missingOutput := filepath.Join(dir, "no-such-directory", "out.mp4")
	tempBefore := countTempMetadata()
	out, errStr, code := runEmbedForTest(chaptersFile, video, missingOutput, false)
	if code == 0 {
		t.Fatalf("embed into a nonexistent directory must fail\nstdout:\n%s", out)
	}
	if !strings.Contains(errStr, "FFmpeg failed to remux the video") {
		t.Errorf("expected an FFmpeg remux failure, got:\n%s", errStr)
	}
	if _, err := os.Stat(missingOutput); err == nil {
		t.Error("no output may exist at the uncreatable path")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
	}
}

// TestAddchBatchDirectoryPretendingToBeMedia proves discovery dismisses a
// directory that only carries a media-looking name: it is not a regular file,
// so it cannot be a candidate, and a directory-only prefix yields the friendly
// "nothing to do" result.
func TestAddchBatchDirectoryPretendingToBeMedia(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "trick.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trick.txt"), []byte("00:00:00 A\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("a media-shaped directory alone must exit 0, got %d:\n%s\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "No media files with a matching .txt sidecar") {
		t.Errorf("expected the nothing-to-do message, got:\n%s", out)
	}
}

// TestAddchMoovDurationLieIgnored patches a real container's mvhd header to
// declare a wildly wrong duration (600 s instead of the true few seconds) and
// proves the toolkit binds chapters to the probed duration rather than the
// bogus header: the embed succeeds and the final chapter END equals the real
// probed duration. ffprobe reconstructs duration from the actual sample
// timestamps, so a lying header must never corrupt the result.
func TestAddchMoovDurationLieIgnored(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "5")
	patchMvhdDurationMs(t, video, 600000)

	chaptersFile := filepath.Join(dir, "chapters.txt")
	chaptersContent := "00:00:00 A\n00:00:02 B\n"
	os.WriteFile(chaptersFile, []byte(chaptersContent), 0o644)

	durationMs, err := media.GetVideoDurationMs(video)
	if err != nil {
		t.Fatalf("probe patched video: %v", err)
	}
	if durationMs > 10000 {
		t.Fatalf("the mvhd lie must not leak into the probed duration (probed %d ms)", durationMs)
	}

	out, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("embed of an intentionally mis-declared container must succeed\nstdout:\n%s\nstderr:\n%s", out, errStr)
	}
	// The emitted chapter END equals the probed (true) duration, not the lie
	// in the mvhd header.
	want, err := chapters.Parse(strings.NewReader(chaptersContent))
	if err != nil {
		t.Fatal(err)
	}
	if err := media.VerifyChapters(fsutil.DefaultOutputPath(video), want, durationMs); err != nil {
		t.Errorf("verification must bind to the true probed duration: %v", err)
	}
}

// findBox walks the given byte slice for the first occurrence of a top-level
// (or header-nesting) MP4 box with a matching fourcc. It is intentionally a
// shallow scan: the mov header lies near the end for the fixture, and mvhd is
// unique enough for test purposes. Returns the box start offset or -1.
func findBox(b []byte, fourcc string) int {
	for i := 0; i+8 <= len(b); i++ {
		if string(b[i+4:i+8]) == fourcc {
			return i
		}
	}
	return -1
}

// patchMvhdDurationMs rewrites the full-box duration field of a standard MP4
// mvhd (version 0) so the container HEADER declares targetMs milliseconds. The
// media sample data is untouched, so ffprobe still reports the true duration.
// If no mappable mvhd is found the test helper skips, since the fixture shape
// is not under our control.
func patchMvhdDurationMs(t *testing.T, path string, targetMs int64) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s for mvhd patch: %v", path, err)
	}
	off := findBox(data, "mvhd")
	if off < 0 || off+32 > len(data) {
		t.Skip("fixture has no patchable mvhd; skipping moov-lie coverage")
	}
	timescale := binary.BigEndian.Uint32(data[off+20 : off+24])
	if timescale == 0 {
		t.Skip("fixture mvhd has a zero timescale; skipping moov-lie coverage")
	}
	// version 0: duration (big-endian u32) sits 24 bytes into the box, in
	// timescale units. Round up so the declaration can never coincide with the
	// real (much shorter) duration.
	duration := (targetMs*int64(timescale) + 999) / 1000
	binary.BigEndian.PutUint32(data[off+24:off+28], uint32(duration))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write patched %s: %v", path, err)
	}
}

// TestAddchBatchReadOnlyOutputDirContinues proves an unwritable output
// directory fails only its own candidate while a writable sibling directory in
// the same recursive batch completes: read protection is not contagious but is
// also never silently papered over.
func TestAddchBatchReadOnlyOutputDirContinues(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	rw := filepath.Join(dir, "rw")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rw, 0o755); err != nil {
		t.Fatal(err)
	}
	roVideo := makeNamedContainer(t, ro, "x.mp4", "mp4", "8")
	writeSidecar(t, roVideo, batchChapters)
	rwVideo := makeNamedContainer(t, rw, "y.mp4", "mp4", "8")
	writeSidecar(t, rwVideo, batchChapters)

	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0o755) // keep t.TempDir cleanup from stumbling
	if dirWritable(ro) {
		t.Skip("chmod did not make the directory read-only (running as root or on Windows)")
	}

	tempBefore := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, true, false)
	if code != 1 {
		t.Fatalf("expected exit 1 (one unwritable item), got %d:\n%s\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(roVideo)); err == nil {
		t.Error("read-only directory must not receive an output file")
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(rwVideo)); err != nil {
		t.Errorf("writable sibling must be processed: %v", err)
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
	}
}

// dirListable probes whether dir can actually be listed, so a chmod 000 that
// was ineffective (privileged runners on Unix) does not fake a traversal error.
func dirListable(dir string) bool {
	_, err := os.ReadDir(dir)
	return err == nil
}

// TestAddchBatchUnreadableSubdirAbortsDiscovery proves an entry-level
// traversal error (an unreadable nested directory) aborts recursive discovery:
// the batch must exit nonzero, name the failing subtree on stderr, and never
// claim a complete run (no batch summary) or process even the readable half of
// the tree — an incomplete traversal is never reported as success.
func TestAddchBatchUnreadableSubdirAbortsDiscovery(t *testing.T) {
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits are not enforced on Windows")
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	goodVideo := makeNamedContainer(t, good, "a.mp4", "mp4", "8")
	writeSidecar(t, goodVideo, batchChapters)
	lockedVideo := makeNamedContainer(t, locked, "hidden.mp4", "mp4", "8")
	writeSidecar(t, lockedVideo, batchChapters)

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(locked, 0o755) }()
	if !dirListable(dir) || dirListable(locked) {
		t.Skip("chmod did not make the subdirectory unlistable (running as root or on Windows)")
	}

	out, errStr, code := runBatchForTest(dir, true, false)
	if code != 1 {
		t.Fatalf("expected exit 1 for a traversal error, got %d:\n%s\n%s", code, errStr, out)
	}
	if !strings.Contains(errStr, "cannot traverse") {
		t.Errorf("stderr must name the failed traversal, got:\n%s", errStr)
	}
	if strings.Contains(out, "Total:") {
		t.Errorf("an aborted discovery must not produce a batch summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(goodVideo)); err == nil {
		t.Error("no media may be processed when discovery aborts")
	}
}

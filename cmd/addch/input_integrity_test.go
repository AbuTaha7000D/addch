package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 4 (addch): prove the original files — the input video and the
// user's chapter file — are never modified, byte-for-byte (SHA-256) and
// timestamp-wise (ModTime), across every addch execution path: success,
// explicit output elsewhere, --check, failed remux, lying-FFmpeg verification
// failure, batch with a corrupt sibling, same-path collision refusal, and
// --overwrite.

// fileSnapshot is the recorded identity of an input we promise never to touch.
type fileSnapshot struct {
	path    string
	hash    string
	modTime time.Time
}

// sha256Hex returns the hex SHA-256 of a file's current contents.
func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s for hashing: %v", path, err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// snapshotFiles records the hash and modification time of each path.
func snapshotFiles(t *testing.T, paths ...string) []fileSnapshot {
	t.Helper()
	snaps := make([]fileSnapshot, 0, len(paths))
	for _, p := range paths {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s for snapshot: %v", p, err)
		}
		snaps = append(snaps, fileSnapshot{path: p, hash: sha256Hex(t, p), modTime: fi.ModTime()})
	}
	return snaps
}

// assertFilesUntouched re-checks every snapshot: an identical SHA-256 proves the
// bytes did not change and an identical ModTime proves the file was not even
// rewritten in place.
func assertFilesUntouched(t *testing.T, snaps []fileSnapshot) {
	t.Helper()
	for _, s := range snaps {
		t.Helper()
		if got := sha256Hex(t, s.path); got != s.hash {
			t.Errorf("%s was modified: hash %s, want %s", s.path, got, s.hash)
		}
		fi, err := os.Stat(s.path)
		if err != nil {
			t.Fatalf("stat %s after run: %v", s.path, err)
		}
		if !fi.ModTime().Equal(s.modTime) {
			t.Errorf("%s modification time changed: %v, want %v", s.path, fi.ModTime(), s.modTime)
		}
	}
}

func TestAddchInputIntegrityDefaultOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("embed failed with code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err != nil {
		t.Fatalf("expected output %s: %v", fsutil.DefaultOutputPath(video), err)
	}
	assertFilesUntouched(t, inputs)
}

func TestAddchInputIntegrityCustomOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	customOut := filepath.Join(outDir, "course-chapters.mp4")

	inputs := snapshotFiles(t, video, chaptersFile)

	_, errStr, code := runEmbedForTest(chaptersFile, video, customOut, false)
	if code != 0 {
		t.Fatalf("embed with explicit output failed with code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(customOut); err != nil {
		t.Fatalf("expected explicit output %s: %v", customOut, err)
	}
	assertFilesUntouched(t, inputs)
}

func TestAddchInputIntegrityCheckOnly(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	var out, errBuf bytes.Buffer
	if code := runCheck(&out, &errBuf); code != 0 {
		t.Fatalf("--check failed with code %d:\n%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "System is ready.") {
		t.Errorf("expected readiness line, got:\n%s", out.String())
	}
	assertFilesUntouched(t, inputs)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("no output may be produced by --check")
	}
}

func TestAddchInputIntegrityFailedRemux(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")
	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{chaptersFile, video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected nonzero exit with a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "FFmpeg failed to remux the video") {
		t.Errorf("expected the remux failure diagnostic, got:\n%s", errBuf.String())
	}
	assertFilesUntouched(t, inputs)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("no output may exist after a failed remux")
	}
}

func TestAddchInputIntegrityVerificationFailure(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	t.Setenv("FAKE_FFMPEG", "garbage")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")
	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{chaptersFile, video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected verification to reject the lying ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "verification failed") {
		t.Errorf("expected a verification failure, got:\n%s", errBuf.String())
	}
	assertFilesUntouched(t, inputs)
}

func TestAddchInputIntegrityBatchCorruptSibling(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	okVideo := makeNamedContainer(t, dir, "ok.mp4", "mp4", "8")
	okSidecar := writeSidecar(t, okVideo, batchChapters)
	brokenVideo := makeNamedContainer(t, dir, "broken.mp4", "mp4", "6")
	brokenSidecar := writeSidecar(t, brokenVideo, "this is not a valid chapter line\n")

	inputs := snapshotFiles(t, okVideo, brokenVideo, okSidecar, brokenSidecar)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("batch with a corrupt sibling must exit 1, got %d:\n%s\n%s", code, out, errStr)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(okVideo)); err != nil {
		t.Errorf("healthy sibling must have produced output: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(brokenVideo)); err == nil {
		t.Error("corrupt sibling must not leave an output behind")
	}
	assertFilesUntouched(t, inputs)
}

func TestAddchInputIntegritySamePathRefusal(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	// Output == input video: refused up front, video untouched.
	if _, errStr, code := runEmbedForTest(chaptersFile, video, video, true); code == 0 {
		t.Fatalf("expected output==video to be refused, got exit 0:\n%s", errStr)
	}
	assertFilesUntouched(t, inputs)

	// Output == chapter file: refused up front, chapter file untouched.
	if _, errStr, code := runEmbedForTest(chaptersFile, video, chaptersFile, true); code == 0 {
		t.Fatalf("expected output==chapters to be refused, got exit 0:\n%s", errStr)
	}
	assertFilesUntouched(t, inputs)
}

func TestAddchInputIntegrityOverwrite(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	inputs := snapshotFiles(t, video, chaptersFile)

	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", false); code != 0 {
		t.Fatalf("first embed failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", true); code != 0 {
		t.Fatalf("overwrite embed failed with code %d:\n%s", code, errStr)
	}
	assertFilesUntouched(t, inputs)
}

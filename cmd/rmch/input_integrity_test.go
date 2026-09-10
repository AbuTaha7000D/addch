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

// Phase 8 item 4 (rmch): prove the input video is never modified — byte-for-byte
// (SHA-256) and timestamp-wise (ModTime) — across every rmch execution path:
// success, same-path collision refusal, --overwrite, failed strip via a fake
// ffmpeg, and a batch with a corrupt sibling.

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

func TestRmchInputIntegritySuccess(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	_, errStr, code := runRemovalForTest(input, "", false)
	if code != 0 {
		t.Fatalf("removal failed with code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(input)); err != nil {
		t.Fatalf("expected output %s: %v", fsutil.DefaultNoChaptersOutputPath(input), err)
	}
	assertFilesUntouched(t, inputs)
}

func TestRmchInputIntegritySamePathRefusal(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	if _, errStr, code := runRemovalForTest(input, input, true); code == 0 {
		t.Fatalf("expected output==input to be refused, got exit 0:\n%s", errStr)
	}
	assertFilesUntouched(t, inputs)
}

func TestRmchInputIntegrityOverwrite(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	if _, errStr, code := runRemovalForTest(input, "", false); code != 0 {
		t.Fatalf("first removal failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runRemovalForTest(input, "", true); code != 0 {
		t.Fatalf("overwrite removal failed with code %d:\n%s", code, errStr)
	}
	assertFilesUntouched(t, inputs)
}

func TestRmchInputIntegrityFailedStrip(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")
	var out, errBuf bytes.Buffer
	code := runRmchWithPath(t, []string{input}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected nonzero exit with a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "FFmpeg failed to strip the video") {
		t.Errorf("expected the strip failure diagnostic, got:\n%s", errBuf.String())
	}
	assertFilesUntouched(t, inputs)
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(input)); err == nil {
		t.Error("no output may exist after a failed strip")
	}
}

func TestRmchInputIntegrityBatchCorruptSibling(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	goodVideo := makeChapteredFixture(t, dir, "good.mp4", "mp4", "10")
	badFile := filepath.Join(dir, "broken.mp4")
	if err := os.WriteFile(badFile, []byte("not a real media file"), 0o644); err != nil {
		t.Fatal(err)
	}

	inputs := snapshotFiles(t, goodVideo, badFile)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("batch with a corrupt sibling must exit 1, got %d:\n%s\n%s", code, out, errStr)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(goodVideo)); err != nil {
		t.Errorf("healthy sibling must have produced output: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(badFile)); err == nil {
		t.Error("corrupt sibling must not leave an output behind")
	}
	assertFilesUntouched(t, inputs)
}

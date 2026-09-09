package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 4 (rmch): prove temporary files are always cleaned. Every run
// must leave the count of addch temp metadata files exactly where it found it
// (counted inside a private ADDCH_METADATA_TMPDIR so it is hermetic), with zero
// WriteFileAtomic sidecar temps and no partial-output remnants behind.

// countSidecarLitter returns the number of .addch-sidecar-* temp intermediates
// left in dir.
func countSidecarLitter(dir string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, ".addch-sidecar-*"))
	return len(matches)
}

// requireTempClean asserts the metadata temp count is unchanged from before and
// that no sidecar-temp or partial-output litter is present in the given dirs.
func requireTempClean(t *testing.T, before int, dirs ...string) {
	t.Helper()
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
	for _, d := range dirs {
		t.Helper()
		if n := countSidecarLitter(d); n != 0 {
			t.Errorf("sidecar temp files leaked in %s: %d", d, n)
		}
	}
}

func TestRmchTempCleanupSuccess(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	_, errStr, code := runRemovalForTest(input, "", false)
	if code != 0 {
		t.Fatalf("removal failed with code %d:\n%s", code, errStr)
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(input)); err != nil {
		t.Errorf("expected output: %v", err)
	}
}

func TestRmchTempCleanupSamePathRefusal(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	if _, _, code := runRemovalForTest(input, input, true); code == 0 {
		t.Fatal("expected output==input to be refused")
	}
	requireTempClean(t, before, dir)
}

func TestRmchTempCleanupOverwrite(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	if _, errStr, code := runRemovalForTest(input, "", false); code != 0 {
		t.Fatalf("first removal failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runRemovalForTest(input, "", true); code != 0 {
		t.Fatalf("overwrite removal failed with code %d:\n%s", code, errStr)
	}
	requireTempClean(t, before, dir)
}

func TestRmchTempCleanupFailedStrip(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")
	var out, errBuf bytes.Buffer
	code := runRmchWithPath(t, []string{input}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "FFmpeg failed to remux the video") {
		t.Errorf("expected the strip failure diagnostic, got:\n%s", errBuf.String())
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(input)); err == nil {
		t.Error("no partial output may remain after a failed strip")
	}
}

func TestRmchTempCleanupBatchCorruptSibling(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	goodVideo := makeChapteredFixture(t, dir, "good.mp4", "mp4", "10")
	badFile := filepath.Join(dir, "broken.mp4")
	if err := os.WriteFile(badFile, []byte("not a real media file"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("batch with a corrupt sibling must exit 1, got %d:\n%s\n%s", code, out, errStr)
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(goodVideo)); err != nil {
		t.Errorf("healthy sibling must have produced output: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(badFile)); err == nil {
		t.Error("corrupt sibling must not leave an output behind")
	}
}

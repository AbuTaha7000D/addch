package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 4 (getch): prove temporary files are always cleaned. Every run
// must leave the count of addch temp metadata files exactly where it found it
// (counted inside a private ADDCH_METADATA_TMPDIR so it is hermetic) and leave
// zero .addch-sidecar-* WriteFileAtomic temp intermediates behind.

func TestGetchTempCleanupProbeOnly(t *testing.T) {
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(video, []byte("dummy input; the fake ffprobe decides the outcome"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := countTempMetadata()
	t.Setenv("FAKE_FFPROBE", "ok")
	shim := fakeToolShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{video}, shim, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0 with a healthy fake ffprobe, got %d\nstderr:\n%s", code, errBuf.String())
	}
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
	if n := countSidecarTemp(dir); n != 0 {
		t.Errorf("sidecar temp files leaked in %s: %d", dir, n)
	}
}

func TestGetchTempCleanupFileOutput(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	sidecar := fsutil.SidecarPath(input)
	_, errStr, code := runExtractForTest(input, sidecar, false)
	if code != 0 {
		t.Fatalf("extract failed with code %d:\n%s", code, errStr)
	}
	if n := countSidecarTemp(dir); n != 0 {
		t.Errorf("sidecar temp files leaked in %s: %d", dir, n)
	}
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Errorf("expected sidecar: %v", err)
	}
}

func TestGetchTempCleanupOverwrite(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	before := countTempMetadata()
	sidecar := fsutil.SidecarPath(input)
	if _, errStr, code := runExtractForTest(input, sidecar, false); code != 0 {
		t.Fatalf("first extract failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runExtractForTest(input, sidecar, true); code != 0 {
		t.Fatalf("overwrite extract failed with code %d:\n%s", code, errStr)
	}
	if n := countSidecarTemp(dir); n != 0 {
		t.Errorf("sidecar temp files leaked in %s: %d", dir, n)
	}
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
}

func TestGetchTempCleanupNoChaptersFileOutput(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	sidecar := fsutil.SidecarPath(video)

	before := countTempMetadata()
	out, errStr, code := runExtractForTest(video, sidecar, false)
	if code != 0 {
		t.Fatalf("expected exit 0 for a chapterless video, got %d:\n%s", code, errStr)
	}
	if !strings.Contains(errStr, "No chapters") {
		t.Errorf("expected a no-chapters note on stderr, got:\n%s", errStr)
	}
	if _, err := os.Stat(sidecar); err == nil {
		t.Error("no sidecar may be created for a chapterless video")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout must stay empty, got:\n%s", out)
	}
	if n := countSidecarTemp(dir); n != 0 {
		t.Errorf("sidecar temp files leaked in %s: %d", dir, n)
	}
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
}

func TestGetchTempCleanupBatchFileOutput(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	a := makeChapteredFixture(t, dir, "a.mp4", "mp4", "10")
	b := makeChapteredFixture(t, dir, "b.mp4", "mp4", "10")

	before := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch failed with code %d:\n%s\n%s", code, out, errStr)
	}
	for _, v := range []string{a, b} {
		if _, err := os.Stat(fsutil.SidecarPath(v)); err != nil {
			t.Errorf("expected sidecar for %s: %v", v, err)
		}
	}
	if n := countSidecarTemp(dir); n != 0 {
		t.Errorf("sidecar temp files leaked in %s: %d", dir, n)
	}
	if got := countTempMetadata(); got != before {
		t.Errorf("temp metadata leaked: before=%d after=%d", before, got)
	}
}

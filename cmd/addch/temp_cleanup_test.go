package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 4 (addch): prove temporary files are always cleaned. Every run
// must leave the count of addch temp metadata files exactly where it found it
// (counted inside a private ADDCH_METADATA_TMPDIR so it is hermetic), with zero
// WriteFileAtomic sidecar temps and no partial output remnants behind.

// countSidecarLitter returns the number of .addch-sidecar-* temp intermediates
// left in dir.
func countSidecarLitter(dir string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, ".addch-sidecar-*"))
	return len(matches)
}

// requireTempClean asserts the metadata temp count is unchanged from before and
// that no sidecar-temp or partial-output litter is present in the given dirs.
// The default addch output for failed runs must be absent.
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

func TestAddchTempCleanupDefaultOutput(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	before := countTempMetadata()
	_, errStr, code := runEmbedForTest(chaptersFile, video, "", false)
	if code != 0 {
		t.Fatalf("embed failed with code %d:\n%s", code, errStr)
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err != nil {
		t.Errorf("expected output: %v", err)
	}
}

func TestAddchTempCleanupCustomOutput(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	customOut := filepath.Join(outDir, "course-chapters.mp4")

	before := countTempMetadata()
	_, errStr, code := runEmbedForTest(chaptersFile, video, customOut, false)
	if code != 0 {
		t.Fatalf("embed failed with code %d:\n%s", code, errStr)
	}
	requireTempClean(t, before, dir, outDir)
	if _, err := os.Stat(customOut); err != nil {
		t.Errorf("expected explicit output: %v", err)
	}
}

func TestAddchTempCleanupCheckOnly(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n"), 0o644)

	before := countTempMetadata()
	var out, errBuf bytes.Buffer
	if code := runCheck(&out, &errBuf); code != 0 {
		t.Fatalf("--check failed with code %d:\n%s", code, errBuf.String())
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("--check must not produce an output")
	}
}

func TestAddchTempCleanupFailedRemux(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	before := countTempMetadata()
	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")
	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{chaptersFile, video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("no partial output may remain after a failed remux")
	}
}

func TestAddchTempCleanupVerificationFailure(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	before := countTempMetadata()
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
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("junk output must be removed after a verification failure")
	}
}

func TestAddchTempCleanupSamePathRefusal(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	before := countTempMetadata()
	if _, _, code := runEmbedForTest(chaptersFile, video, video, true); code == 0 {
		t.Fatal("expected output==video to be refused")
	}
	if _, _, code := runEmbedForTest(chaptersFile, video, chaptersFile, true); code == 0 {
		t.Fatal("expected output==chapters to be refused")
	}
	requireTempClean(t, before, dir)
}

func TestAddchTempCleanupOverwrite(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Second\n"), 0o644)

	before := countTempMetadata()
	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", false); code != 0 {
		t.Fatalf("first embed failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runEmbedForTest(chaptersFile, video, "", true); code != 0 {
		t.Fatalf("overwrite embed failed with code %d:\n%s", code, errStr)
	}
	requireTempClean(t, before, dir)
}

func TestAddchTempCleanupBatchCorruptSibling(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	okVideo := makeNamedContainer(t, dir, "ok.mp4", "mp4", "8")
	writeSidecar(t, okVideo, batchChapters)
	brokenVideo := makeNamedContainer(t, dir, "broken.mp4", "mp4", "6")
	writeSidecar(t, brokenVideo, "this is not a valid chapter line\n")

	before := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("batch with a corrupt sibling must exit 1, got %d:\n%s\n%s", code, out, errStr)
	}
	requireTempClean(t, before, dir)
	if _, err := os.Stat(fsutil.DefaultOutputPath(okVideo)); err != nil {
		t.Errorf("healthy sibling must have produced output: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(brokenVideo)); err == nil {
		t.Error("corrupt sibling must not leave an output behind")
	}
}

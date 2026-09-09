package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// runBatchForTest drives runBatch with a minimal parsedArgs and returns the
// captured stdout, stderr, and exit code.
func runBatchForTest(dir string, recursive, overwrite bool) (string, string, int) {
	pa := &parsedArgs{dir: !recursive, recursive: recursive, dirPath: dir, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runBatch(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

// makeNamedContainer creates a small real media fixture and renames it to name
// so several distinct candidates can coexist in one directory.
func makeNamedContainer(t *testing.T, dir, name, container, durationSec string) string {
	t.Helper()
	src := makeTestContainer(t, dir, container, durationSec)
	dest := filepath.Join(dir, name)
	if err := os.Rename(src, dest); err != nil {
		t.Fatalf("rename %s to %s: %v", src, name, err)
	}
	return dest
}

// writeSidecar writes the "<stem>.txt" sidecar for mediaPath.
func writeSidecar(t *testing.T, mediaPath, content string) string {
	t.Helper()
	sidecar := fsutil.SidecarPath(mediaPath)
	if err := os.WriteFile(sidecar, []byte(content), 0o644); err != nil {
		t.Fatalf("write sidecar %s: %v", sidecar, err)
	}
	return sidecar
}

const batchChapters = "00:00:00 Intro\n00:00:03 Second\n00:00:06 End\n"

func TestRunBatchSuccessSummary(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
	b := makeNamedContainer(t, dir, "b.mkv", "mkv", "8")
	writeSidecar(t, a, batchChapters)
	writeSidecar(t, b, batchChapters)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 2 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	for _, base := range []string{"a-chapters.mp4", "b-chapters.mkv"} {
		if _, err := os.Stat(filepath.Join(dir, base)); err != nil {
			t.Errorf("expected output %s: %v", base, err)
		}
	}
}

func TestRunBatchExistingOutputSkipped(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
	writeSidecar(t, a, batchChapters)

	// Pre-create the default output with dummy content; it must not be rewritten.
	outPath := fsutil.DefaultOutputPath(a)
	dummy := []byte("pre-existing output")
	if err := os.WriteFile(outPath, dummy, 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if got, err := os.ReadFile(outPath); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got, dummy) {
		t.Error("existing output was rewritten despite no --overwrite")
	}
}

func TestRunBatchMissingSidecarExcluded(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	with := makeNamedContainer(t, dir, "with.mp4", "mp4", "8")
	writeSidecar(t, with, batchChapters)
	// A media file with no sidecar must not become a candidate (dummy bytes are
	// enough: discovery never probes content).
	if err := os.WriteFile(filepath.Join(dir, "orphan.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if strings.Contains(out, "orphan") {
		t.Errorf("sidecar-less media must be absent from the report:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(filepath.Join(dir, "orphan.mp4"))); err == nil {
		t.Error("sidecar-less media must not be processed")
	}
}

func TestRunBatchContinuesAfterFailure(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	good := makeNamedContainer(t, dir, "good.mp4", "mp4", "8")
	writeSidecar(t, good, batchChapters)

	bad := makeNamedContainer(t, dir, "bad.mkv", "mkv", "8")
	writeSidecar(t, bad, "this is not a valid chapter line\n")

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("expected exit 1 with a failed item, got %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(good)); err != nil {
		t.Errorf("good item output missing: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(bad)); err == nil {
		t.Error("failed item must not leave an output behind")
	}
}

func TestRunBatchGeneratedOutputExcluded(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	keep := makeNamedContainer(t, dir, "keep.mp4", "mp4", "8")
	writeSidecar(t, keep, batchChapters)

	// A generated-output filename with its own sidecar must never be
	// re-processed (chaining prevention).
	if err := os.WriteFile(filepath.Join(dir, "video-chapters.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video-chapters.txt"), []byte("00:00:00 A\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "video-chapters-chapters.mp4")); err == nil {
		t.Error("chaining: generated output was processed, producing a nested output")
	}
}

func TestRunBatchRootErrors(t *testing.T) {
	requireTools(t)

	missing := filepath.Join(t.TempDir(), "missing")
	out, errStr, code := runBatchForTest(missing, true, false)
	if code != 1 {
		t.Fatalf("missing recursive root: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errStr)
	}
	if !strings.Contains(errStr, "Error:") {
		t.Errorf("missing recursive root: want an Error line on stderr, got:\n%s", errStr)
	}

	fileAsRoot := filepath.Join(t.TempDir(), "not-a-directory.mp4")
	if err := os.WriteFile(fileAsRoot, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errStr, code = runBatchForTest(fileAsRoot, true, false)
	if code != 1 {
		t.Fatalf("file as recursive root: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errStr)
	}
	if !strings.Contains(errStr, "Error:") {
		t.Errorf("file as recursive root: want an Error line on stderr, got:\n%s", errStr)
	}
}

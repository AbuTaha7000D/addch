package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// TestRmchAdversarialOutputDirectory pins the output-path-is-a-directory
// behavior in both single and batch modes. Without --overwrite the existing
// guard fires; with --overwrite FFmpeg fails and the directory must never be
// removed (regression for the Phase 8 item 2 finding that cleanup deleted an
// empty directory at the output path).
func TestRmchAdversarialOutputDirectory(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	t.Run("single: directory at output path refused without overwrite", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := filepath.Join(dir, "output-as-dir")
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runRemovalForTest(input, asDir, false)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(errStr, "already exists") {
			t.Errorf("unexpected error: %s", errStr)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("refused output directory must be preserved")
		}
	})

	t.Run("single: empty directory at default output path survives --overwrite", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		out := fsutil.DefaultNoChaptersOutputPath(input)
		if err := os.Mkdir(out, 0o755); err != nil {
			t.Fatal(err)
		}
		srcBefore := readBytes(t, input)
		tempBefore := countTempMetadata()
		_, errStr, code := runRemovalForTest(input, "", true)
		if code != 1 {
			t.Fatalf("overwrite into a directory must fail, code %d", code)
		}
		if errStr == "" {
			t.Fatal("expected an error message")
		}
		if fi, err := os.Stat(out); err != nil || !fi.IsDir() {
			t.Error("--overwrite run deleted the empty directory at the default output path")
		}
		if !bytes.Equal(readBytes(t, input), srcBefore) {
			t.Error("input was modified")
		}
		if got := countTempMetadata(); got != tempBefore {
			t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
		}
	})

	t.Run("batch: directory at output path skipped", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := fsutil.DefaultNoChaptersOutputPath(input)
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		out, errStr, code := runBatchForTest(dir, false, false)
		if code != 0 {
			t.Fatalf("expected skip, exit 0, got %d:\n%s", code, errStr)
		}
		if !strings.Contains(out, "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0") {
			t.Errorf("unexpected summary:\n%s", out)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("batch must not remove a directory at the output path")
		}
	})

	t.Run("batch: directory at output path survives --overwrite failure", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := fsutil.DefaultNoChaptersOutputPath(input)
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		out, errStr, code := runBatchForTest(dir, false, true)
		if code != 1 {
			t.Fatalf("expected exit 1 from a failed overwrite item, got %d:\n%s", code, errStr)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("batch --overwrite deleted the directory at the output path")
		}
		_ = out
	})
}

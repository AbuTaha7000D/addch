package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// TestGetchAdversarialOutputDirectory pins the output-path-is-a-directory
// behavior in both single and batch modes. Without --overwrite the existing
// guard fires; with --overwrite the atomic write fails cleanly (renaming a temp
// file over a directory) and the directory and its contents survive.
func TestGetchAdversarialOutputDirectory(t *testing.T) {
	requireTools(t)

	t.Run("single: directory at output path refused without overwrite", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := filepath.Join(dir, "output-as-dir")
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runExtractForTest(input, asDir, false)
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

	t.Run("single: directory at output path survives --overwrite", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := filepath.Join(dir, "output-as-dir")
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runExtractForTest(input, asDir, true)
		if code != 1 {
			t.Fatalf("overwrite into a directory must fail, code %d", code)
		}
		if !strings.Contains(errStr, "rename") {
			t.Errorf("expected the rename failure to surface, got: %s", errStr)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("--overwrite run deleted the output directory")
		}
		entries, err := os.ReadDir(asDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("temporary sidecar litter remained in the output directory: %v", entries)
		}
	})

	t.Run("batch: directory at sidecar path skipped", func(t *testing.T) {
		dir := t.TempDir()
		input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
		asDir := fsutil.SidecarPath(input)
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		out, errStr, code := runBatchForTest(dir, false, false)
		if code != 0 {
			t.Fatalf("expected skip, exit 0, got %d:\n%s", code, errStr)
		}
		if out != "" {
			t.Errorf("batch mode must keep stdout empty, got %q", out)
		}
		if !strings.Contains(errStr, "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0") {
			t.Errorf("unexpected summary:\n%s", errStr)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("batch must not remove a directory at the sidecar path")
		}
	})
}

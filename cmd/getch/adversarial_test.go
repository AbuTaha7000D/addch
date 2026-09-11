package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
)

const getchHostileMediaName = "  cafe\u00e9\u0301 \u30e2\u30b8 \u0645\u0631\u062d\u0628\u0627 \U0001f3ac \u0410.   .mp4"

// getchHostileRoot returns a batch/work directory whose own path contains
// Unicode scripts, emoji, and spaces. The directory name deliberately avoids a
// trailing space: Windows cannot reliably address a trailing-space component in
// the middle of a path, so such a name would exercise a platform limitation of
// the fixture rather than the toolkit's path handling.
func getchHostileRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "فيديوهات \u30e2\u30b8 \u0639\u0631\u0628\u064a\u0629 \U0001f3ac movies")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create hostile root: %v", err)
	}
	return root
}

func getchHostileSubdir(t *testing.T, root string) string {
	t.Helper()
	sub := filepath.Join(root, "مجلد فرعي \u30b5\u30d6 \U0001f3ac")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("create hostile subdir: %v", err)
	}
	return sub
}

func getchSymlinkForTest(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

func TestGetchAdversarialHostileNameExtract(t *testing.T) {
	requireTools(t)

	dir := getchHostileRoot(t)
	video := makeChapteredFixture(t, dir, getchHostileMediaName, "mp4", "10")

	out, errStr, code := runExtractForTest(video, "", false)
	if code != 0 {
		t.Fatalf("extract failed code %d:\n%s", code, errStr)
	}
	if out != chapters.Format(seedChapters) {
		t.Errorf("chapter output mismatch:\n%q", out)
	}
}

func TestGetchAdversarialUnicodeCustomOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "video.mp4", "mp4", "10")

	out := filepath.Join(dir, "نتيجة نهائية \u30a2\u30a6\u30c8\u30d7\u30c3\u30c8 \U0001f3ac with spaces.txt")
	gotOut, errStr, code := runExtractForTest(video, out, false)
	if code != 0 {
		t.Fatalf("extract failed code %d:\n%s", code, errStr)
	}
	if gotOut != "" {
		t.Errorf("file-output mode wrote chapter data to stdout: %q", gotOut)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != chapters.Format(seedChapters) {
		t.Errorf("chapter file mismatch:\n%s", data)
	}
}

func TestGetchAdversarialBatchHostileNames(t *testing.T) {
	requireTools(t)

	root := getchHostileRoot(t)
	a := filepath.Join(root, "  درس إعدادي \u30ac\u30a4\u30c9 \U0001f3ad.mp4")
	b := filepath.Join(root, "  \u043c\u043e\u0434\u0443\u043b\u044c \u4e8c \u2605.mkv")
	c := filepath.Join(root, "  preexisting \u0645\u0633\u0628\u0642\u0627\u064b.mp4")
	makeChapteredFixture(t, root, filepath.Base(a), "mp4", "10")
	makeChapteredFixture(t, root, filepath.Base(b), "mkv", "10")
	makeChapteredFixture(t, root, filepath.Base(c), "mp4", "10")
	if err := os.WriteFile(fsutil.SidecarPath(c), []byte("stale sidecar"), 0o644); err != nil {
		t.Fatalf("pre-create sidecar: %v", err)
	}

	out, errStr, code := runBatchForTest(root, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 3 | Succeeded: 2 | Skipped: 1 | Failed: 0") {
		t.Errorf("unexpected summary (existing sidecar must be skipped):\n%s", errStr)
	}
	for _, v := range []string{a, b} {
		data, err := os.ReadFile(fsutil.SidecarPath(v))
		if err != nil {
			t.Fatalf("read sidecar for %q: %v", v, err)
		}
		if string(data) != chapters.Format(seedChapters) {
			t.Errorf("sidecar content mismatch for %q:\n%s", v, data)
		}
	}
	if got, _ := os.ReadFile(fsutil.SidecarPath(c)); string(got) != "stale sidecar" {
		t.Error("existing sidecar must not be overwritten without --overwrite")
	}
}

func TestGetchAdversarialBatchRecursiveHostileSubdir(t *testing.T) {
	requireTools(t)

	root := getchHostileRoot(t)
	sub := getchHostileSubdir(t, root)
	makeChapteredFixture(t, root, "a.mp4", "mp4", "10")
	makeChapteredFixture(t, sub, "b\u00e9.mp4", "mp4", "10")

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 2 | Succeeded: 2 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", errStr)
	}
	if _, err := os.ReadFile(fsutil.SidecarPath(filepath.Join(sub, "b\u00e9.mp4"))); err != nil {
		t.Errorf("expected sidecar in hostile subdir: %v", err)
	}
}

func TestGetchAdversarialSymlinkInput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	getchSymlinkForTest(t, real, link)

	out, errStr, code := runExtractForTest(link, "", false)
	if code != 0 {
		t.Fatalf("extract via symlink failed code %d:\n%s", code, errStr)
	}
	if out != chapters.Format(seedChapters) {
		t.Errorf("chapter output mismatch through symlink:\n%q", out)
	}
}

func TestGetchAdversarialBatchSymlinkMedia(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	getchSymlinkForTest(t, real, link)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 2 | Succeeded: 2 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", errStr)
	}
	for _, v := range []string{real, link} {
		if _, err := os.ReadFile(fsutil.SidecarPath(v)); err != nil {
			t.Errorf("expected sidecar for %q: %v", v, err)
		}
	}
}

func TestGetchAdversarialBatchSymlinkedDirNoDoubleProcess(t *testing.T) {
	requireTools(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	makeChapteredFixture(t, realDir, "clip.mp4", "mp4", "10")
	getchSymlinkForTest(t, realDir, filepath.Join(root, "alias"))

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 1") {
		t.Errorf("symlinked dir must not double-process; got:\n%s", errStr)
	}
	if _, err := os.ReadFile(fsutil.SidecarPath(filepath.Join(realDir, "clip.mp4"))); err != nil {
		t.Errorf("expected sidecar in real dir: %v", err)
	}
}

func TestGetchAdversarialBatchBrokenSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "ok.mp4", "mp4", "10")
	getchSymlinkForTest(t, filepath.Join(dir, "missing-target.mp4"), filepath.Join(dir, "broken.mp4"))

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch with broken symlink failed code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 1") {
		t.Errorf("broken symlink must not be a candidate; got:\n%s", errStr)
	}
	if _, err := os.ReadFile(fsutil.SidecarPath(real)); err != nil {
		t.Errorf("expected sidecar for real candidate: %v", err)
	}
}

func TestGetchAdversarialSamePathViaSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	getchSymlinkForTest(t, real, link)

	for name, test := range map[string]struct {
		video, output string
	}{
		"link input, real output": {video: link, output: real},
		"real input, link output": {video: real, output: link},
	} {
		_, errStr, code := runExtractForTest(test.video, test.output, false)
		if code == 0 {
			t.Fatalf("%s: expected same-path guard to reject the run", name)
		}
		if !strings.Contains(errStr, "would overwrite the input video") {
			t.Errorf("%s: unexpected error: %s", name, errStr)
		}
	}
}

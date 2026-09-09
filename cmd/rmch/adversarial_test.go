package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

const rmchHostileMediaName = "  cafe\u00e9\u0301 \u30e2\u30b8 \u0645\u0631\u062d\u0628\u0627 \U0001f3ac \u0410.   .mp4"

func rmchHostileRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "فيديوهات \u30e2\u30b8 \u0639\u0631\u0628\u064a\u0629 \U0001f3ac movies ")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create hostile root: %v", err)
	}
	return root
}

func rmchHostileSubdir(t *testing.T, root string) string {
	t.Helper()
	sub := filepath.Join(root, "مجلد فرعي \u30b5\u30d6 \U0001f3ac")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("create hostile subdir: %v", err)
	}
	return sub
}

func rmchSymlinkForTest(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

// readBytes returns the current contents of path.
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestRmchAdversarialHostileNameRemove(t *testing.T) {
	requireTools(t)

	dir := rmchHostileRoot(t)
	video := makeChapteredFixture(t, dir, rmchHostileMediaName, "mp4", "10")
	original := readBytes(t, video)

	_, errStr, code := runRemovalForTest(video, "", false)
	if code != 0 {
		t.Fatalf("remove failed code %d:\n%s", code, errStr)
	}

	outPath := fsutil.DefaultNoChaptersOutputPath(video)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output %q: %v", outPath, err)
	}
	requireZeroChapters(t, outPath)
	if !bytes.Equal(readBytes(t, video), original) {
		t.Error("chaptered input was modified")
	}
	if countTempMetadata() != 0 {
		t.Error("temporary metadata file leaked")
	}
}

func TestRmchAdversarialUnicodeCustomOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "video.mp4", "mp4", "10")

	out := filepath.Join(dir, "نتيجة نهائية \u30a2\u30a6\u30c8\u30d7\u30c3\u30c8 \U0001f3ac with spaces.mp4")
	_, errStr, code := runRemovalForTest(video, out, false)
	if code != 0 {
		t.Fatalf("remove failed code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output %q: %v", out, err)
	}
	requireZeroChapters(t, out)
}

func TestRmchAdversarialBatchHostileNames(t *testing.T) {
	requireTools(t)

	root := rmchHostileRoot(t)
	a := filepath.Join(root, "  درس إعدادي \u30ac\u30a4\u30c9 \U0001f3ad.mp4")
	b := filepath.Join(root, "  \u043c\u043e\u0434\u0443\u043b\u044c \u4e8c \u2605.mkv")
	makeChapteredFixture(t, root, filepath.Base(a), "mp4", "10")
	makeChapteredFixture(t, root, filepath.Base(b), "mkv", "10")

	out, errStr, code := runBatchForTest(root, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 2 candidates processed, got:\n%s", out)
	}
	for _, v := range []string{a, b} {
		requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(v))
	}
}

func TestRmchAdversarialBatchRecursiveHostileSubdir(t *testing.T) {
	requireTools(t)

	root := rmchHostileRoot(t)
	sub := rmchHostileSubdir(t, root)
	makeChapteredFixture(t, root, "a.mp4", "mp4", "10")
	makeChapteredFixture(t, sub, "b\u00e9.mp4", "mp4", "10")

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 2 candidates found, got:\n%s", out)
	}
	requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(filepath.Join(sub, "b\u00e9.mp4")))
}

func TestRmchAdversarialSymlinkInput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	rmchSymlinkForTest(t, real, link)

	_, errStr, code := runRemovalForTest(link, "", false)
	if code != 0 {
		t.Fatalf("remove via symlink failed code %d:\n%s", code, errStr)
	}

	outPath := fsutil.DefaultNoChaptersOutputPath(link)
	info, err := os.Lstat(outPath)
	if err != nil {
		t.Fatalf("expected output %q: %v", outPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("output must be a real file, not a symlink")
	}
	requireZeroChapters(t, outPath)
	requirePreservedStreams(t, real, outPath)
}

func TestRmchAdversarialBatchSymlinkMedia(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	rmchSymlinkForTest(t, real, link)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected both candidates processed, got:\n%s", out)
	}
	realOut := fsutil.DefaultNoChaptersOutputPath(real)
	linkOut := fsutil.DefaultNoChaptersOutputPath(link)
	requireZeroChapters(t, realOut)
	requireZeroChapters(t, linkOut)
	if fsutil.SamePath(realOut, linkOut) {
		t.Error("real and link outputs resolved to the same file")
	}
}

func TestRmchAdversarialBatchSymlinkedDirNoDoubleProcess(t *testing.T) {
	requireTools(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	makeChapteredFixture(t, realDir, "clip.mp4", "mp4", "10")
	rmchSymlinkForTest(t, realDir, filepath.Join(root, "alias"))

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 1") {
		t.Errorf("symlinked dir must not double-process; got:\n%s", out)
	}
	requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(filepath.Join(realDir, "clip.mp4")))
}

func TestRmchAdversarialBatchBrokenSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "ok.mp4", "mp4", "10")
	rmchSymlinkForTest(t, filepath.Join(dir, "missing-target.mp4"), filepath.Join(dir, "broken.mp4"))

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch with broken symlink failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 1") {
		t.Errorf("broken symlink must not be a candidate; got:\n%s", out)
	}
	requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(real))
}

func TestRmchAdversarialSamePathViaSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeChapteredFixture(t, dir, "real.mp4", "mp4", "10")
	link := filepath.Join(dir, "link.mp4")
	rmchSymlinkForTest(t, real, link)

	for name, test := range map[string]struct {
		video, output string
	}{
		"link input, real output": {video: link, output: real},
		"real input, link output": {video: real, output: link},
	} {
		_, errStr, code := runRemovalForTest(test.video, test.output, false)
		if code == 0 {
			t.Fatalf("%s: expected same-path guard to reject the run", name)
		}
		if !strings.Contains(errStr, "would overwrite the input video") {
			t.Errorf("%s: unexpected error: %s", name, errStr)
		}
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// hostileMediaName mixes leading/trailing spaces with CJK, RTL script, Latin
// with a combining diacritic, emoji, and Cyrillic so every path-handling layer
// of the toolkit is exercised at once.
const hostileMediaName = "  cafe\u00e9\u0301 \u30e2\u30b8 \u0645\u0631\u062d\u0628\u0627 \U0001f3ac \u0410.   .mp4"

// hostileRoot returns a batch/work directory whose own path contains Unicode
// scripts, emoji, and spaces.
func hostileRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "فيديوهات \u30e2\u30b8 \u0639\u0631\u0628\u064a\u0629 \U0001f3ac movies ")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create hostile root: %v", err)
	}
	return root
}

// hostileSubdir creates a nested directory with a hostile name under root.
func hostileSubdir(t *testing.T, root string) string {
	t.Helper()
	sub := filepath.Join(root, "مجلد فرعي \u30b5\u30d6 \U0001f3ac")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("create hostile subdir: %v", err)
	}
	return sub
}

// symlinkForTest creates a symlink or skips the test when the platform cannot
// create one (mirrors the fsutil test convention).
func symlinkForTest(t *testing.T, oldname, newname string) {
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

func TestAddchAdversarialHostileNameEmbed(t *testing.T) {
	requireTools(t)

	dir := hostileRoot(t)
	video := makeTestVideo(t, dir, "10")
	hostile := filepath.Join(dir, hostileMediaName)
	if err := os.Rename(video, hostile); err != nil {
		t.Fatalf("rename to hostile name: %v", err)
	}

	content := "00:00:00 עברית 🎬 начало العربية\n" +
		"00:00:05 日本語 test cafe\u0301\n"
	chaptersFile := filepath.Join(dir, "فهرست chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}
	original := readBytes(t, hostile)

	_, errStr, code := runEmbedForTest(chaptersFile, hostile, "", false)
	if code != 0 {
		t.Fatalf("embed failed code %d:\n%s", code, errStr)
	}

	outPath := fsutil.DefaultOutputPath(hostile)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output %q: %v", outPath, err)
	}
	got := probeChaptersRaw(t, outPath)
	if len(got) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(got))
	}
	for i, want := range []string{"עברית 🎬 начало العربية", "日本語 test cafe\u0301"} {
		if got[i].Tags.Title != want {
			t.Errorf("chapter %d title = %q, want %q", i, got[i].Tags.Title, want)
		}
	}
	if !bytes.Equal(readBytes(t, hostile), original) {
		t.Error("input video was modified")
	}
	if countTempMetadata() != 0 {
		t.Error("temporary metadata file leaked")
	}
}

func TestAddchAdversarialUnicodeCustomOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Final\n"), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	out := filepath.Join(dir, "نتيجة نهائية \u30a2\u30a6\u30c8\u30d7\u30c3\u30c8 \U0001f3ac with spaces.mp4")
	_, errStr, code := runEmbedForTest(chaptersFile, video, out, false)
	if code != 0 {
		t.Fatalf("embed failed code %d:\n%s", code, errStr)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output %q: %v", out, err)
	}
	if len(probeChaptersRaw(t, out)) != 2 {
		t.Error("expected 2 chapters in custom output")
	}
}

func TestAddchAdversarialBatchHostileNames(t *testing.T) {
	requireTools(t)

	root := hostileRoot(t)
	a := filepath.Join(root, "  درس إعدادي \u30ac\u30a4\u30c9 \U0001f3ad.mp4")
	b := filepath.Join(root, "  \u043c\u043e\u0434\u0443\u043b\u044c \u4e8c \u2605.mkv")
	makeNamedContainer(t, root, filepath.Base(a), "mp4", "8")
	makeNamedContainer(t, root, filepath.Base(b), "mkv", "8")
	writeSidecar(t, a, batchChapters)
	writeSidecar(t, b, batchChapters)

	stale := filepath.Join(root, "existing service-chapters.mp4")
	if err := os.WriteFile(stale, []byte("not real media"), 0o644); err != nil {
		t.Fatalf("write stale generated output: %v", err)
	}

	out, errStr, code := runBatchForTest(root, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	for _, v := range []string{a, b} {
		if _, err := os.Stat(fsutil.DefaultOutputPath(v)); err != nil {
			t.Errorf("expected output for %q: %v", v, err)
		}
	}
	if len(probeChaptersRaw(t, fsutil.DefaultOutputPath(a))) != 3 {
		t.Error("expected 3 chapters in embedded output")
	}
	if strings.Contains(out, "existing service") {
		t.Errorf("stale generated output was discovered:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "existing service-chapters-chapters.mp4")); !os.IsNotExist(err) {
		t.Error("stale generated output was re-processed")
	}
}

func TestAddchAdversarialBatchRecursiveHostileSubdir(t *testing.T) {
	requireTools(t)

	root := hostileRoot(t)
	sub := hostileSubdir(t, root)
	makeNamedContainer(t, root, "a.mp4", "mp4", "8")
	makeNamedContainer(t, sub, "b\u00e9.mp4", "mp4", "8")
	writeSidecar(t, filepath.Join(root, "a.mp4"), batchChapters)
	writeSidecar(t, filepath.Join(sub, "b\u00e9.mp4"), batchChapters)

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected 2 candidates found, got:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(filepath.Join(sub, "b\u00e9.mp4"))); err != nil {
		t.Errorf("expected output in hostile subdir: %v", err)
	}
}

func TestAddchAdversarialSymlinkInput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeTestVideo(t, dir, "10")
	link := filepath.Join(dir, "link.mp4")
	symlinkForTest(t, real, link)

	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:05 Final\n"), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}
	original := readBytes(t, real)

	_, errStr, code := runEmbedForTest(chaptersFile, link, "", false)
	if code != 0 {
		t.Fatalf("embed via symlink failed code %d:\n%s", code, errStr)
	}

	outPath := fsutil.DefaultOutputPath(link)
	info, err := os.Lstat(outPath)
	if err != nil {
		t.Fatalf("expected output %q: %v", outPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("output must be a real file, not a symlink")
	}
	if len(probeChaptersRaw(t, outPath)) != 2 {
		t.Error("expected 2 chapters in symlink-derived output")
	}
	if !bytes.Equal(readBytes(t, real), original) {
		t.Error("symlink target was modified")
	}
}

func TestAddchAdversarialBatchSymlinkMedia(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeNamedContainer(t, dir, "real.mp4", "mp4", "8")
	writeSidecar(t, real, batchChapters)
	link := filepath.Join(dir, "link.mp4")
	symlinkForTest(t, real, link)
	writeSidecar(t, link, batchChapters)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 2") {
		t.Errorf("expected both candidates processed, got:\n%s", out)
	}
	realOut := fsutil.DefaultOutputPath(real)
	linkOut := fsutil.DefaultOutputPath(link)
	if _, err := os.Stat(realOut); err != nil {
		t.Errorf("expected output %q: %v", realOut, err)
	}
	if _, err := os.Stat(linkOut); err != nil {
		t.Errorf("expected output %q: %v", linkOut, err)
	}
	if fsutil.SamePath(realOut, linkOut) {
		t.Error("real and link outputs resolved to the same file")
	}
}

func TestAddchAdversarialBatchSymlinkedDirNoDoubleProcess(t *testing.T) {
	requireTools(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	makeNamedContainer(t, realDir, "clip.mp4", "mp4", "8")
	writeSidecar(t, filepath.Join(realDir, "clip.mp4"), batchChapters)
	symlinkForTest(t, realDir, filepath.Join(root, "alias"))

	out, errStr, code := runBatchForTest(root, true, false)
	if code != 0 {
		t.Fatalf("recursive batch failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 1") {
		t.Errorf("symlinked dir must not double-process; got:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(filepath.Join(realDir, "clip.mp4"))); err != nil {
		t.Errorf("expected output in real dir: %v", err)
	}
}

func TestAddchAdversarialBatchBrokenSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeNamedContainer(t, dir, "ok.mp4", "mp4", "8")
	writeSidecar(t, real, batchChapters)
	symlinkForTest(t, filepath.Join(dir, "missing-target.mp4"), filepath.Join(dir, "broken.mp4"))

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("batch with broken symlink failed code %d:\n%s", code, errStr)
	}
	if !strings.Contains(out, "Total: 1") {
		t.Errorf("broken symlink must not be a candidate; got:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(real)); err != nil {
		t.Errorf("expected output for real candidate: %v", err)
	}
}

func TestAddchAdversarialSamePathViaSymlink(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	real := makeTestVideo(t, dir, "10")
	link := filepath.Join(dir, "link.mp4")
	symlinkForTest(t, real, link)

	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n"), 0o644); err != nil {
		t.Fatalf("write chapters: %v", err)
	}

	for name, test := range map[string]struct {
		video, output string
	}{
		"link input, real output": {video: link, output: real},
		"real input, link output": {video: real, output: link},
	} {
		_, errStr, code := runEmbedForTest(chaptersFile, test.video, test.output, false)
		if code == 0 {
			t.Fatalf("%s: expected same-path guard to reject the run", name)
		}
		if !strings.Contains(errStr, "would overwrite the input video") {
			t.Errorf("%s: unexpected error: %s", name, errStr)
		}
	}
}

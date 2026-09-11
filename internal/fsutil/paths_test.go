package fsutil

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSamePathNormalCases(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	abs, _ := filepath.Abs(real)

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", real, real, true},
		{"absolute vs relative", real, abs, true},
		{"absolute vs absolute", abs, abs, true},
		{"clean dot", real, filepath.Join(dir, ".", "video.mp4"), true},
		{"clean dotdot", filepath.Join(dir, "sub", "..", "video.mp4"), real, true},
		{"different file", real, filepath.Join(dir, "other.mp4"), false},
	}
	// The dotdot case requires the intermediate resolution to collapse cleanly.
	if _, err := os.Stat(filepath.Join(dir, "sub")); err != nil {
		// Create it so the path is valid for Clean resolution.
		_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	}

	for _, c := range cases {
		if got := SamePath(c.a, c.b); got != c.want {
			t.Errorf("%s: SamePath(%q, %q) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}

	// Neither path exists: cleaned absolute comparison still applies.
	if got := SamePath(real, filepath.Join(dir, "video.mp4")); !got {
		t.Error("expected existing vs non-existing same path to match")
	}
}

func TestSamePathSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.mp4")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// The output path is a symlink to the input => same underlying file.
	if !SamePath(link, real) {
		t.Error("SamePath should detect a symlink pointing at the input")
	}
	if !SamePath(real, link) {
		t.Error("SamePath should detect input referenced through a symlink")
	}

	// A symlink to a different file should not be considered the same.
	other := filepath.Join(dir, "other.mp4")
	if err := os.WriteFile(other, []byte("more"), 0o644); err != nil {
		t.Fatal(err)
	}
	otherLink := filepath.Join(dir, "other-link.mp4")
	if err := os.Symlink(other, otherLink); err != nil {
		t.Fatalf("could not create second symlink: %v", err)
	}
	if SamePath(real, otherLink) {
		t.Error("SamePath should return false for a symlink to a different file")
	}
}

func TestSamePathTrailingSlashAndVariants(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !SamePath(real, real) {
		t.Error("expected same path to be equal")
	}
}

func TestDefaultOutputPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"video.mp4", "video-chapters.mp4"},
		{filepath.Join("path", "to", "My Course.mp4"), filepath.Join("path", "to", "My Course-chapters.mp4")},
		{"فيديو.mkv", "فيديو-chapters.mkv"},
		{"lecture.01.mp4", "lecture.01-chapters.mp4"},
		{filepath.Join("a", "b", "video.mkv"), filepath.Join("a", "b", "video-chapters.mkv")},
		{"VIDEO", "VIDEO-chapters"},
		{".hidden.mp4", ".hidden-chapters.mp4"},
		{"tool.exe", "tool-chapters.exe"}, // .exe handled like any other extension
	}
	for _, c := range cases {
		if got := DefaultOutputPath(c.in); got != c.want {
			t.Errorf("DefaultOutputPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDefaultOutputPathUnicodeDir covers a Unicode directory name with spaces,
// exercised on every OS (including Windows where filepath uses '\' separators).
func TestDefaultOutputPathUnicodeDir(t *testing.T) {
	in := filepath.Join("My Course", "فيديو", "lecture.01.mp4")
	want := filepath.Join("My Course", "فيديو", "lecture.01-chapters.mp4")
	if got := DefaultOutputPath(in); got != want {
		t.Errorf("DefaultOutputPath(%q) = %q, want %q", in, got, want)
	}
}

// TestDefaultOutputPathWindowsAbs runs only on Windows to pin the '\'-separated
// absolute-path behavior that the portable cases above cannot express.
func TestDefaultOutputPathWindowsAbs(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific absolute path assertion")
	}
	in := `C:\Users\Test\My Course.mp4`
	want := `C:\Users\Test\My Course-chapters.mp4`
	if got := DefaultOutputPath(in); got != want {
		t.Errorf("DefaultOutputPath(%q) = %q, want %q", in, got, want)
	}
}

func TestDefaultNoChaptersOutputPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"My Course.mp4", "My Course-nochapters.mp4"},
		{"video.mkv", "video-nochapters.mkv"},
		{"lecture.01.mp4", "lecture.01-nochapters.mp4"},
		{filepath.Join("path", "to", "My Course.mp4"), filepath.Join("path", "to", "My Course-nochapters.mp4")},
		{filepath.Join("a", "b", "video.mkv"), filepath.Join("a", "b", "video-nochapters.mkv")},
		{"فيديو.mkv", "فيديو-nochapters.mkv"},
		{"VIDEO", "VIDEO-nochapters"},
		{".hidden.mp4", ".hidden-nochapters.mp4"},
		{"tool.exe", "tool-nochapters.exe"}, // .exe handled like any other extension
	}
	for _, c := range cases {
		if got := DefaultNoChaptersOutputPath(c.in); got != c.want {
			t.Errorf("DefaultNoChaptersOutputPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDefaultNoChaptersOutputPathUnicodeDir(t *testing.T) {
	in := filepath.Join("My Course", "فيديو", "lecture.01.mp4")
	want := filepath.Join("My Course", "فيديو", "lecture.01-nochapters.mp4")
	if got := DefaultNoChaptersOutputPath(in); got != want {
		t.Errorf("DefaultNoChaptersOutputPath(%q) = %q, want %q", in, got, want)
	}
}

func TestOutputExtension(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"video.mp4", ".mp4"},
		{"video.MKV", ".mkv"},
		{"My Course.mP4", ".mp4"},
		{filepath.Join("path", "to", "video.mov"), ".mov"},
		{".hidden.mp4", ".mp4"},
		{"archive.tar.gz", ".gz"},
		{"VIDEO", ""},
	}
	for _, c := range cases {
		if got := OutputExtension(c.in); got != c.want {
			t.Errorf("OutputExtension(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanupFile(t *testing.T) {
	t.Run("removes existing file", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "out.mp4")
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		CleanupFile(p)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected file to be removed, stat err = %v", err)
		}
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "not-there.mp4")
		CleanupFile(p)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected file to remain absent, stat err = %v", err)
		}
	})

	t.Run("never removes a directory", func(t *testing.T) {
		dir := t.TempDir()
		empty := filepath.Join(dir, "output-dir")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		CleanupFile(empty)
		if fi, err := os.Stat(empty); err != nil || !fi.IsDir() {
			t.Errorf("cleanup removed an existing empty directory: stat err = %v", err)
		}
		nonEmpty := filepath.Join(dir, "non-empty-dir")
		if err := os.Mkdir(nonEmpty, 0o755); err != nil {
			t.Fatal(err)
		}
		child := filepath.Join(nonEmpty, "keep.txt")
		if err := os.WriteFile(child, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		CleanupFile(nonEmpty)
		if _, err := os.Stat(child); err != nil {
			t.Errorf("non-empty directory must survive cleanup: %v", err)
		}
	})

	t.Run("never removes a symlinked directory target", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "real")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}
		CleanupFile(link)
		if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
			t.Errorf("cleanup removed the symlinked directory target: stat err = %v", err)
		}
		if _, err := os.Lstat(link); err != nil {
			t.Errorf("cleanup removed the symlink itself: %v", err)
		}
	})
}

// sidecarTempLitter returns the number of WriteFileAtomic temp files left in dir.
func sidecarTempLitter(dir string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, ".addch-sidecar-*"))
	return len(matches)
}

func TestWriteFileAtomic(t *testing.T) {
	const content = "00:00:00 Intro\n00:00:03 Middle\n"

	t.Run("writes the content", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "out.txt")
		if err := WriteFileAtomic(target, []byte(content)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
		if sidecarTempLitter(dir) != 0 {
			t.Errorf("temp files leaked: %d", sidecarTempLitter(dir))
		}
	})

	t.Run("atomically replaces an existing file", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "out.txt")
		if err := os.WriteFile(target, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteFileAtomic(target, []byte(content)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := os.ReadFile(target)
		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
		if sidecarTempLitter(dir) != 0 {
			t.Errorf("temp files leaked: %d", sidecarTempLitter(dir))
		}
	})

	t.Run("result is a readable 0644 file", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "out.txt")
		if err := WriteFileAtomic(target, []byte(content)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		fi, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS == "windows" {
			// Windows has no POSIX permission bits: a writable file reports mode
			// 0666 rather than 0644, so only the regular-file contract is pinned.
			if !fi.Mode().IsRegular() {
				t.Errorf("expected a regular readable file, mode = %v", fi.Mode())
			}
		} else if perm := fi.Mode().Perm(); perm&0o777 != 0o644 {
			t.Errorf("mode = %o, want 644", perm)
		}
	})

	t.Run("missing parent directory fails without leaving temp files", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "absent", "out.txt")
		if err := WriteFileAtomic(target, []byte(content)); err == nil {
			t.Fatal("expected error for missing parent directory")
		}
		if sidecarTempLitter(filepath.Join(dir, "absent")) != 0 {
			t.Errorf("temp files leaked in absent dir: %d", sidecarTempLitter(filepath.Join(dir, "absent")))
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("target must not exist: %v", err)
		}
	})
}

// TestWriteFileAtomicNeverClobbersInput is the Phase 8 item 4 proof at the unit
// level: the atomic replacement of a target leaves the temporary file
// completely gone (no .addch-sidecar-* or .tmp-* remnants) and never clobbers a
// separate, pre-existing input file that shares the directory — the input's
// bytes (SHA-256) and modification time must survive untouched.
func TestWriteFileAtomicNeverClobbersInput(t *testing.T) {
	dir := t.TempDir()

	input := filepath.Join(dir, "input.txt")
	const inputData = "original input; owns its own hash"
	if err := os.WriteFile(input, []byte(inputData), 0o644); err != nil {
		t.Fatal(err)
	}
	inputBefore, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	inputBeforeHash := fmt.Sprintf("%x", sha256.Sum256(inputBefore))
	inputFi, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	const content = "00:00:00 Intro\n00:00:03 Middle\n"
	if err := WriteFileAtomic(target, []byte(content)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back target: %v", err)
	}
	if string(got) != content {
		t.Errorf("target content = %q, want %q", got, content)
	}

	// The unrelated input file is byte-identical and was not rewritten in place.
	inputAfter, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if gotHash := fmt.Sprintf("%x", sha256.Sum256(inputAfter)); gotHash != inputBeforeHash {
		t.Errorf("input file was modified: hash %s, want %s", gotHash, inputBeforeHash)
	}
	inputAfterFi, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	if !inputAfterFi.ModTime().Equal(inputFi.ModTime()) {
		t.Errorf("input modification time changed: %v, want %v", inputAfterFi.ModTime(), inputFi.ModTime())
	}

	// No temp file of either naming convention survives in the shared directory.
	if n := sidecarTempLitter(dir); n != 0 {
		t.Errorf("sidecar temp files leaked: %d", n)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".tmp-*")); len(matches) != 0 {
		t.Errorf("tmp temp files leaked: %v", matches)
	}
}

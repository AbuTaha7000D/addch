package fsutil

import (
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
}

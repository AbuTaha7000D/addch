package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

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
		if got := defaultOutputPath(c.in); got != c.want {
			t.Errorf("defaultOutputPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDefaultOutputPathUnicodeDir covers a Unicode directory name with spaces,
// exercised on every OS (including Windows where filepath uses '\' separators).
func TestDefaultOutputPathUnicodeDir(t *testing.T) {
	in := filepath.Join("My Course", "فيديو", "lecture.01.mp4")
	want := filepath.Join("My Course", "فيديو", "lecture.01-chapters.mp4")
	if got := defaultOutputPath(in); got != want {
		t.Errorf("defaultOutputPath(%q) = %q, want %q", in, got, want)
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
	if got := defaultOutputPath(in); got != want {
		t.Errorf("defaultOutputPath(%q) = %q, want %q", in, got, want)
	}
}

func TestIsMP4Family(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".mp4", true},
		{".m4v", true},
		{".mov", true},
		{".3gp", true},
		{".3g2", true},
		{".MP4", true},
		{".mkv", false},
		{".webm", false},
		{".avi", false},
		{"", false},
		{".mp3", false},
	}
	for _, c := range cases {
		if got := isMP4Family(c.ext); got != c.want {
			t.Errorf("isMP4Family(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}

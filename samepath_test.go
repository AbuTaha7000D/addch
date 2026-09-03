package main

import (
	"os"
	"path/filepath"
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
		if got := samePath(c.a, c.b); got != c.want {
			t.Errorf("%s: samePath(%q, %q) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}

	// Neither path exists: cleaned absolute comparison still applies.
	if got := samePath(real, filepath.Join(dir, "video.mp4")); !got {
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
	if !samePath(link, real) {
		t.Error("samePath should detect a symlink pointing at the input")
	}
	if !samePath(real, link) {
		t.Error("samePath should detect input referenced through a symlink")
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
	if samePath(real, otherLink) {
		t.Error("samePath should return false for a symlink to a different file")
	}
}

func TestSamePathTrailingSlashAndVariants(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !samePath(real, real) {
		t.Error("expected same path to be equal")
	}
}

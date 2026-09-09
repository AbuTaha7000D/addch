package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// symlinkOrSkip creates a symlink or skips the test when the platform cannot
// create one.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

// TestFindMediaFilesSymlinkRootRecursive is a regression test for the Phase 8
// adversarial finding: a recursive walk whose root is a symlink to a directory
// must descend into the target and discover its media files instead of silently
// reporting an empty result.
func TestFindMediaFilesSymlinkRootRecursive(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(real, "clip.mp4"))
	link := filepath.Join(root, "rootlink")
	symlinkOrSkip(t, real, link)

	got, err := FindMediaFiles(link, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "clip.mp4" {
		t.Fatalf("recursive walk through symlink root = %v, want 1 file clip.mp4", got)
	}
}

// TestFindCandidatesSymlinkRootRecursive locks the same fix for the addch side:
// a recursive batch rooted at a directory symlink must find the media/sidecar
// candidate pair inside the target.
func TestFindCandidatesSymlinkRootRecursive(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(real, "clip.mp4"))
	writeFile(t, filepath.Join(real, "clip.txt"))
	link := filepath.Join(root, "rootlink")
	symlinkOrSkip(t, real, link)

	cands, err := FindCandidates(link, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidates through symlink root = %+v, want 1", cands)
	}
	if filepath.Base(cands[0].MediaPath) != "clip.mp4" || filepath.Base(cands[0].SidecarPath) != "clip.txt" {
		t.Fatalf("candidate = %+v, want clip.mp4/clip.txt pair", cands[0])
	}
}

// TestFindMediaFilesSymlinkRootShallow guards the already-correct shallow path:
// os.ReadDir follows the root symlink, so a symlink-to-dir root must discover
// the media file without any fix.
func TestFindMediaFilesSymlinkRootShallow(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(real, "clip.mp4"))
	link := filepath.Join(root, "rootlink")
	symlinkOrSkip(t, real, link)

	got, err := FindMediaFiles(link, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "clip.mp4" {
		t.Fatalf("shallow walk through symlink root = %v, want 1 file clip.mp4", got)
	}
}

// TestFindMediaFilesSymlinkedFileAndDirNonFollowed locks the intentional
// subdir-symlink semantics: a symlink resolving to a FILE is discovered via the
// followed os.Stat check, while a symlink to a DIRECTORY is not descended, so
// its contents are never double-processed and there is no loop risk.
func TestFindMediaFilesSymlinkedFileAndDirNonFollowed(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(real, "clip.mp4"))
	symlinkOrSkip(t, filepath.Join(real, "clip.mp4"), filepath.Join(root, "link.mp4"))
	symlinkOrSkip(t, real, filepath.Join(root, "alias"))

	got, err := FindMediaFiles(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want the real file plus the file symlink only", got)
	}
	for _, p := range got {
		base := filepath.Base(p)
		if base != "clip.mp4" && base != "link.mp4" {
			t.Errorf("unexpected discovered path %q", p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatalf("rel %q from %q: %v", p, root, err)
		}
		components := strings.Split(rel, string(filepath.Separator))
		if components[0] == "alias" {
			t.Errorf("symlinked directory must not be descended or reported: %q", rel)
		}
	}
}

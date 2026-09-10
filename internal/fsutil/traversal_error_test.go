package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Phase 8.5.2 regression: a recursive discovery that hits an entry-level
// traversal error (e.g. an unreadable subdirectory) must abort and surface as
// an error instead of silently dropping the inaccessible paths and reporting a
// complete traversal. The policy lives in walkVisit, which is tested directly
// with synthetic entries — a deterministic mechanism that does not depend on
// OS-specific permission behavior — plus a chmod-based walk test that is
// skipped whenever the unreadable-directory setup is ineffective (privileged
// runners, Windows).

// walkErr is a sentinel proving the callback propagates whatever error
// filepath.WalkDir hands it, without touching the filesystem.
var walkErr = errors.New("simulated traversal failure")

// fakeDirEntry implements fs.DirEntry for synthetic callback testing.
type fakeDirEntry struct {
	dir bool
}

func (fakeDirEntry) Name() string  { return "entry" }
func (f fakeDirEntry) IsDir() bool { return f.dir }
func (f fakeDirEntry) Type() fs.FileMode {
	if f.dir {
		return fs.ModeDir
	}
	return 0
}
func (fakeDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("unused") }

// visitRecorder captures the paths forwarded by the callback under test.
type visitRecorder struct{ got []string }

func (r *visitRecorder) visit(p string) { r.got = append(r.got, p) }

func TestWalkVisitTraversalErrorAborts(t *testing.T) {
	rec := &visitRecorder{}
	ce := walkVisit(rec.visit)

	for _, entry := range []fakeDirEntry{{dir: true}, {dir: false}} {
		name := "dir"
		if !entry.dir {
			name = "file"
		}
		t.Run(name, func(t *testing.T) {
			failing := filepath.Join("root", "locked")
			err := ce(failing, entry, walkErr)
			if err == nil {
				t.Fatal("a traversal error must be returned, not swallowed")
			}
			if !errors.Is(err, walkErr) {
				t.Errorf("error must wrap the cause, got %v", err)
			}
			if !strings.Contains(err.Error(), failing) {
				t.Errorf("error must name the failing path, got %v", err)
			}
			if len(rec.got) != 0 {
				t.Errorf("nothing may be forwarded after a traversal error, got %v", rec.got)
			}
		})
	}
}

func TestWalkVisitDirectorySkipped(t *testing.T) {
	rec := &visitRecorder{}
	ce := walkVisit(rec.visit)

	if err := ce(filepath.Join("root", "sub"), fakeDirEntry{dir: true}, nil); err != nil {
		t.Fatalf("a healthy directory must be skipped without error, got %v", err)
	}
	if len(rec.got) != 0 {
		t.Errorf("a directory must never be forwarded, got %v", rec.got)
	}
}

func TestWalkVisitForwardsMedia(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &visitRecorder{}
	ce := walkVisit(rec.visit)
	if err := ce(media, fakeDirEntry{dir: false}, nil); err != nil {
		t.Fatalf("a healthy media file must be forwarded without error, got %v", err)
	}
	if len(rec.got) != 1 || rec.got[0] != media {
		t.Errorf("expected exactly %q forwarded, got %v", media, rec.got)
	}
}

// dirReadable probes whether dir can actually be listed, so a chmod that was
// ineffective (privileged runners on Unix) does not fake a traversal error.
func dirReadable(dir string) bool {
	_, err := os.ReadDir(dir)
	return err == nil
}

func TestFindMediaFilesRecursiveTraversalError(t *testing.T) {
	testTraversalError(t, func(dir string, recursive bool) error {
		_, err := FindMediaFiles(dir, recursive)
		return err
	})
}

func TestFindCandidatesRecursiveTraversalError(t *testing.T) {
	testTraversalError(t, func(dir string, recursive bool) error {
		_, err := FindCandidates(dir, recursive)
		return err
	})
}

// testTraversalError asserts that a recursive discovery over a tree containing
// an unreadable subdirectory errors instead of silently returning a partial,
// apparently-complete result, while the same tree without the locked directory
// succeeds and while a shallow walk never descends into it.
func testTraversalError(t *testing.T, discover func(dir string, recursive bool) error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits are not enforced on Windows")
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(good, "a.mp4"))
	writeFile(t, filepath.Join(good, "a.txt"))
	writeFile(t, filepath.Join(locked, "hidden.mp4"))
	writeFile(t, filepath.Join(locked, "hidden.txt"))

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(locked, 0o755)
	}()

	// A privileged runner (Unix root) can bypass the permission bits, so next
	// to the Windows skip above, probe that the lock actually took effect
	// before expecting a traversal failure.
	if dirReadable(locked) {
		t.Skip("chmod did not make the directory unreadable (running as root or on Windows)")
	}

	// A locked directory must abort the recursive walk.
	err := discover(dir, true)
	if err == nil {
		t.Fatal("an unreadable subdirectory must abort recursive discovery, not be silently skipped")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("error should name the failing subtree, got: %v", err)
	}

	// The healthy control: the same tree with the lock removed discovers fully.
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := discover(dir, true); err != nil {
		t.Errorf("a fully readable tree must not error, got: %v", err)
	}

	// Shallow discovery never descends into the locked directory, so the lock
	// must be irrelevant there.
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	if err := discover(dir, false); err != nil {
		t.Errorf("a shallow walk must not care about a locked subdirectory, got: %v", err)
	}
}

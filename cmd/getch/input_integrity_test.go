package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 4 (getch): prove the input media file is never modified —
// byte-for-byte (SHA-256) and timestamp-wise (ModTime) — across every getch
// path: probe-only extraction, every fake-ffprobe probe failure mode, file
// output, --overwrite, --check, and same-path collision refusal.

// fileSnapshot is the recorded identity of an input we promise never to touch.
type fileSnapshot struct {
	path    string
	hash    string
	modTime time.Time
}

// sha256Hex returns the hex SHA-256 of a file's current contents.
func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s for hashing: %v", path, err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// snapshotFiles records the hash and modification time of each path.
func snapshotFiles(t *testing.T, paths ...string) []fileSnapshot {
	t.Helper()
	snaps := make([]fileSnapshot, 0, len(paths))
	for _, p := range paths {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s for snapshot: %v", p, err)
		}
		snaps = append(snaps, fileSnapshot{path: p, hash: sha256Hex(t, p), modTime: fi.ModTime()})
	}
	return snaps
}

// assertFilesUntouched re-checks every snapshot: an identical SHA-256 proves the
// bytes did not change and an identical ModTime proves the file was not even
// rewritten in place.
func assertFilesUntouched(t *testing.T, snaps []fileSnapshot) {
	t.Helper()
	for _, s := range snaps {
		t.Helper()
		if got := sha256Hex(t, s.path); got != s.hash {
			t.Errorf("%s was modified: hash %s, want %s", s.path, got, s.hash)
		}
		fi, err := os.Stat(s.path)
		if err != nil {
			t.Fatalf("stat %s after run: %v", s.path, err)
		}
		if !fi.ModTime().Equal(s.modTime) {
			t.Errorf("%s modification time changed: %v, want %v", s.path, fi.ModTime(), s.modTime)
		}
	}
}

func TestGetchInputIntegrityProbeFailureModes(t *testing.T) {
	cases := []struct {
		name      string
		role      string
		wantCode  int
		wantErr   string
		wantPrint bool
	}{
		{"exit-code", "fail", 1, "could not read chapters", false},
		{"garbage-output", "garbage", 1, "could not parse ffprobe chapter output", false},
		{"bad-time-base", "badtimebase", 1, "could not interpret time_base", false},
		{"healthy-control", "ok", 0, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			video := filepath.Join(dir, "clip.mp4")
			if err := os.WriteFile(video, []byte("dummy input; the fake ffprobe decides the outcome"), 0o644); err != nil {
				t.Fatal(err)
			}

			inputs := snapshotFiles(t, video)

			t.Setenv("FAKE_FFPROBE", c.role)
			shim := fakeToolShim(t, "ffprobe")
			var out, errBuf bytes.Buffer
			code := runWithPath(t, []string{video}, shim, &out, &errBuf)
			if code != c.wantCode {
				t.Fatalf("exit = %d, want %d\nstderr:\n%s", code, c.wantCode, errBuf.String())
			}
			if c.wantCode != 0 {
				if !strings.Contains(errBuf.String(), c.wantErr) {
					t.Errorf("stderr should contain %q, got:\n%s", c.wantErr, errBuf.String())
				}
				if out.Len() != 0 {
					t.Errorf("stdout must stay empty on failure, got:\n%s", out.String())
				}
			} else if c.wantPrint {
				if !strings.Contains(out.String(), "Intro") {
					t.Errorf("expected chapters on stdout, got:\n%s", out.String())
				}
			}
			assertFilesUntouched(t, inputs)
		})
	}
}

func TestGetchInputIntegrityFileOutput(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	sidecar := fsutil.SidecarPath(input)
	_, errStr, code := runExtractForTest(input, sidecar, false)
	if code != 0 {
		t.Fatalf("extract failed with code %d:\n%s", code, errStr)
	}
	if data, err := os.ReadFile(sidecar); err != nil {
		t.Fatalf("expected sidecar %s: %v", sidecar, err)
	} else if !strings.Contains(string(data), "Intro") {
		t.Errorf("sidecar should carry the chapters, got:\n%s", data)
	}
	assertFilesUntouched(t, inputs)
}

func TestGetchInputIntegrityOverwrite(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	sidecar := fsutil.SidecarPath(input)
	if _, errStr, code := runExtractForTest(input, sidecar, false); code != 0 {
		t.Fatalf("first extract failed with code %d:\n%s", code, errStr)
	}
	if _, errStr, code := runExtractForTest(input, sidecar, true); code != 0 {
		t.Fatalf("overwrite extract failed with code %d:\n%s", code, errStr)
	}
	assertFilesUntouched(t, inputs)
}

func TestGetchInputIntegritySamePathRefusal(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(video, []byte("dummy input; the same-path guard fires before any probe"), 0o644); err != nil {
		t.Fatal(err)
	}

	inputs := snapshotFiles(t, video)

	// The guard fires before the probe, so FAKE_FFPROBE only needs to answer the
	// dependency gate: the "ok" role keeps the run hermetic.
	t.Setenv("FAKE_FFPROBE", "ok")
	shim := fakeToolShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{"-o", video, video}, shim, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit 1 for output==input, got %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "would overwrite the input video") {
		t.Errorf("expected the same-path guard message, got:\n%s", errBuf.String())
	}
	assertFilesUntouched(t, inputs)
}

func TestGetchInputIntegrityCheckOnly(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	var out, errBuf bytes.Buffer
	if code := runCheck(&out, &errBuf); code != 0 {
		t.Fatalf("--check failed with code %d:\n%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "System is ready.") {
		t.Errorf("expected readiness line, got:\n%s", out.String())
	}
	assertFilesUntouched(t, inputs)
}

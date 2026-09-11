package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

// H1 regression: getch reads chapters with ffprobe and never runs ffmpeg, so an
// ffmpeg-absent PATH must be a perfectly supported environment. These tests
// re-execute the real binary (via the GETCH_TEST_BINARY hook) with a PATH that
// exposes only the requested tools, proving the dependency gate is ffprobe-only.

// makePathShim creates a directory exposing only the given tools (as symlinks
// to the real binaries, suffixed with the platform executable extension) and
// returns it. It is used to run the CLI under a PATH that hides ffmpeg from it
// while still providing ffprobe.
func makePathShim(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range tools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("could not locate %s for PATH shim: %v", tool, err)
		}
		if err := os.Symlink(real, filepath.Join(dir, tool+exeExt)); err != nil {
			t.Fatalf("could not symlink %s into PATH shim: %v", tool, err)
		}
	}
	return dir
}

// runWithPath runs the real getch binary (re-exec) with the given PATH
// override and returns its exit code, capturing stdout/stderr separately.
func runWithPath(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	cmd := exec.Command(exePath(t), args...)
	env := []string{"GETCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "GETCH_TEST_BINARY=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+path)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err == nil {
		return 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	} else {
		t.Fatalf("could not run getch child: %v", err)
		return -1
	}
}

// TestGetchCheckFFprobeOnlySucceeds proves getch --check passes when only
// ffprobe is present: an ffmpeg-less PATH must not block getch.
func TestGetchCheckFFprobeOnlySucceeds(t *testing.T) {
	requireTools(t)

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code != 0 {
		t.Fatalf("getch --check with ffprobe-only PATH: exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "System is ready.") {
		t.Errorf("expected the readiness line on stdout, got:\n%s", out.String())
	}
	if strings.Contains(errBuf.String(), "not found") {
		t.Errorf("no install hint expected on stderr, got:\n%s", errBuf.String())
	}
}

// TestGetchExtractFFprobeOnlySucceeds proves extraction works with an
// ffmpeg-less PATH: the chapters must still be read and printed verbatim.
func TestGetchExtractFFprobeOnlySucceeds(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{video}, shim, &out, &errBuf)
	if code != 0 {
		t.Fatalf("getch extract with ffprobe-only PATH: exit %d, want 0\nstderr:\n%s", code, errBuf.String())
	}
	if got := out.String(); got == "" {
		t.Error("expected chapter text on stdout with ffprobe-only PATH")
	} else if !strings.Contains(got, "Intro") || !strings.Contains(got, "End") {
		t.Errorf("unexpected chapter output:\n%s", got)
	}
	if !strings.Contains(errBuf.String(), "Dependencies found") {
		t.Errorf("expected progress line on stderr, got:\n%s", errBuf.String())
	}
}

// TestGetchBatchFFprobeOnlySucceeds proves batch discovery + extraction works
// with an ffmpeg-less PATH: the sidecar must still be produced.
func TestGetchBatchFFprobeOnlySucceeds(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	batchDir := t.TempDir()
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batchDir, "clip.mp4"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{"--dir", batchDir}, shim, &out, &errBuf)
	if code != 0 {
		t.Fatalf("getch batch with ffprobe-only PATH: exit %d, want 0\nstderr:\n%s", code, errBuf.String())
	}
	sidecar := filepath.Join(batchDir, "clip.txt")
	got, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("expected sidecar %q to be written: %v", sidecar, err)
	}
	if want := chapters.Format(seedChapters); string(got) != want {
		t.Errorf("sidecar content mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestGetchFailsWithoutFFprobe proves the complementary case: when ffprobe is
// entirely absent, getch --check fails with a scoped hint (exit 1), even though
// ffmpeg might be present.
func TestGetchFailsWithoutFFprobe(t *testing.T) {
	requireTools(t)

	shim := makePathShim(t, "ffmpeg")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("getch --check without ffprobe must exit nonzero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffprobe") || !strings.Contains(errBuf.String(), "not found") {
		t.Errorf("expected an ffprobe scoped hint on stderr, got:\n%s", errBuf.String())
	}
}

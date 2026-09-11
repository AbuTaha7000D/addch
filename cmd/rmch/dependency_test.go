package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// H1 regression: even though getch now accepts an ffmpeg-less PATH, rmch strips
// chapters with ffmpeg and must still REQUIRE both tools. Re-exec the real
// binary (via the RMCH_TEST_BINARY hook) with a PATH exposing only ffprobe and
// prove --check refuses to proceed.

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

func runRmchWithPath(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	return runRmchChild(t, args, path, false, stdout, stderr)
}

// runRmchFakeTool runs the real rmch binary with the fake-tool aliases in shim
// resolving first, while every other tool (in particular the real ffmpeg and
// ffprobe) still resolves from the normal environment PATH — the same
// resolution path the CLI uses in production. Fake alias names never collide
// with real tools in a fakeToolShim, so appending the real PATH is lossless.
func runRmchFakeTool(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	return runRmchChild(t, args, path, true, stdout, stderr)
}

func runRmchChild(t *testing.T, args []string, path string, realPath bool, stdout, stderr io.Writer) int {
	t.Helper()
	cmd := exec.Command(exePath(t), args...)
	env := []string{"RMCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "RMCH_TEST_BINARY=") {
			continue
		}
		env = append(env, kv)
	}
	if realPath {
		env = append(env, "PATH="+path+string(os.PathListSeparator)+os.Getenv("PATH"))
	} else {
		env = append(env, "PATH="+path)
	}
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err == nil {
		return 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	} else {
		t.Fatalf("could not run rmch child: %v", err)
		return -1
	}
}

// TestRmchCheckRequiresBothTools proves rmch still refuses to run when ffmpeg
// is absent even though ffprobe is present.
func TestRmchCheckRequiresBothTools(t *testing.T) {
	requireTools(t)

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runRmchWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("rmch --check with ffprobe-only PATH must exit nonzero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffmpeg") {
		t.Errorf("expected an ffmpeg requirement hint on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.String(), "System is ready.") {
		t.Errorf("rmch must not report ready with ffprobe-only PATH, got:\n%s", out.String())
	}
}

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

// H1 regression: even though getch now accepts an ffmpeg-less PATH, addch
// embeds chapters with ffmpeg and must still REQUIRE both tools. Re-exec the
// real binary (via the ADDCH_TEST_BINARY hook) with a PATH exposing only
// ffprobe and prove --check refuses to proceed.

func makePathShim(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range tools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("could not locate %s for PATH shim: %v", tool, err)
		}
		if err := os.Symlink(real, filepath.Join(dir, tool)); err != nil {
			t.Fatalf("could not symlink %s into PATH shim: %v", tool, err)
		}
	}
	return dir
}

func runAddchWithPath(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	cmd := exec.Command(exePath(t), args...)
	env := []string{"ADDCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "ADDCH_TEST_BINARY=") {
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
		t.Fatalf("could not run addch child: %v", err)
		return -1
	}
}

// TestAddchCheckRequiresBothTools proves addch still refuses to run when ffmpeg
// is absent even though ffprobe is present.
func TestAddchCheckRequiresBothTools(t *testing.T) {
	requireTools(t)

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("addch --check with ffprobe-only PATH must exit nonzero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffmpeg") {
		t.Errorf("expected an ffmpeg requirement hint on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.String(), "System is ready.") {
		t.Errorf("addch must not report ready with ffprobe-only PATH, got:\n%s", out.String())
	}
}

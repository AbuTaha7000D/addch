package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Phase 8 item 3: failure-injection coverage for getch, using the same
// PATH-shim fake-tool machinery as the addch/rmch suites. getch needs only
// ffprobe, so the fakes here drive the probe path end to end; the "ok" role is
// a positive control proving the fake mechanism itself can succeed.

// maybeRunFakeTool lets the test binary masquerade as a fake ffmpeg/ffprobe
// when it is re-executed from a PATH shim under the matching name. It is called
// from TestMain ahead of the CLI re-exec hook.
func maybeRunFakeTool() bool {
	switch strings.ToLower(filepath.Base(os.Args[0])) {
	case "ffmpeg":
		if role := os.Getenv("FAKE_FFMPEG"); role != "" {
			fakeFFmpeg(role)
			return true
		}
	case "ffprobe":
		if role := os.Getenv("FAKE_FFPROBE"); role != "" {
			fakeFFprobe(role)
			return true
		}
	}
	return false
}

// fakeFFmpeg is kept for symmetry with the other suites' harness even though
// getch never runs ffmpeg; only the -version gate call succeeds.
func fakeFFmpeg(role string) {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "-version" {
		fmt.Println("ffmpeg version 5.1.3-testsuite")
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "getch never invokes ffmpeg; unexpected "+role+" role")
	os.Exit(2)
}

// fakeFFprobe plays an ffprobe on the shim PATH. Only the -version gate call
// succeeds; probing invocations behave per role.
func fakeFFprobe(role string) {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "-version" {
		fmt.Println("ffprobe version 5.1.3-testsuite")
		os.Exit(0)
	}
	switch role {
	case "fail":
		fmt.Fprintln(os.Stderr, "simulated probe failure")
		os.Exit(1)
	case "garbage":
		fmt.Print("this is {{{not json")
		os.Exit(0)
	case "badtimebase":
		fmt.Print(`{"chapters":[{"id":0,"time_base":"garbage","start":5,"tags":{"title":"X"}}]}`)
		os.Exit(0)
	case "ok":
		fmt.Print(`{"chapters":[{"id":0,"time_base":"1/1000","start":0,"tags":{"title":"Intro"}},{"id":1,"time_base":"1/1000","start":6000,"tags":{"title":"End"}}]}`)
		os.Exit(0)
	default:
		fmt.Fprintln(os.Stderr, "unknown FAKE_FFPROBE role "+role)
		os.Exit(2)
	}
}

// fakeToolShim builds a PATH directory that exposes fakeTool as a symlink to
// the test binary itself (which dispatches via maybeRunFakeTool) plus the
// requested real tools, and returns the directory.
func fakeToolShim(t *testing.T, fakeTool string, realTools ...string) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary for fake %s: %v", fakeTool, err)
	}
	getchSymlinkForTest(t, exe, filepath.Join(dir, fakeTool))
	for _, tool := range realTools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("could not locate real %s for PATH shim: %v", tool, err)
		}
		getchSymlinkForTest(t, real, filepath.Join(dir, tool))
	}
	return dir
}

// TestGetchFakeFFprobeRejections proves extraction surfaces every class of
// broken ffprobe output: nonzero exit, non-JSON garbage, and chapters with an
// uninterpretable time_base. Stdout (which must stay reserved for chapter data)
// stays empty at exit 1.
func TestGetchFakeFFprobeRejections(t *testing.T) {
	cases := []struct {
		name, role, wantErr string
	}{
		{"exit-code", "fail", "could not read chapters"},
		{"garbage-output", "garbage", "could not parse ffprobe chapter output"},
		{"bad-time-base", "badtimebase", "could not interpret time_base"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			video := filepath.Join(dir, "clip.mp4")
			if err := os.WriteFile(video, []byte("dummy input; the fake ffprobe decides the outcome"), 0o644); err != nil {
				t.Fatal(err)
			}

			t.Setenv("FAKE_FFPROBE", c.role)
			shim := fakeToolShim(t, "ffprobe")
			var out, errBuf bytes.Buffer
			code := runWithPath(t, []string{video}, shim, &out, &errBuf)
			if code == 0 {
				t.Fatalf("expected nonzero exit with a broken fake ffprobe\nstderr:\n%s", errBuf.String())
			}
			if !strings.Contains(errBuf.String(), c.wantErr) {
				t.Errorf("stderr should contain %q, got:\n%s", c.wantErr, errBuf.String())
			}
			if out.Len() != 0 {
				t.Errorf("stdout must stay empty (chapter data only), got:\n%s", out.String())
			}
		})
	}
}

// TestGetchFakeFFprobePositiveControl proves the fake mechanism itself can
// succeed: with the "ok" role the extract flow drains real ffprobe output and
// prints the chapters verbatim, closing the loop on the fake harness.
func TestGetchFakeFFprobePositiveControl(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(video, []byte("dummy input; the fake ffprobe decides the outcome"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_FFPROBE", "ok")
	shim := fakeToolShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{video}, shim, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0 with a healthy fake ffprobe, got %d\nstderr:\n%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Intro") || !strings.Contains(out.String(), "End") {
		t.Errorf("expected the fake chapters on stdout, got:\n%s", out.String())
	}
}

// TestGetchCheckNonExecutableFFprobeRejected proves a non-executable file named
// "ffprobe" on PATH is treated as absent, and --check reports it as missing.
func TestGetchCheckNonExecutableFFprobeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable bits are not enforced on Windows")
	}
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, "ffprobe"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := runWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("getch --check with a non-executable ffprobe must exit nonzero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffprobe") {
		t.Errorf("expected an ffprobe scoped hint on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.String(), "System is ready.") {
		t.Errorf("getch must not report ready with a non-executable ffprobe, got:\n%s", out.String())
	}
}

// TestGetchInvalidPayloads proves extraction rejects media that is empty, pure
// garbage, or a truncated real file — ffprobe cannot read chapters from any of
// them — and keeps stdout empty.
func TestGetchInvalidPayloads(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	realFixture := makeTestContainer(t, dir, "mp4", "5")
	full, err := os.ReadFile(realFixture)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, fileName string
		payload        []byte
	}{
		{"zero-byte", "empty.mp4", nil},
		{"garbage", "garbage.mp4", []byte("this is not a real media file at all")},
		{"truncated", "truncated.mp4", full[:4096]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			video := filepath.Join(dir, c.fileName)
			if err := os.WriteFile(video, c.payload, 0o644); err != nil {
				t.Fatal(err)
			}

			out, errStr, code := runExtractForTest(video, "", false)
			if code == 0 {
				t.Fatalf("extraction of %s must fail\nstdout:\n%s", c.fileName, out)
			}
			if !strings.Contains(errStr, "could not read chapters") {
				t.Errorf("expected a chapter-probe diagnostic, got:\n%s", errStr)
			}
			if out != "" {
				t.Errorf("stdout must stay empty for failed extraction, got:\n%s", out)
			}
		})
	}
}

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
	"time"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 3: failure-injection coverage.
//
// The PATH-shim machinery re-executes the real CLI (via the ADDCH_TEST_BINARY
// hook) with a PATH that swaps in FAKE tools. A fake tool is the test binary
// itself, symlinked into the shim directory under the tool's name; TestMain
// dispatches it to a role handler selected by the FAKE_FFMPEG / FAKE_FFPROBE
// env vars before the CLI code is ever reached, so fakes never recurse into the
// real CLI. Every fake answers `-version` (to pass the dependency gate) and
// misbehaves only on the real probes/remux it is supposed to break.

// exeExt is the platform executable suffix used for PATH-shim entries (".exe"
// on Windows, "" elsewhere), so tool lookup and launch resolve on every OS.
var exeExt = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// maybeRunFakeTool lets the test binary masquerade as a fake ffmpeg/ffprobe
// when it is re-executed from a PATH shim under the matching name. It is called
// from TestMain ahead of the CLI re-exec hook.
func maybeRunFakeTool() bool {
	base := strings.ToLower(filepath.Base(os.Args[0]))
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
	}
	switch base {
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

// fakeFFmpeg plays an ffmpeg on the shim PATH. The dependency gate runs
// `ffmpeg -version`, so that invocation (and only that one) always reports a
// valid version. Every other invocation misbehaves according to role.
func fakeFFmpeg(role string) {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "-version" {
		fmt.Println("ffmpeg version 5.1.3-testsuite")
		os.Exit(0)
	}
	switch role {
	case "fail":
		fmt.Fprintln(os.Stderr, "simulated remux failure")
		os.Exit(1)
	case "garbage":
		out := ""
		if len(args) > 0 {
			out = args[len(args)-1] // the remux output path is the last argument
		}
		_ = os.WriteFile(out, []byte("garbage-bytes-that-are-not-media"), 0o644)
		os.Exit(0)
	case "hang":
		// A long blocking sleep (not an empty select, which the Go runtime's
		// deadlock detector would abort) makes this ffmpeg genuinely unable to
		// finish on its own; the CLI's Interrupt kills it with SIGKILL.
		time.Sleep(72 * time.Hour)
		os.Exit(0)
	default:
		fmt.Fprintln(os.Stderr, "unknown FAKE_FFMPEG role "+role)
		os.Exit(2)
	}
}

// fakeFFprobe plays an ffprobe on the shim PATH. As with fakeFFmpeg, only the
// `-version` gate call succeeds; the probing invocations behave per role.
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
	case "noduration":
		fmt.Print(`{"format":{}}`)
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
// the test binary itself (which dispatches via maybeRunFakeTool) and returns
// the directory. It deliberately contains no real tool aliases: real tools are
// resolved from the normal environment PATH via runAddchFakeTool, so the fake
// alias can never shadow or be shadowed by a real binary.
func fakeToolShim(t *testing.T, fakeTool string) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary for fake %s: %v", fakeTool, err)
	}
	symlinkForTest(t, exe, filepath.Join(dir, fakeTool+exeExt))
	return dir
}

// TestAddchFakeFFprobeRejections proves the duration-probe pipeline surfaces
// every class of broken ffprobe output: nonzero exit, non-JSON garbage, and
// well-formed JSON that carries no duration. In every case the CLI exits
// nonzero, names the input, and leaves no output behind.
func TestAddchFakeFFprobeRejections(t *testing.T) {
	requireTools(t)

	cases := []struct {
		name, role, wantErr string
	}{
		{"exit-code", "fail", "ffprobe failed: simulated probe failure"},
		{"garbage-output", "garbage", "could not parse ffprobe output"},
		{"missing-duration", "noduration", "could not determine the duration"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			video := makeTestVideo(t, dir, "10")
			chaptersFile := filepath.Join(dir, "chapters.txt")
			os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

			t.Setenv("FAKE_FFPROBE", c.role)
			shim := fakeToolShim(t, "ffprobe")
			var out, errBuf bytes.Buffer
			code := runAddchFakeTool(t, []string{chaptersFile, video}, shim, &out, &errBuf)
			if code == 0 {
				t.Fatalf("expected nonzero exit with a broken fake ffprobe\nstdout:\n%s", out.String())
			}
			if !strings.Contains(errBuf.String(), c.wantErr) {
				t.Errorf("stderr should contain %q, got:\n%s", c.wantErr, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), video) {
				t.Errorf("stderr should identify the input video, got:\n%s", errBuf.String())
			}
			if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
				t.Error("no output may exist after a failed probe")
			}
		})
	}
}

// TestAddchFakeFFmpegRemuxFailure proves a failing FFmpeg (nonzero exit with a
// diagnostic) surfaces through the enriched error, cleans the partial output
// and the temp metadata, and leaves no artifacts.
func TestAddchFakeFFmpegRemuxFailure(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())
	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg")

	tempBefore := countTempMetadata()
	var out, errBuf bytes.Buffer
	code := runAddchFakeTool(t, []string{chaptersFile, video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected nonzero exit with a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "FFmpeg failed to remux the video: simulated remux failure") {
		t.Errorf("the FFmpeg diagnostic must reach the caller, got:\n%s", errBuf.String())
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("no output may exist after a failed remux")
	}
	if countTempMetadata() != tempBefore {
		t.Error("temporary metadata file was left behind after the failed remux")
	}
}

// TestAddchFakeFFmpegGarbageOutputFailsVerification proves the verification
// step is not blind: an FFmpeg that writes garbage to the output and exits 0
// must be caught, the junk output removed, and the temp metadata cleaned.
func TestAddchFakeFFmpegGarbageOutputFailsVerification(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644)

	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())
	t.Setenv("FAKE_FFMPEG", "garbage")
	shim := fakeToolShim(t, "ffmpeg")

	tempBefore := countTempMetadata()
	var out, errBuf bytes.Buffer
	code := runAddchFakeTool(t, []string{chaptersFile, video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected verification to reject the lying FFmpeg's output\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "verification failed") {
		t.Errorf("expected a verification failure, got:\n%s", errBuf.String())
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err == nil {
		t.Error("junk output must be removed after a verification failure")
	}
	if countTempMetadata() != tempBefore {
		t.Error("temporary metadata file was left behind after the verification failure")
	}
}

// TestAddchDirectoryNamedFFmpegRejected proves a directory that merely carries
// the name "ffmpeg" on PATH is not treated as the tool: exec.LookPath requires
// a regular executable, so addch --check must fail and hint at the missing
// ffmpeg instead of blundering into the directory.
func TestAddchDirectoryNamedFFmpegRejected(t *testing.T) {
	requireTools(t)

	shim := t.TempDir()
	if err := os.Mkdir(filepath.Join(shim, "ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatalf("could not locate ffprobe for PATH shim: %v", err)
	}
	symlinkForTest(t, real, filepath.Join(shim, "ffprobe"))

	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("addch --check must fail when ffmpeg is only a directory\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffmpeg") {
		t.Errorf("expected an ffmpeg requirement hint on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.String(), "System is ready.") {
		t.Errorf("addch must not report ready with a directory named ffmpeg\n%s", out.String())
	}
}

// TestAddchNonExecutableFFprobeRejected proves a non-executable file named
// "ffprobe" on PATH is treated as absent, and --check reports ffprobe missing.
func TestAddchNonExecutableFFprobeRejected(t *testing.T) {
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable bits are not enforced on Windows")
	}

	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, "ffprobe"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("could not locate ffmpeg for PATH shim: %v", err)
	}
	symlinkForTest(t, real, filepath.Join(shim, "ffmpeg"))

	var out, errBuf bytes.Buffer
	code := runAddchWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("addch --check must fail when ffprobe is not executable\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffprobe") || !strings.Contains(errBuf.String(), "not found") {
		t.Errorf("expected an ffprobe-not-found hint, got:\n%s", errBuf.String())
	}
}

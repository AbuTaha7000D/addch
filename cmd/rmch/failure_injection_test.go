package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/fsutil"
)

// Phase 8 item 3: failure-injection coverage for rmch, using the same
// PATH-shim fake-tool machinery as the addch suite. A fake tool is the test
// binary itself, symlinked into a shim directory under the tool's name;
// TestMain dispatches it to a role handler selected by FAKE_FFMPEG /
// FAKE_FFPROBE before the CLI code is ever reached. The fakes answer only the
// dependency-gate `-version` probe honestly and misbehave on real work.

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

// fakeFFmpeg plays an ffmpeg on the shim PATH: only `-version` (the dependency
// gate) succeeds, and every real strip invocation misbehaves per role.
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
			out = args[len(args)-1] // the strip output path is the last argument
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

// fakeFFprobe plays an ffprobe on the shim PATH with the same honest-only-on-
// -version contract.
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
// the test binary itself (which dispatches via maybeRunFakeTool) plus the
// requested real tools, and returns the directory.
func fakeToolShim(t *testing.T, fakeTool string, realTools ...string) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary for fake %s: %v", fakeTool, err)
	}
	rmchSymlinkForTest(t, exe, filepath.Join(dir, fakeTool))
	for _, tool := range realTools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("could not locate real %s for PATH shim: %v", tool, err)
		}
		rmchSymlinkForTest(t, real, filepath.Join(dir, tool))
	}
	return dir
}

// TestRmchFakeFFprobeRejections proves the removal front door refuses media a
// broken ffprobe cannot describe: nonzero exit, failing input named, and no
// strip output. rmch probes the duration before it ever reaches the ffmpeg
// stage, so every broken probe role must gate the work before it starts.
func TestRmchFakeFFprobeRejections(t *testing.T) {
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
			video := filepath.Join(dir, "clip.mp4")
			if err := os.WriteFile(video, []byte("dummy input; the fake ffprobe decides the outcome"), 0o644); err != nil {
				t.Fatal(err)
			}

			t.Setenv("FAKE_FFPROBE", c.role)
			shim := fakeToolShim(t, "ffprobe", "ffmpeg")
			var out, errBuf bytes.Buffer
			code := runRmchWithPath(t, []string{video}, shim, &out, &errBuf)
			if code == 0 {
				t.Fatalf("expected nonzero exit with a broken fake ffprobe\nstdout:\n%s", out.String())
			}
			if !strings.Contains(errBuf.String(), c.wantErr) {
				t.Errorf("stderr should contain %q, got:\n%s", c.wantErr, errBuf.String())
			}
			if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(video)); err == nil {
				t.Error("no strip output may exist after a failed probe")
			}
		})
	}
}

// TestRmchFakeFFmpegStripFailure proves a failing FFmpeg during a strip
// surfaces through the enriched error and leaves no output artifact.
func TestRmchFakeFFmpegStripFailure(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	t.Setenv("FAKE_FFMPEG", "fail")
	shim := fakeToolShim(t, "ffmpeg", "ffprobe")

	var out, errBuf bytes.Buffer
	code := runRmchWithPath(t, []string{video}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected nonzero exit with a failing fake ffmpeg\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "FFmpeg failed to remux the video: simulated remux failure") {
		t.Errorf("the FFmpeg diagnostic must reach the caller, got:\n%s", errBuf.String())
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(video)); err == nil {
		t.Error("no output may exist after a failed strip")
	}
}

// TestRmchInvalidPayloads proves the removal front door rejects media that is
// empty, pure garbage, or a truncated real file, and leaves no output behind.
func TestRmchInvalidPayloads(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	realFixture := makeTestVideo(t, dir, "5")
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

			out, errStr, code := runRemovalForTest(video, "", false)
			if code == 0 {
				t.Fatalf("removal of %s must fail\nstdout:\n%s", c.fileName, out)
			}
			if !strings.Contains(errStr, "is it a valid video file") {
				t.Errorf("expected a valid-video diagnostic, got:\n%s", errStr)
			}
			if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(video)); err == nil {
				t.Error("failed removal must not leave an output file behind")
			}
		})
	}
}

// TestRmchBatchCorruptSiblingContinues proves batch mode isolates a broken
// candidate for removal: a corrupt media file fails on its own while a
// healthy chaptered sibling is stripped, and the summary counts both.
func TestRmchBatchCorruptSiblingContinues(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	good := makeChapteredFixture(t, dir, "good.mp4", "mp4", "10")
	bad := filepath.Join(dir, "bad.mp4")
	if err := os.WriteFile(bad, []byte("garbage pretending to be a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	tempBefore := countTempMetadata()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("expected exit 1 (one failed item), got %d:\n%s\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(good)); err != nil {
		t.Errorf("healthy sibling must be stripped: %v", err)
	}
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(bad)); err == nil {
		t.Error("corrupt sibling must not leave an output file behind")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp files leaked: before=%d after=%d", tempBefore, got)
	}
}

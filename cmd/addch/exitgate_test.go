package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

// exePath returns the path of the currently running test binary, used to
// re-execute the real addch CLI via the TestMain hook.
func exePath(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// waitWithTimeout waits for cmd to exit, failing loudly if it does not reap
// within timeout. Returns the wait error (an *exec.ExitError for a non-zero
// exit, nil for exit 0).
func waitWithTimeout(t *testing.T, cmd *exec.Cmd, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("child process did not exit within timeout")
		return nil
	}
}

func TestBatchEndToEndMixedDirectory(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	okMP4 := makeNamedContainer(t, dir, "ok.mp4", "mp4", "8")
	writeSidecar(t, okMP4, batchChapters)
	okMKV := makeNamedContainer(t, dir, "good.mkv", "mkv", "8")
	writeSidecar(t, okMKV, batchChapters)
	badMP4 := makeNamedContainer(t, dir, "broken.mp4", "mp4", "6")
	writeSidecar(t, badMP4, "this is not a valid chapter line\n")

	// A generated-output filename with a sidecar must be excluded (no chaining).
	if err := os.WriteFile(filepath.Join(dir, "video-chapters.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video-chapters.txt"), []byte("00:00:00 A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Media with no sidecar must be excluded (and sidecar-less "dummy" never probed).
	if err := os.WriteFile(filepath.Join(dir, "orphan.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	// Exit semantics: any failed item -> 1.
	if code != 1 {
		t.Fatalf("expected exit 1, got %d:\n%s\n%s", code, errStr, out)
	}
	// Summary counts only discovered candidates (ok.mp4, good.mkv, broken.mp4):
	// excluded files are not part of the totals.
	if !strings.Contains(out, "Total: 3 | Succeeded: 2 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}

	// Successful candidates have verified outputs.
	chs, err := chapters.ParseFile(fsutil.SidecarPath(okMP4))
	if err != nil {
		t.Fatalf("parse ok.mp4 sidecar: %v", err)
	}
	durationMs, err := media.GetVideoDurationMs(okMP4)
	if err != nil {
		t.Fatalf("probe ok.mp4: %v", err)
	}
	for _, v := range []string{fsutil.DefaultOutputPath(okMP4), fsutil.DefaultOutputPath(okMKV)} {
		if _, err := os.Stat(v); err != nil {
			t.Errorf("expected output %s: %v", v, err)
		}
	}
	if err := media.VerifyChapters(fsutil.DefaultOutputPath(okMP4), chs, durationMs); err != nil {
		t.Errorf("ok.mp4 chapters verification failed: %v", err)
	}

	// No chaining output for the excluded generated file.
	if _, err := os.Stat(filepath.Join(dir, "video-chapters-chapters.mp4")); err == nil {
		t.Error("chaining: generated output was processed, producing a nested output")
	}
	// The failed item must leave no output behind.
	if _, err := os.Stat(fsutil.DefaultOutputPath(badMP4)); err == nil {
		t.Error("failed item must not leave an output file behind")
	}
}

func TestBatchRecursiveNested(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	top := makeNamedContainer(t, dir, "top.mp4", "mp4", "8")
	writeSidecar(t, top, batchChapters)

	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	deep := makeNamedContainer(t, nested, "deep.mkv", "mkv", "8")
	writeSidecar(t, deep, batchChapters)

	// Shallow must not descend into subdirectories.
	out, _, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("shallow run failed, code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("shallow summary wrong:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(nested, "deep-chapters.mkv")); err == nil {
		t.Error("shallow mode must not process nested files")
	}

	// Recursive must descend; the top-level output now exists -> skipped.
	out2, _, code2 := runBatchForTest(dir, true, false)
	if code2 != 0 {
		t.Fatalf("recursive run failed, code %d:\n%s", code2, out2)
	}
	if !strings.Contains(out2, "Total: 2 | Succeeded: 1 | Skipped: 1 | Failed: 0") {
		t.Errorf("recursive summary wrong:\n%s", out2)
	}
	if _, err := os.Stat(filepath.Join(nested, "deep-chapters.mkv")); err != nil {
		t.Errorf("recursive mode did not process nested file: %v", err)
	}
}

// runSignalTest spawns the REAL addch binary (via the TestMain re-exec hook),
// waits for it to finish startup and register its signal handler (handshake on
// its first output line), then interrupts it mid-batch with sig and asserts the
// child reaps itself with wantCode and leaves no temp metadata.
func runSignalTest(t *testing.T, sig syscall.Signal, wantCode int) {
	t.Helper()
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("process signaling semantics differ on Windows")
	}

	// Several candidates keep the batch running past the signal, so the child
	// cannot finish its whole run before the signal lands. The assertions hold
	// regardless of how far processing got: exit code + no temp litter.
	dir := t.TempDir()
	for _, name := range []string{"one.mp4", "two.mp4", "three.mp4", "four.mp4", "five.mp4"} {
		v := makeNamedContainer(t, dir, name, "mp4", "10")
		writeSidecar(t, v, batchChapters)
	}

	cmd := exec.Command(exePath(t), "--recursive", dir)
	cmd.Env = append(os.Environ(), "ADDCH_TEST_BINARY=1")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = pw

	tempBefore := countTempMetadata()
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		t.Fatal(err)
	}
	pw.Close() // child holds the write end; the parent only reads.

	// Handshake: read the child's stdout live; signal only after it has clearly
	// started (its first progress line is printed before discovery + handler
	// registration, so this proves the runaway-init window has passed).
	var stdout []string
	depsSeen := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			stdout = append(stdout, line)
			if strings.Contains(line, "Dependencies found") {
				select {
				case <-depsSeen:
				default:
					close(depsSeen)
				}
			}
		}
	}()

	select {
	case <-depsSeen:
		// Child has finished init and registered its handler; it is about to
		// process the batch.
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		pr.Close()
		t.Fatalf("child never started:\nstderr:\n%s", errBuf.String())
	}

	// Let the first remux begin (the handler is registered microseconds after
	// the deps print; the full batch of five items takes far longer than this),
	// then interrupt mid-batch.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(sig); err != nil {
		_ = cmd.Process.Kill()
		pr.Close()
		<-scanDone
		t.Fatalf("send signal: %v", err)
	}

	err = waitWithTimeout(t, cmd, 15*time.Second)
	pr.Close()
	<-scanDone
	if err == nil {
		t.Fatalf("child exited 0 (batch completed before signal); expected exit %d.\nstdout:\n%s\nstderr:\n%s",
			wantCode, strings.Join(stdout, "\n"), errBuf.String())
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected wait error: %v", err)
	}
	if code := ee.ExitCode(); code != wantCode {
		t.Errorf("exit code = %d, want %d.\nstdout:\n%s\nstderr:\n%s",
			code, wantCode, strings.Join(stdout, "\n"), errBuf.String())
	}

	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked after interrupt: before=%d after=%d", tempBefore, got)
	}
	t.Logf("child exit %d;\nstdout:\n%s", wantCode, strings.Join(stdout, "\n"))
}

func TestBatchSIGINTExit130(t *testing.T) {
	runSignalTest(t, syscall.SIGINT, 130)
}

func TestBatchSIGTERMExit143(t *testing.T) {
	runSignalTest(t, syscall.SIGTERM, 143)
}

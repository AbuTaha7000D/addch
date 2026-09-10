package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/media"
)

// exePath returns the path of the currently running test binary, used to
// re-execute the real rmch CLI via the TestMain hook.
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

// partialNoChaptersOutputs returns any *-nochapters* files under dir that are
// not valid, complete chapter-stripped outputs. A strip interrupted mid-write
// leaves a truncated/broken file, which ffprobe cannot fully read, so such a
// survivor is a genuine leak. A strip that finished before the signal lands
// leaves a complete zero-chapter output that ffprobe reads cleanly; completed
// work surviving an interrupt is legitimate (same contract as getch's
// exit-gate batch, where completed sidecars are not leaks).
func partialNoChaptersOutputs(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(filepath.Base(path), "-nochapters") {
			return nil
		}
		if err := media.VerifyNoChapters(path); err != nil {
			found = append(found, fmt.Sprintf("%s (%v)", path, err))
		}
		return nil
	})
	return found
}

// runSignalTest spawns the REAL rmch binary (via the TestMain re-exec hook),
// waits for it to finish startup (handshake on its first output line), then
// interrupts it mid-batch with sig and asserts the child reaps itself with
// wantCode, leaves no incomplete -nochapters output and no temp litter.
func runSignalTest(t *testing.T, sig syscall.Signal, wantCode int) {
	t.Helper()
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("process signaling semantics differ on Windows")
	}

	// Several chaptered candidates keep the batch running past the signal, so
	// the child cannot finish its whole run before the signal lands and at
	// least some items are cancelled. Items whose strip completes before the
	// signal (a fast machine can finish one in the 250ms window) legitimately
	// keep their output, so only incomplete survivors are leaks.
	dir := t.TempDir()
	for _, name := range []string{"one.mp4", "two.mp4", "three.mp4", "four.mp4", "five.mp4"} {
		makeChapteredFixture(t, dir, name, "mp4", "10")
	}

	cmd := exec.Command(exePath(t), "--recursive", dir)
	cmd.Env = append(os.Environ(), "RMCH_TEST_BINARY=1")
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

	// Let the first strip begin (the handler is registered microseconds after
	// the deps print; the full batch of five items takes far longer than this),
	// then interrupt mid-batch.
	time.Sleep(250 * time.Millisecond)
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

	if leaks := partialNoChaptersOutputs(t, dir); len(leaks) != 0 {
		t.Errorf("incomplete -nochapters outputs left after interrupt: %v", leaks)
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

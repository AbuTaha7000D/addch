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
)

// exePath returns the path of the currently running test binary, used to
// re-execute the real getch CLI via the TestMain hook.
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

// sidecarTempLitter walks dir for leftover WriteFileAtomic temp files (normally
// renamed or removed by the time the command finishes).
func sidecarTempLitter(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(filepath.Base(path), ".addch-sidecar-") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// emptySidecars walks dir for any zero-byte .txt sidecars (getch never writes
// one; a survivor is a partial-artifact leak).
func emptySidecars(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.Size() == 0 {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// runSignalTest spawns the REAL getch binary (via the TestMain re-exec hook),
// waits for it to finish startup — handshaking on its stderr "Dependencies
// found" line, since getch keeps stdout free of anything but chapter data —
// then interrupts it mid-batch with sig and asserts the child reaps itself with
// wantCode, leaves no partial sidecar or temp litter, and writes nothing to
// stdout.
//
// The batch cannot run to completion before the signal: extraction is fast, so
// the fixture is a single embedded MP4 copied to 30 files; the full batch still
// takes well over a second while the signal lands ~150ms after the handshake,
// so the remainder is deterministically recorded as cancelled.
func runSignalTest(t *testing.T, sig syscall.Signal, wantCode int) {
	t.Helper()
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("process signaling semantics differ on Windows")
	}

	dir := t.TempDir()
	first := makeChapteredFixture(t, dir, "ch-01.mp4", "mp4", "10")
	bytes_, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 30; i++ {
		name := fmt.Sprintf("ch-%02d.mp4", i)
		if err := os.WriteFile(filepath.Join(dir, name), bytes_, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(exePath(t), "--recursive", dir)
	cmd.Env = append(os.Environ(), "GETCH_TEST_BINARY=1")
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = pw

	tempBefore := countTempMetadata()
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		t.Fatal(err)
	}
	pw.Close() // child holds the write end; the parent only reads.

	// Handshake on stderr: getch prints "Dependencies found" before discovery
	// and handler registration, so seeing it proves the runaway-init window has
	// passed. stdout is captured separately and asserted empty at the end.
	var stderrLines []string
	depsSeen := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			stderrLines = append(stderrLines, line)
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
		t.Fatalf("child never started:\nstderr:\n%s", strings.Join(stderrLines, "\n"))
	}

	// Let the first items run (extraction is sub-second per item but the full
	// batch of thirty takes far longer), then interrupt mid-batch.
	time.Sleep(150 * time.Millisecond)
	if err := cmd.Process.Signal(sig); err != nil {
		_ = cmd.Process.Kill()
		pr.Close()
		<-scanDone
		t.Fatalf("send signal: %v", err)
	}

	err = waitWithTimeout(t, cmd, 15*time.Second)
	pr.Close()
	<-scanDone
	report := strings.Join(stderrLines, "\n")
	if err == nil {
		t.Fatalf("child exited 0 (batch completed before signal); expected exit %d.\nstdout:\n%s\nstderr:\n%s",
			wantCode, outBuf.String(), report)
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected wait error: %v", err)
	}
	if code := ee.ExitCode(); code != wantCode {
		t.Errorf("exit code = %d, want %d.\nstdout:\n%s\nstderr:\n%s",
			code, wantCode, outBuf.String(), report)
	}
	if out := outBuf.String(); out != "" {
		t.Errorf("batch stdout must stay empty even under signal, got %q", out)
	}
	if !strings.Contains(report, "(cancelled)") {
		t.Errorf("expected cancelled items in the report:\n%s", report)
	}
	if leaks := sidecarTempLitter(t, dir); len(leaks) != 0 {
		t.Errorf("partial sidecar temp files left after interrupt: %v", leaks)
	}
	if empties := emptySidecars(t, dir); len(empties) != 0 {
		t.Errorf("empty sidecars left after interrupt: %v", empties)
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked after interrupt: before=%d after=%d", tempBefore, got)
	}
	t.Logf("child exit %d;\nstdout:\n%s", wantCode, outBuf.String())
}

func TestBatchSIGINTExit130(t *testing.T) {
	runSignalTest(t, syscall.SIGINT, 130)
}

func TestBatchSIGTERMExit143(t *testing.T) {
	runSignalTest(t, syscall.SIGTERM, 143)
}

package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abutaha/addch/internal/fsutil"
)

// runRmchHangTest proves the ONLY exit path available while a strip can never
// finish on its own — the exitgate signal handler. A fake ffmpeg that hangs
// forever guarantees the child cannot complete (Done() can never fire), so an
// interrupting signal must take the handler path deterministically: the handler
// kills and reaps the wedged ffmpeg, removes the partial output, and exits with
// the expected code.
func runRmchHangTest(t *testing.T, sig syscall.Signal, wantCode int) {
	t.Helper()
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("process signaling semantics differ on Windows")
	}

	dir := t.TempDir()
	video := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	t.Setenv("FAKE_FFMPEG", "hang")
	shim := fakeToolShim(t, "ffmpeg")

	cmd := exec.Command(exePath(t), video)
	env := []string{"RMCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "Path=") || strings.HasPrefix(kv, "RMCH_TEST_BINARY=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = env
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		t.Fatal(err)
	}
	pw.Close()

	// Handshake on "→ Removing chapters...": the strip (and its signal-handler
	// registration) happens strictly after that progress line, so once it is seen
	// the pre-Notify window has closed and a signal definitely lands on the
	// handler.
	var stdout []string
	removalSeen := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			stdout = append(stdout, line)
			if strings.Contains(line, "Removing chapters") {
				select {
				case <-removalSeen:
				default:
					close(removalSeen)
				}
			}
		}
	}()

	select {
	case <-removalSeen:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		pr.Close()
		t.Fatalf("child never reached the strip:\nstderr:\n%s", errBuf.String())
	}

	// Allow the hang to be reached (the wedge lasts forever, so any settling
	// time keeps the child mid-strip), then interrupt the child alone. Its
	// handler is responsible for killing and reaping the wedged ffmpeg itself.
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(sig); err != nil {
		_ = cmd.Process.Kill()
		pr.Close()
		<-scanDone
		t.Fatalf("signal child: %v", err)
	}

	err = waitWithTimeout(t, cmd, 15*time.Second)
	pr.Close()
	<-scanDone
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected a signal exit; got wait error %v.\nstdout:\n%s\nstderr:\n%s",
			err, strings.Join(stdout, "\n"), errBuf.String())
	}
	if code := ee.ExitCode(); code != wantCode {
		t.Errorf("exit code = %d, want %d.\nstdout:\n%s\nstderr:\n%s",
			code, wantCode, strings.Join(stdout, "\n"), errBuf.String())
	}
	if _, statErr := os.Stat(fsutil.DefaultNoChaptersOutputPath(video)); statErr == nil {
		t.Error("interrupted strip must not leave an output file behind")
	}
	t.Logf("child exit %d;\nstdout:\n%s", wantCode, strings.Join(stdout, "\n"))
}

// TestRmchStripHangSIGTERM proves an in-flight strip that can never finish is
// reaped with the SIGTERM convention code (143).
func TestRmchStripHangSIGTERM(t *testing.T) {
	runRmchHangTest(t, syscall.SIGTERM, 143)
}

// TestRmchStripHangSIGINT proves an in-flight strip that can never finish is
// reaped with the SIGINT convention code (130).
func TestRmchStripHangSIGINT(t *testing.T) {
	runRmchHangTest(t, syscall.SIGINT, 130)
}

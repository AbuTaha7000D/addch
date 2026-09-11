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

	"github.com/abutaha/addch/internal/fsutil"
)

// runHangTest proves the ONLY exit path available while a remux can never
// finish on its own — the exitgate signal handler. A fake ffmpeg that hangs
// forever guarantees the child cannot complete (Done() can never fire), so an
// interrupting signal must take the handler path deterministically: the handler
// kills and reaps the wedged ffmpeg, removes the partial output and temp
// metadata, and exits with the expected code.
func runHangTest(t *testing.T, sig syscall.Signal, wantCode int) {
	t.Helper()
	requireTools(t)
	if runtime.GOOS == "windows" {
		t.Skip("process signaling semantics differ on Windows")
	}

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 A\n00:00:05 B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_FFMPEG", "hang")
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())
	shim := fakeToolShim(t, "ffmpeg")

	cmd := exec.Command(exePath(t), chaptersFile, video)
	env := []string{"ADDCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "ADDCH_TEST_BINARY=") {
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

	tempBefore := countTempMetadata()
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		t.Fatal(err)
	}
	pw.Close()

	// Handshake on "✓ Video duration checked": the remux (and its signal
	// handler registration) happens strictly after that probe, so once this line
	// is seen the pre-Notify window has closed and a signal definitely lands on
	// the handler.
	var stdout []string
	probeSeen := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			stdout = append(stdout, line)
			if strings.Contains(line, "Video duration checked") {
				select {
				case <-probeSeen:
				default:
					close(probeSeen)
				}
			}
		}
	}()

	select {
	case <-probeSeen:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		pr.Close()
		t.Fatalf("child never probed the duration:\nstderr:\n%s", errBuf.String())
	}

	// Allow the hang to be reached (the wedge lasts forever, so any settling
	// time keeps the child mid-remux), then interrupt the child alone. Its
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
	if _, statErr := os.Stat(fsutil.DefaultOutputPath(video)); statErr == nil {
		t.Error("interrupted embed must not leave an output file behind")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked after interrupt: before=%d after=%d", tempBefore, got)
	}
	t.Logf("child exit %d;\nstdout:\n%s", wantCode, strings.Join(stdout, "\n"))
}

// TestAddchEmbedHangSIGINT proves an in-flight embed that can never finish is
// reaped with the SIGINT convention code (130).
func TestAddchEmbedHangSIGINT(t *testing.T) {
	runHangTest(t, syscall.SIGINT, 130)
}

// TestAddchEmbedHangSIGTERM proves an in-flight embed that can never finish is
// reaped with the SIGTERM convention code (143).
func TestAddchEmbedHangSIGTERM(t *testing.T) {
	runHangTest(t, syscall.SIGTERM, 143)
}

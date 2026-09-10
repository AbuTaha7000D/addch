package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

// Phase 8.5.1 regression: a chapter-write failure in stdout mode must be
// reported. runExtract writes progress to stderr and chapters to stdout, so a
// stdout writer that fails deterministically exercises exactly the
// chapters.Write error path without relying on OS-specific broken-pipe
// behavior. The source file must stay untouched, the exit code must be nonzero,
// stdout must carry nothing, and exactly one Error: diagnostic must reach
// stderr.

// errWriter is an io.Writer that always fails with the given error. It fails
// the write before committing any bytes, which is why stdout stays empty.
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// countingWriter records what it is asked to write while committing it, so the
// healthy control can assert the exact canonical bytes still flow on success.
type countingWriter struct {
	buf bytes.Buffer
}

func (w *countingWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }

func TestGetchStdoutWriteFailureReported(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	inputs := snapshotFiles(t, input)

	pa := &parsedArgs{video: input, output: ""}
	var errBuf bytes.Buffer
	fail := errWriter{err: errors.New("simulated write failure")}
	code := runExtract(pa, fail, &errBuf)

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (a broken stdout writer must not report success)", code)
	}
	if !strings.Contains(errBuf.String(), "Error: simulated write failure") {
		t.Errorf("stderr must carry one Error: diagnostic naming the write failure, got:\n%s", errBuf.String())
	}
	if n := strings.Count(errBuf.String(), "Error:"); n != 1 {
		t.Errorf("expected exactly one Error: diagnostic, got %d:\n%s", n, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "✓ Chapters") {
		t.Errorf("no success confirmation may appear when stdout write failed:\n%s", errBuf.String())
	}
	assertFilesUntouched(t, inputs)
}

func TestGetchStdoutWriteFailureHealthyControl(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")

	pa := &parsedArgs{video: input, output: ""}
	var out countingWriter
	var errBuf bytes.Buffer
	if code := runExtract(pa, &out, &errBuf); code != 0 {
		t.Fatalf("healthy stdout writer must exit 0, got %d:\n%s", code, errBuf.String())
	}
	if got, want := out.buf.String(), chapters.Format(seedChapters); got != want {
		t.Errorf("success stdout = %q, want canonical %q (format must be unchanged)", got, want)
	}
	if !strings.Contains(errBuf.String(), "→ Extracting chapters...") {
		t.Errorf("expected progress on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.buf.String(), "Dependencies found") {
		t.Errorf("no progress may leak into stdout, got:\n%s", out.buf.String())
	}
}

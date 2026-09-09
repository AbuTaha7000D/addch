package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
)

// runBatchForTest drives runBatch with a minimal parsedArgs, returning the
// captured stdout (which must stay empty in batch mode), stderr, and exit code.
func runBatchForTest(dir string, recursive, overwrite bool) (string, string, int) {
	pa := &parsedArgs{dir: !recursive, recursive: recursive, dirPath: dir, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runBatch(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

// reportOrder returns the path of each reported item, in report order.
func reportOrder(report string) []string {
	var order []string
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "  [") {
			idx := strings.Index(line, "] ")
			if idx < 0 {
				continue
			}
			path := strings.TrimSpace(line[idx+2:])
			if sp := strings.Index(path, " ("); sp >= 0 {
				path = path[:sp]
			}
			order = append(order, path)
		}
	}
	return order
}

// requireStdoutEmpty asserts the batch contract: stdout carries no data.
func requireStdoutEmpty(t *testing.T, out string) {
	t.Helper()
	if out != "" {
		t.Errorf("batch mode must keep stdout empty, got %q", out)
	}
}

func TestRunBatchMixedDirectory(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	// Chaptered files: succeed.
	a := makeChapteredFixture(t, dir, "a.mp4", "mp4", "8")
	b := makeChapteredFixture(t, dir, "b.mkv", "mkv", "8")
	// No-chapter media: skipped with reason "no chapters".
	_ = makeNamedContainer(t, dir, "c.mp4", "mp4", "8")
	// Chaptered file whose sidecar already exists: skipped "output exists".
	d := makeChapteredFixture(t, dir, "d.mkv", "mkv", "8")
	existing := fsutil.SidecarPath(d)
	dummy := []byte("pre-existing sidecar")
	if err := os.WriteFile(existing, dummy, 0o644); err != nil {
		t.Fatal(err)
	}
	// A generated-output filename must be excluded, never a candidate.
	if err := os.WriteFile(filepath.Join(dir, "gen-chapters.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstderr:\n%s", code, errStr, out)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 4 | Succeeded: 2 | Skipped: 2 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", errStr)
	}
	// Deterministic (lexicographic) ordering.
	want := []string{a, b, filepath.Join(dir, "c.mp4"), d}
	got := reportOrder(errStr)
	if len(got) != len(want) {
		t.Fatalf("report order length = %d, want %d:\n%s", len(got), len(want), errStr)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("report[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	// Reasons are reported.
	if !strings.Contains(errStr, "(no chapters)") {
		t.Errorf("expected 'no chapters' reason in report:\n%s", errStr)
	}
	if !strings.Contains(errStr, "(output exists (use --overwrite to replace))") {
		t.Errorf("expected 'output exists' reason in report:\n%s", errStr)
	}

	// Successes produced canonical sidecars.
	if data, err := os.ReadFile(fsutil.SidecarPath(a)); err != nil {
		t.Fatal(err)
	} else if string(data) != chapters.Format(seedChapters) {
		t.Errorf("a sidecar = %q, want %q", data, chapters.Format(seedChapters))
	}
	if data, err := os.ReadFile(fsutil.SidecarPath(b)); err != nil {
		t.Fatal(err)
	} else if string(data) != chapters.Format(seedChapters) {
		t.Errorf("b sidecar = %q, want %q", data, chapters.Format(seedChapters))
	}
	// No-chapters item produced no empty sidecar.
	if _, err := os.Stat(fsutil.SidecarPath(filepath.Join(dir, "c.mp4"))); err == nil {
		t.Error("no-chapters item must not produce a sidecar")
	}
	// Existing sidecar left byte-identical (skip, not overwrite).
	if got, err := os.ReadFile(existing); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got, dummy) {
		t.Error("existing sidecar was rewritten despite no --overwrite")
	}
	// Generated output excluded and never reprocessed.
	if strings.Contains(errStr, "gen-chapters") {
		t.Errorf("generated output must be absent from the report:\n%s", errStr)
	}
	if countSidecarTemp(dir) != 0 {
		t.Errorf("sidecar temp files leaked after batch")
	}
}

func TestRunBatchShallowVsRecursive(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	top := makeChapteredFixture(t, dir, "top.mp4", "mp4", "8")
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	deep := makeChapteredFixture(t, nested, "deep.mkv", "mkv", "8")

	// Shallow must not descend into subdirectories.
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("shallow run failed, code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("shallow summary wrong:\n%s", errStr)
	}
	if _, err := os.Stat(fsutil.SidecarPath(deep)); err == nil {
		t.Error("shallow mode must not process nested files")
	}

	// Recursive must descend; the top-level sidecar now exists -> skipped.
	out2, errStr2, code2 := runBatchForTest(dir, true, false)
	if code2 != 0 {
		t.Fatalf("recursive run failed, code %d:\n%s", code2, errStr2)
	}
	requireStdoutEmpty(t, out2)
	if !strings.Contains(errStr2, "Total: 2 | Succeeded: 1 | Skipped: 1 | Failed: 0") {
		t.Errorf("recursive summary wrong:\n%s", errStr2)
	}
	if data, err := os.ReadFile(fsutil.SidecarPath(deep)); err != nil {
		t.Errorf("recursive mode did not process nested file: %v", err)
	} else if string(data) != chapters.Format(seedChapters) {
		t.Errorf("deep sidecar = %q, want %q", data, chapters.Format(seedChapters))
	}
	if _, err := os.Stat(fsutil.SidecarPath(top)); err != nil {
		t.Errorf("top sidecar missing: %v", err)
	}
}

func TestRunBatchOverwriteRegenerates(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	m := makeChapteredFixture(t, dir, "m.mp4", "mp4", "8")
	sidecar := fsutil.SidecarPath(m)
	dummy := []byte("stale")
	if err := os.WriteFile(sidecar, dummy, 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --overwrite the existing sidecar is skipped untouched.
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("run without overwrite failed, code %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0") {
		t.Errorf("expected the stale sidecar to be skipped:\n%s", errStr)
	}
	if got, _ := os.ReadFile(sidecar); !bytes.Equal(got, dummy) {
		t.Fatal("stale sidecar must not be replaced without --overwrite")
	}

	// With --overwrite the sidecar is regenerated.
	out2, errStr2, code2 := runBatchForTest(dir, false, true)
	if code2 != 0 {
		t.Fatalf("run with overwrite failed, code %d:\n%s", code2, errStr2)
	}
	requireStdoutEmpty(t, out2)
	if !strings.Contains(errStr2, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("expected success with --overwrite:\n%s", errStr2)
	}
	if got, _ := os.ReadFile(sidecar); string(got) != chapters.Format(seedChapters) {
		t.Errorf("regenerated sidecar = %q, want %q", got, chapters.Format(seedChapters))
	}
}

func TestRunBatchEmptyDirectory(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("expected an empty directory to exit 0, got %d:\n%s", code, errStr)
	}
	requireStdoutEmpty(t, out)
	if !strings.Contains(errStr, "No media files found in") {
		t.Errorf("expected a 'no media files' note on stderr, got %q", errStr)
	}
}

func TestRunBatchRootErrors(t *testing.T) {
	requireTools(t)

	missing := filepath.Join(t.TempDir(), "missing")
	out, errStr, code := runBatchForTest(missing, true, false)
	if code != 1 {
		t.Fatalf("missing recursive root: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errStr)
	}
	if !strings.Contains(errStr, "Error:") {
		t.Errorf("missing recursive root: want an Error line on stderr, got:\n%s", errStr)
	}

	fileAsRoot := filepath.Join(t.TempDir(), "not-a-directory.mp4")
	if err := os.WriteFile(fileAsRoot, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errStr, code = runBatchForTest(fileAsRoot, true, false)
	if code != 1 {
		t.Fatalf("file as recursive root: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errStr)
	}
	if !strings.Contains(errStr, "Error:") {
		t.Errorf("file as recursive root: want an Error line on stderr, got:\n%s", errStr)
	}
}

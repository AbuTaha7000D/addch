package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// runBatchForTest drives runBatch with a minimal parsedArgs and returns the
// captured stdout, stderr, and exit code.
func runBatchForTest(dir string, recursive, overwrite bool) (string, string, int) {
	pa := &parsedArgs{dir: !recursive, recursive: recursive, dirPath: dir, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runBatch(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

// reportOrder returns the path of each reported item, in report order.
func reportOrder(out string) []string {
	var order []string
	for _, line := range strings.Split(out, "\n") {
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

func TestRunBatchMixedDirectory(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	// Chaptered files: succeed.
	a := makeChapteredFixture(t, dir, "a.mp4", "mp4", "8")
	b := makeChapteredFixture(t, dir, "b.mkv", "mkv", "8")
	// No-chapter media: skipped with reason "no chapters to remove".
	_ = makeNamedContainer(t, dir, "c.mp4", "mp4", "8")
	// Chaptered file whose default output already exists: skipped "output exists".
	d := makeChapteredFixture(t, dir, "d.mkv", "mkv", "8")
	existing := fsutil.DefaultNoChaptersOutputPath(d)
	dummy := []byte("pre-existing output")
	if err := os.WriteFile(existing, dummy, 0o644); err != nil {
		t.Fatal(err)
	}
	// A generated-output filename must be excluded, never a candidate.
	if err := os.WriteFile(filepath.Join(dir, "gen-chapters.mp4"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("runBatch failed code %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 4 | Succeeded: 2 | Skipped: 2 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	// Deterministic (lexicographic) ordering.
	want := []string{a, b, filepath.Join(dir, "c.mp4"), d}
	got := reportOrder(out)
	if len(got) != len(want) {
		t.Fatalf("report order length = %d, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("report[%d] = %s, want %s", i, got[i], want[i])
		}
	}

	// Successes produced verified zero-chapter outputs.
	requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(a))
	requireZeroChapters(t, fsutil.DefaultNoChaptersOutputPath(b))
	// No output for the no-chapters item.
	if _, err := os.Stat(fsutil.DefaultNoChaptersOutputPath(filepath.Join(dir, "c.mp4"))); err == nil {
		t.Error("no-chapters item must not produce an output")
	}
	// Existing output left byte-identical (skip, not overwrite).
	if got, err := os.ReadFile(existing); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got, dummy) {
		t.Error("existing output was rewritten despite no --overwrite")
	}
	// Generated output excluded and never reprocessed.
	if strings.Contains(out, "gen-chapters") {
		t.Errorf("generated output must be absent from the report:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen-chapters-nochapters.mp4")); err == nil {
		t.Error("generated output must not be processed")
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
	_ = makeChapteredFixture(t, nested, "deep.mkv", "mkv", "8")

	// Shallow must not descend into subdirectories.
	out, _, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("shallow run failed, code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("shallow summary wrong:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(nested, "deep-nochapters.mkv")); err == nil {
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
	if _, err := os.Stat(filepath.Join(nested, "deep-nochapters.mkv")); err != nil {
		t.Errorf("recursive mode did not process nested file: %v", err)
	} else {
		requireZeroChapters(t, filepath.Join(nested, "deep-nochapters.mkv"))
	}
	wantTop := fsutil.DefaultNoChaptersOutputPath(top)
	if _, err := os.Stat(wantTop); err != nil {
		t.Errorf("top output missing: %v", err)
	}
}

func TestRunBatchOverwriteRegenerates(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	m := makeChapteredFixture(t, dir, "m.mp4", "mp4", "8")
	output := fsutil.DefaultNoChaptersOutputPath(m)
	dummy := []byte("stale")
	if err := os.WriteFile(output, dummy, 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --overwrite the existing output is skipped untouched.
	out, _, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("run without overwrite failed, code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Skipped: 1 | Failed: 0") {
		t.Errorf("expected the stale output to be skipped:\n%s", out)
	}
	if got, _ := os.ReadFile(output); !bytes.Equal(got, dummy) {
		t.Fatal("stale output must not be replaced without --overwrite")
	}

	// With --overwrite the output is regenerated and carries zero chapters.
	out2, _, code2 := runBatchForTest(dir, false, true)
	if code2 != 0 {
		t.Fatalf("run with overwrite failed, code %d:\n%s", code2, out2)
	}
	if !strings.Contains(out2, "Succeeded: 1") {
		t.Errorf("expected success with --overwrite:\n%s", out2)
	}
	requireZeroChapters(t, output)
}

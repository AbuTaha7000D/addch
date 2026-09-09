package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

// TestAddchAdversarialChaptersFileGuards pins the single-file guards around
// the chapters-file argument and the (deliberate) absence of an extension
// guard: a nonexistent file and a directory both fail cleanly with exit 1,
// while any filename extension parses normally when the content is valid.
func TestAddchAdversarialChaptersFileGuards(t *testing.T) {
	requireTools(t)

	t.Run("nonexistent chapters file", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		_, errStr, code := runEmbedForTest(filepath.Join(dir, "missing.txt"), video, "", false)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(errStr, "cannot open chapters file") {
			t.Errorf("unexpected error: %s", errStr)
		}
	})

	t.Run("chapters file is a directory", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		chapDir := filepath.Join(dir, "chapters-as-dir")
		if err := os.Mkdir(chapDir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapDir, video, "", false)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(errStr, "is a directory") {
			t.Errorf("unexpected error: %s", errStr)
		}
	})

	t.Run("no extension guard on chapters file", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		chapFile := filepath.Join(dir, "chapters.md")
		if err := os.WriteFile(chapFile, []byte("00:00:00 Intro\n00:00:05 Final\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, video, "", false)
		if code != 0 {
			t.Fatalf("any extension parses valid content, code %d:\n%s", code, errStr)
		}
		if _, err := os.Stat(fsutil.DefaultOutputPath(video)); err != nil {
			t.Errorf("expected output: %v", err)
		}
	})
}

// TestAddchAdversarialDurationBoundaries exercises the timestamp-duration
// boundary at real-video resolution: a chapter exactly at the probed duration
// and just under it embed successfully, just over it (and a 23-hour timestamp
// on a one-second clip) fail with the canonical M1 message and no output.
func TestAddchAdversarialDurationBoundaries(t *testing.T) {
	requireTools(t)

	t.Run("chapter exactly at duration embeds", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		durationMs := durationMs(t, video)
		chapFile := filepath.Join(dir, "exact.txt")
		content := "00:00:00 Intro\n" + chapters.FormatMilliseconds(durationMs) + " ExactlyAtEnd\n"
		if err := os.WriteFile(chapFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, video, "", false)
		if code != 0 {
			t.Fatalf("expected == duration to embed, code %d:\n%s", code, errStr)
		}
		got := probeChaptersRaw(t, fsutil.DefaultOutputPath(video))
		if len(got) != 2 {
			t.Fatalf("expected 2 chapters, got %d", len(got))
		}
		last := millisecondsOf(t, got[1])
		if diff(last, durationMs) > media.ToleranceMs {
			t.Errorf("last chapter start = %d ms, want duration %d ms", last, durationMs)
		}
	})

	t.Run("chapter just under duration embeds", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		durationMs := durationMs(t, video)
		chapFile := filepath.Join(dir, "under.txt")
		content := "00:00:00 Intro\n" + chapters.FormatMilliseconds(durationMs-1) + " UnderEnd\n"
		if err := os.WriteFile(chapFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, video, "", false)
		if code != 0 {
			t.Fatalf("expected duration-1 to embed, code %d:\n%s", code, errStr)
		}
	})

	t.Run("chapter just over duration rejected", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		durationMs := durationMs(t, video)
		chapFile := filepath.Join(dir, "over.txt")
		overTs := chapters.FormatMilliseconds(durationMs + 1)
		content := "00:00:00 Intro\n" + overTs + " OverEnd\n"
		if err := os.WriteFile(chapFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, video, "", false)
		if code != 1 {
			t.Fatalf("expected duration+1 to be rejected, code %d", code)
		}
		if !strings.Contains(errStr, "exceeds the video duration") || !strings.Contains(errStr, overTs) {
			t.Errorf("unexpected error: %s", errStr)
		}
		if _, err := os.Stat(fsutil.DefaultOutputPath(video)); !os.IsNotExist(err) {
			t.Error("no output may exist after the rejected boundary run")
		}
	})

	t.Run("23-hour timestamp on a one-second clip", func(t *testing.T) {
		dir1 := t.TempDir()
		clip := makeTestVideo(t, dir1, "1")
		chapFile := filepath.Join(dir1, "huge.txt")
		content := "00:00:00 Intro\n23:59:59 WaaayLater\n"
		if err := os.WriteFile(chapFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, clip, "", false)
		if code != 1 {
			t.Fatalf("expected 23:59:59 on a 1s clip to be rejected, code %d", code)
		}
		if !strings.Contains(errStr, "timestamp 23:59:59 exceeds the video duration") {
			t.Errorf("unexpected error: %s", errStr)
		}
	})
}

// TestAddchAdversarialOutputDirectory pins the output-path-is-a-directory
// behavior in both single and batch modes. Without --overwrite the existing
// guard fires; with --overwrite FFmpeg fails and the directory must never be
// removed (regression for the Phase 8 item 2 finding that cleanup deleted an
// empty directory at the output path).
func TestAddchAdversarialOutputDirectory(t *testing.T) {
	requireTools(t)

	t.Run("single: directory at output path refused without overwrite", func(t *testing.T) {
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		chapFile := writeSimpleChapters(t, dir, "chapters.txt")
		asDir := filepath.Join(dir, "output-as-dir")
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runEmbedForTest(chapFile, video, asDir, false)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(errStr, "already exists") {
			t.Errorf("unexpected error: %s", errStr)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("refused output directory must be preserved")
		}
	})

	t.Run("single: directory at output path survives --overwrite", func(t *testing.T) {
		t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())
		dir := t.TempDir()
		video := makeTestVideo(t, dir, "10")
		chapFile := writeSimpleChapters(t, dir, "chapters.txt")
		out := fsutil.DefaultOutputPath(video)
		if err := os.Mkdir(out, 0o755); err != nil {
			t.Fatal(err)
		}
		srcBefore := readBytes(t, video)
		tempBefore := countTempMetadata()
		_, errStr, code := runEmbedForTest(chapFile, video, "", true)
		if code != 1 {
			t.Fatalf("overwrite into a directory must fail, code %d", code)
		}
		if errStr == "" {
			t.Fatal("expected an error message")
		}
		if fi, err := os.Stat(out); err != nil || !fi.IsDir() {
			t.Error("--overwrite run deleted the empty directory at the default output path")
		}
		if !bytes.Equal(readBytes(t, video), srcBefore) {
			t.Error("input was modified")
		}
		if got := countTempMetadata(); got != tempBefore {
			t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
		}
	})

	t.Run("batch: directory at output path skipped", func(t *testing.T) {
		dir := t.TempDir()
		a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
		writeSidecar(t, a, batchChapters)
		asDir := fsutil.DefaultOutputPath(a)
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		out, errStr, code := runBatchForTest(dir, false, false)
		if code != 0 {
			t.Fatalf("expected skip, exit 0, got %d:\n%s", code, errStr)
		}
		if !strings.Contains(out, "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0") {
			t.Errorf("unexpected summary:\n%s", out)
		}
		if strings.Contains(out, "output exists") == false {
			t.Errorf("expected an output-exists reason:\n%s", out)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("batch must not remove a directory at the output path")
		}
	})

	t.Run("batch: directory at output path survives --overwrite failure", func(t *testing.T) {
		dir := t.TempDir()
		a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
		writeSidecar(t, a, batchChapters)
		asDir := fsutil.DefaultOutputPath(a)
		if err := os.Mkdir(asDir, 0o755); err != nil {
			t.Fatal(err)
		}
		out, errStr, code := runBatchForTest(dir, false, true)
		if code != 1 {
			t.Fatalf("expected exit 1 from a failed overwrite item, got %d:\n%s", code, errStr)
		}
		if strings.Contains(out, "Total: 1 | Succeeded: 0 | Skipped: 0 | Failed: 1") == false {
			t.Errorf("unexpected summary:\n%s", out)
		}
		if fi, err := os.Stat(asDir); err != nil || !fi.IsDir() {
			t.Error("batch --overwrite deleted the directory at the output path")
		}
	})
}

// TestAddchAdversarialBatchDurationExceeded verifies the failed-item batch
// mechanics for a sidecar whose timestamp exceeds the video duration: the
// offending file fails cleanly, later candidates still succeed, and the process
// exits 1.
func TestAddchAdversarialBatchDurationExceeded(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	a := makeNamedContainer(t, dir, "a.mp4", "mp4", "1")
	if err := os.WriteFile(fsutil.SidecarPath(a), []byte("00:00:00 Intro\n00:00:59 TooLong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := makeNamedContainer(t, dir, "b.mp4", "mp4", "8")
	writeSidecar(t, b, batchChapters)

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("expected exit 1 with a failed item, got %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, `[failed] `+a+`: line 2: timestamp 00:00:59 exceeds the video duration`) {
		t.Errorf("missing failed-item diagnostic:\n%s", out)
	}
	if !strings.Contains(out, "Total: 2 | Succeeded: 1 | Skipped: 0 | Failed: 1") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(a)); err == nil {
		t.Error("failed item must not leave an output behind")
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(b)); err != nil {
		t.Errorf("healthy item output missing: %v", err)
	}
}

// TestAddchAdversarialBatchNonMonotonic pins the batch failure path for a
// sidecar whose chapters are present but not in chronological order (a
// validation-class mismatch rather than a parse error).
func TestAddchAdversarialBatchNonMonotonic(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
	if err := os.WriteFile(fsutil.SidecarPath(a), []byte("00:00:00 A\n00:00:05 B\n00:00:02 C\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "before the previous chapter") {
		t.Errorf("missing chronological-order diagnostic:\n%s", out)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(a)); err == nil {
		t.Error("failed item must not leave an output behind")
	}
}

// TestAddchAdversarialStaleTempsAndOrphanSidecars verifies that leftover temp
// artifacts (WriteFileAtomic temps, generic "<name>.mp4.tmp" partials) and
// orphan sidecars without a media file are invisible to batch discovery: the
// run proceeds normally, produces only the expected outputs, and leaves the
// litter untouched.
func TestAddchAdversarialStaleTempsAndOrphanSidecars(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	a := makeNamedContainer(t, dir, "a.mp4", "mp4", "8")
	writeSidecar(t, a, batchChapters)

	litter := []string{
		".addch-sidecar-stale01",
		"a-chapters.mp4.tmp",
		"random.mp4.tmp",
		"orphan.txt",
	}
	for _, name := range litter {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stale bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, errStr, code := runBatchForTest(dir, false, false)
	if code != 0 {
		t.Fatalf("stale artifacts must not break the run, exit %d:\n%s\nstdout:\n%s", code, errStr, out)
	}
	if !strings.Contains(out, "Total: 1 | Succeeded: 1 | Skipped: 0 | Failed: 0") {
		t.Errorf("unexpected summary:\n%s", out)
	}
	for _, name := range litter {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("litter %q was touched: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "orphan-chapters.mp4")); err == nil {
		t.Error("orphan sidecar produced an output")
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(a)); err != nil {
		t.Errorf("expected output for real candidate: %v", err)
	}
}

// TestAddchAdversarialLatin1Title pins the foreign-unmappable-title behavior:
// a title containing a raw ISO-8859-1 byte parses and embeds, but because the
// container cannot round-trip the byte, post-embed verification fails cleanly,
// exit 1, with no output left behind and no temp leak.
func TestAddchAdversarialLatin1Title(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	chapFile := filepath.Join(dir, "latin1.txt")
	if err := os.WriteFile(chapFile, []byte("00:00:00 Intro\n00:00:05 caf\xe9 latin1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcBefore := readBytes(t, video)
	tempBefore := countTempMetadata()

	_, errStr, code := runEmbedForTest(chapFile, video, "", false)
	if code != 1 {
		t.Fatalf("expected exit 1 for a title that cannot round-trip, got %d", code)
	}
	if !strings.Contains(errStr, "verification failed") {
		t.Errorf("unexpected error: %s", errStr)
	}
	if _, err := os.Stat(fsutil.DefaultOutputPath(video)); !os.IsNotExist(err) {
		t.Error("failed verification must not leave an output behind")
	}
	if !bytes.Equal(readBytes(t, video), srcBefore) {
		t.Error("input was modified")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked: before=%d after=%d", tempBefore, got)
	}
}

// writeSimpleChapters writes a two-chapter sidecar and returns its path.
func writeSimpleChapters(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("00:00:00 Intro\n00:00:05 Final\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// durationMs probes the real duration of a video fixture in milliseconds.
func durationMs(t *testing.T, video string) int64 {
	t.Helper()
	d, err := media.GetVideoDurationMs(video)
	if err != nil {
		t.Fatalf("probe duration of %s: %v", video, err)
	}
	return d
}

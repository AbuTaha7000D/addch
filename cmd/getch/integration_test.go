package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

// requireTools skips integration tests if ffmpeg/ffprobe are unavailable.
func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("integration test skipped: %s not found in PATH", tool)
		}
	}
}

// makeTestVideo creates a small, real video file and returns its path.
func makeTestVideo(t *testing.T, dir string, durationSec string) string {
	t.Helper()
	path := filepath.Join(dir, "source.mp4")
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=" + durationSec + ":size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=" + durationSec,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p",
		"-shortest",
		path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not create test video: %v\n%s", err, out)
	}
	return path
}

// makeTestContainer creates a small, real video fixture in the requested
// container ("mp4" or "mkv") and returns its path. The mp4 is produced by
// makeTestVideo; other containers are transmuxed from it with stream copy.
func makeTestContainer(t *testing.T, dir, container, durationSec string) string {
	t.Helper()
	mp4 := makeTestVideo(t, dir, durationSec)
	if container == "mp4" {
		return mp4
	}
	out := filepath.Join(dir, "source."+container)
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", mp4, "-c", "copy", out}
	if o, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not create %s source fixture: %v\n%s", container, err, o)
	}
	// Drop the intermediate mp4 so it never leaks into getch discovery (which,
	// like rmch, has no sidecar filter).
	_ = os.Remove(mp4)
	return out
}

// makeNamedContainer creates a small real media fixture and renames it to name
// so several distinct files can coexist in one directory.
func makeNamedContainer(t *testing.T, dir, name, container, durationSec string) string {
	t.Helper()
	src := makeTestContainer(t, dir, container, durationSec)
	dest := filepath.Join(dir, name)
	if err := os.Rename(src, dest); err != nil {
		t.Fatalf("rename %s to %s: %v", src, name, err)
	}
	return dest
}

// seedChapters are the chapter markers embedded into getch fixtures. The seeds
// start at 0/3/6s, so fixtures must be at least ~6s long.
var seedChapters = []chapters.Chapter{
	{Start: 0, Title: "Intro"},
	{Start: 3000, Title: "Middle"},
	{Start: 6000, Title: "End"},
}

// makeChapteredFixture creates a real media file in dir with chapters embedded
// (via the production media core), named name, and returns its path. The plain
// seed is removed afterwards so the directory only contains the chaptered file.
func makeChapteredFixture(t *testing.T, dir, name, container, durationSec string) string {
	t.Helper()
	src := makeNamedContainer(t, dir, "seed."+container, container, durationSec)
	durationMs, err := media.GetVideoDurationMs(src)
	if err != nil {
		t.Fatalf("[%s] probe seed: %v", container, err)
	}
	meta := media.BuildMetadata(seedChapters, durationMs)
	metaPath, err := media.WriteTempMetadata(meta)
	if err != nil {
		t.Fatalf("[%s] write metadata: %v", container, err)
	}
	defer os.Remove(metaPath)

	dest := filepath.Join(dir, name)
	ext := fsutil.OutputExtension(dest)
	rp, err := media.StartRemux(src, metaPath, dest, ext, false)
	if err != nil {
		t.Fatalf("[%s] start remux: %v", container, err)
	}
	if err := rp.Wait(); err != nil {
		t.Fatalf("[%s] remux failed: %v", container, err)
	}
	if err := media.VerifyChapters(dest, seedChapters, durationMs); err != nil {
		t.Fatalf("[%s] seed verification failed: %v", container, err)
	}
	_ = os.Remove(src)
	return dest
}

// runExtractForTest drives the full single-file extraction pipeline for real
// file paths, capturing output (chapter data on stdout, diagnostics on stderr)
// into string buffers and returning the exit code.
func runExtractForTest(video, output string, overwrite bool) (string, string, int) {
	pa := &parsedArgs{video: video, output: output, overwrite: overwrite}
	var out, errBuf bytes.Buffer
	code := runExtract(pa, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

// probeChaptersRaw runs ffprobe -show_chapters and returns the parsed chapters.
func probeChaptersRaw(t *testing.T, path string) []media.ProbeChapter {
	t.Helper()
	out, err := media.ProbeJSON("-show_chapters", path)
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	var pc struct {
		Chapters []media.ProbeChapter `json:"chapters"`
	}
	if err := json.Unmarshal(out, &pc); err != nil {
		t.Fatalf("parse chapters: %v", err)
	}
	return pc.Chapters
}

// chaptersFromRaw converts a file's raw ffprobe chapters into the toolkit model.
func chaptersFromRaw(t *testing.T, path string) []chapters.Chapter {
	t.Helper()
	conv, err := media.ChaptersFromProbe(probeChaptersRaw(t, path))
	if err != nil {
		t.Fatalf("convert probe for %s: %v", path, err)
	}
	return conv
}

// countTempMetadata returns the number of addch temp metadata files currently
// in the metadata temp directory (getch fixtures create them through the shared
// media core; none may leak). When ADDCH_METADATA_TMPDIR is set the count is
// scoped to that private directory (see media.WriteTempMetadata); otherwise the
// system temp directory is used.
func countTempMetadata() int {
	dir := os.Getenv("ADDCH_METADATA_TMPDIR")
	if dir == "" {
		dir = os.TempDir()
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "addch-metadata-*"))
	return len(matches)
}

// countSidecarTemp returns the number of WriteFileAtomic temp files left in dir.
func countSidecarTemp(dir string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, ".addch-sidecar-*"))
	return len(matches)
}

func TestExtractMP4(t *testing.T) {
	extractContainer(t, "mp4")
}

func TestExtractMKV(t *testing.T) {
	extractContainer(t, "mkv")
}

// extractContainer is the shared MP4+MKV extraction proof (R1/R4): stdout is
// exactly the canonical form of the content FFprobe reports, which for these
// containers is the embedded seed's canonical TXT; parsing the extracted text
// reproduces the original parse exactly; the source is byte-identical; and no
// temp metadata leaks.
func extractContainer(t *testing.T, container string) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered."+container, container, "10")

	srcBefore, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("[%s] read source: %v", container, err)
	}
	tempBefore := countTempMetadata()

	// The container must really carry the seed chapters.
	got := chaptersFromRaw(t, input)
	want := []chapters.Chapter{seedChapters[0], seedChapters[1], seedChapters[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("[%s] fixture chapters %+v, want seed %+v", container, got, want)
	}

	out, errStr, code := runExtractForTest(input, "", false)
	if code != 0 {
		t.Fatalf("[%s] runExtract failed code %d:\n%s", container, code, errStr)
	}
	canonical := chapters.Format(seedChapters)
	if out != canonical {
		t.Errorf("[%s] stdout does not match canonical seed text:\n got %q\nwant %q", container, out, canonical)
	}

	// R4: parsing the extracted text reproduces the original parse.
	parsed, err := chapters.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatalf("[%s] extracted output must be re-parseable: %v", container, err)
	}
	origParsed, err := chapters.Parse(strings.NewReader(chapters.Format(seedChapters)))
	if err != nil {
		t.Fatalf("[%s] parse of canonical seed failed: %v", container, err)
	}
	if !reflect.DeepEqual(parsed, origParsed) {
		t.Errorf("[%s] Parse(extracted) = %+v, want Parse(original) = %+v", container, parsed, origParsed)
	}

	if srcAfter, err := os.ReadFile(input); err != nil {
		t.Fatalf("[%s] read source after extraction: %v", container, err)
	} else if !bytes.Equal(srcBefore, srcAfter) {
		t.Errorf("[%s] source video was modified", container)
	}
	if countSidecarTemp(dir) != 0 {
		t.Errorf("[%s] sidecar temp files leaked", container)
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("[%s] temp metadata leaked: before=%d after=%d", container, tempBefore, got)
	}
}

// TestExtractRoundTripFullChain is the R2 proof: embed txt1 into v1 (done by
// makeChapteredFixture), extract txt2 from v1, embed txt2 into v2, then extract
// txt3 and require txt3 to equal the canonical form of txt2's parse — the chain
// closes.
func TestExtractRoundTripFullChain(t *testing.T) {
	requireTools(t)

	for _, container := range []string{"mp4", "mkv"} {
		container := container
		t.Run(container, func(t *testing.T) {
			dir := t.TempDir()
			v1 := makeChapteredFixture(t, dir, "v1."+container, container, "10")

			_, errStr, code := runExtractForTest(v1, "", false)
			if code != 0 {
				t.Fatalf("[%s] first extraction failed code %d:\n%s", container, code, errStr)
			}
			txtPath := filepath.Join(dir, "extracted.txt")
			out, _, code := runExtractForTest(v1, txtPath, false)
			if code != 0 {
				t.Fatalf("[%s] extraction to file failed code %d", container, code)
			}
			if out != "" {
				t.Errorf("[%s] file-output mode wrote chapter data to stdout: %q", container, out)
			}
			txt2, err := os.ReadFile(txtPath)
			if err != nil {
				t.Fatalf("[%s] read txt2: %v", container, err)
			}
			chs, err := chapters.Parse(strings.NewReader(string(txt2)))
			if err != nil {
				t.Fatalf("[%s] txt2 must be parseable: %v", container, err)
			}
			seedParsed, err := chapters.Parse(strings.NewReader(chapters.Format(seedChapters)))
			if err != nil {
				t.Fatalf("[%s] parse of canonical seed failed: %v", container, err)
			}
			if !reflect.DeepEqual(chs, seedParsed) {
				t.Fatalf("[%s] txt2 chapters %+v, want seed %+v", container, chs, seedParsed)
			}

			v2 := filepath.Join(dir, "v2."+container)
			meta := media.BuildMetadata(chs, 10000)
			metaPath, err := media.WriteTempMetadata(meta)
			if err != nil {
				t.Fatalf("[%s] write txt2 metadata: %v", container, err)
			}
			defer os.Remove(metaPath)
			ext := fsutil.OutputExtension(v2)
			rp, err := media.StartRemux(v1, metaPath, v2, ext, false)
			if err != nil {
				t.Fatalf("[%s] start remux of txt2: %v", container, err)
			}
			if err := rp.Wait(); err != nil {
				t.Fatalf("[%s] remux of txt2 failed: %v", container, err)
			}
			if err := media.VerifyChapters(v2, chs, 10000); err != nil {
				t.Fatalf("[%s] v2 verification failed: %v", container, err)
			}

			out3, errStr3, code3 := runExtractForTest(v2, "", false)
			if code3 != 0 {
				t.Fatalf("[%s] second extraction failed code %d:\n%s", container, code3, errStr3)
			}
			if out3 != chapters.Format(chs) {
				t.Errorf("[%s] txt3 = %q, want canonical form of txt2 = %q", container, out3, chapters.Format(chs))
			}
			parsed3, err := chapters.Parse(strings.NewReader(out3))
			if err != nil {
				t.Fatalf("[%s] txt3 must be re-parseable: %v", container, err)
			}
			if !reflect.DeepEqual(parsed3, chs) {
				t.Errorf("[%s] Parse(txt3) = %+v, want %+v", container, parsed3, chs)
			}
		})
	}
}

func TestExtractZeroChapters(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "10")
	srcBefore, _ := os.ReadFile(video)

	// stdout mode: success with an empty stdout.
	out, _, code := runExtractForTest(video, "", false)
	if code != 0 {
		t.Errorf("zero-chapters input must exit 0, got %d", code)
	}
	if out != "" {
		t.Errorf("zero-chapters input must produce empty stdout, got %q", out)
	}
	if got, _ := os.ReadFile(video); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the zero-chapters run")
	}

	// file-output mode: success, nothing written.
	sidecar := filepath.Join(dir, "zero.txt")
	out2, errStr, code2 := runExtractForTest(video, sidecar, false)
	if code2 != 0 {
		t.Errorf("zero-chapters -o run must exit 0, got %d", code2)
	}
	if out2 != "" {
		t.Errorf("zero-chapters -o run must keep stdout empty, got %q", out2)
	}
	if !strings.Contains(errStr, "nothing written") {
		t.Errorf("expected a 'nothing written' note on stderr, got %q", errStr)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Errorf("no file must be created for a zero-chapters input: %v", err)
	}
}

func TestExtractOutputFile(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	srcBefore, _ := os.ReadFile(input)
	output := filepath.Join(dir, "chapters.txt")

	out, errStr, code := runExtractForTest(input, output, false)
	if code != 0 {
		t.Fatalf("extraction to file failed code %d:\n%s", code, errStr)
	}
	if out != "" {
		t.Errorf("file-output mode must keep stdout empty, got %q", out)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if string(got) != chapters.Format(seedChapters) {
		t.Errorf("output = %q, want %q", got, chapters.Format(seedChapters))
	}
	if countSidecarTemp(dir) != 0 {
		t.Errorf("sidecar temp files leaked after successful write")
	}
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the -o run")
	}
}

func TestExtractOutputFailureCleansUp(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	srcBefore, _ := os.ReadFile(input)

	output := filepath.Join(dir, "absent", "chapters.txt")
	_, errStr, code := runExtractForTest(input, output, false)
	if code == 0 {
		t.Fatal("expected failure when the output directory is missing")
	}
	if errStr == "" {
		t.Fatal("expected an error message")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("no file must exist after a failed write: %v", err)
	}
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the failed run")
	}
}

func TestExtractOutputRefusalAndOverwrite(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	output := filepath.Join(dir, "chapters.txt")

	if _, errStr, code := runExtractForTest(input, output, false); code != 0 {
		t.Fatalf("first extraction should succeed, got code %d:\n%s", code, errStr)
	}
	orig, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	// Second run without --overwrite must fail and leave the output untouched.
	_, errStr, code := runExtractForTest(input, output, false)
	if code == 0 {
		t.Fatal("second extraction should fail without --overwrite")
	}
	if !strings.Contains(errStr, "already exists") {
		t.Errorf("unexpected error message: %q", errStr)
	}
	if got, _ := os.ReadFile(output); !bytes.Equal(got, orig) {
		t.Error("existing output was replaced despite no --overwrite")
	}

	if _, _, code := runExtractForTest(input, output, true); code != 0 {
		t.Fatal("extraction with --overwrite should succeed")
	}
	if got, _ := os.ReadFile(output); string(got) != chapters.Format(seedChapters) {
		t.Errorf("overwritten output = %q, want %q", got, chapters.Format(seedChapters))
	}
}

func TestExtractOutputSamePath(t *testing.T) {
	requireTools(t)
	t.Setenv("ADDCH_METADATA_TMPDIR", t.TempDir())

	dir := t.TempDir()
	input := makeChapteredFixture(t, dir, "chaptered.mp4", "mp4", "10")
	srcBefore, _ := os.ReadFile(input)
	tempBefore := countTempMetadata()

	_, errStr, code := runExtractForTest(input, input, false)
	if code == 0 {
		t.Fatal("an output equal to the input must be refused")
	}
	if !strings.Contains(errStr, "would overwrite the input video") {
		t.Errorf("unexpected error message: %q", errStr)
	}
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the same-path refusal")
	}
	if got := countTempMetadata(); got != tempBefore {
		t.Errorf("temp metadata leaked after refusal: before=%d after=%d", tempBefore, got)
	}
}

// TestExtractForeignChaptersMKV pins getch's faithfulness on a container that
// genuinely preserves a non-toolkit chapter layout: chapters the mkvmuxer keeps
// verbatim include a first chapter at 1s (not 0), an empty title, and exact
// starts. getch must reproduce them exactly — no zero-first invention, no title
// fill, no dedup, no truncation. Verbatim faithful output is deliberately NOT
// re-embeddable (an empty title would not re-parse), and that is the point.
func TestExtractForeignChaptersMKV(t *testing.T) {
	requireTools(t)

	dir := t.TempDir()
	src := makeTestVideo(t, dir, "10")
	meta := filepath.Join(dir, "foreign.txt")
	ffmeta := ";FFMETADATA1\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=5000\ntitle=Early\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=3000\nEND=5000\ntitle=\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=5000\nEND=8000\ntitle=Late\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=9000\nEND=10000\ntitle=Final\n"
	if err := os.WriteFile(meta, []byte(ffmeta), 0o644); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "foreign.mkv")
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", src, "-i", meta, "-map_metadata", "1", "-c", "copy", input}
	if o, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("could not build foreign mkv: %v\n%s", err, o)
	}
	_ = os.Remove(src)
	srcBefore, _ := os.ReadFile(input)

	// Pin the muxer reality: starts exact, empty title verbatim, first != 0.
	want := []chapters.Chapter{
		{Start: 1000, Title: "Early"},
		{Start: 3000, Title: ""},
		{Start: 5000, Title: "Late"},
		{Start: 9000, Title: "Final"},
	}
	got := chaptersFromRaw(t, input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("foreign fixture chapters %+v, want %+v", got, want)
	}

	out, errStr, code := runExtractForTest(input, "", false)
	if code != 0 {
		t.Fatalf("extraction failed code %d:\n%s", code, errStr)
	}
	if out != chapters.Format(want) {
		t.Errorf("stdout = %q, want faithful %q", out, chapters.Format(want))
	}
	// The empty-title line keeps FFmpeg's verbatim title (a trailing space).
	if !strings.Contains(out, "00:00:03 \n") {
		t.Errorf("expected the empty-title line to keep its trailing space, got %q", out)
	}
	// Proves no normalization was applied to make the output re-embeddable.
	if _, err := chapters.Parse(strings.NewReader(out)); err == nil {
		t.Error("faithful foreign output must not re-parse (that would mean normalization)")
	}
	if got, _ := os.ReadFile(input); !bytes.Equal(got, srcBefore) {
		t.Error("source was modified by the foreign extraction")
	}
	if countSidecarTemp(dir) != 0 {
		t.Errorf("sidecar temp files leaked")
	}
}

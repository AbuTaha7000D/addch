package clitest

import (
	"encoding/json"
	"fmt"
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

// metadataSeed are the chapters embedded into the rich fixtures; both start
// times sit inside the 5-second fixture duration.
var metadataSeed = []chapters.Chapter{
	{Start: 0, Title: "Intro"},
	{Start: 2250, Title: "Middle"},
}

// probeStreamEntry is the subset of ffprobe -show_streams used for stream
// preservation assertions (codec identity, language tag, attached-pic flag).
type probeStreamEntry struct {
	Index       int    `json:"index"`
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
	Tags struct {
		Language string `json:"language"`
	} `json:"tags"`
}

func probeRichStreams(t *testing.T, path string) []probeStreamEntry {
	t.Helper()
	out, err := media.ProbeJSON("-show_streams", path)
	if err != nil {
		t.Fatalf("ffprobe streams %q: %v", path, err)
	}
	var sf struct {
		Streams []probeStreamEntry `json:"streams"`
	}
	if err := json.Unmarshal(out, &sf); err != nil {
		t.Fatalf("parse streams %q: %v", path, err)
	}
	return sf.Streams
}

func probeFormatTags(t *testing.T, path string) map[string]string {
	t.Helper()
	out, err := media.ProbeJSON("-show_format", path)
	if err != nil {
		t.Fatalf("ffprobe format %q: %v", path, err)
	}
	var pf struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &pf); err != nil {
		t.Fatalf("parse format %q: %v", path, err)
	}
	return pf.Format.Tags
}

func probeChapterModel(t *testing.T, path string) []chapters.Chapter {
	t.Helper()
	out, err := media.ProbeJSON("-show_chapters", path)
	if err != nil {
		t.Fatalf("ffprobe chapters %q: %v", path, err)
	}
	var pc struct {
		Chapters []media.ProbeChapter `json:"chapters"`
	}
	if err := json.Unmarshal(out, &pc); err != nil {
		t.Fatalf("parse chapters %q: %v", path, err)
	}
	conv, err := media.ChaptersFromProbe(pc.Chapters)
	if err != nil {
		t.Fatalf("convert chapters %q: %v", path, err)
	}
	return conv
}

// makeRichMedia builds a real media fixture carrying unusual-but-supported
// metadata: container title/artist/album/creation_time tags, two audio streams
// with distinct language tags, an attached-picture cover stream, and (for MKV)
// a language-tagged subtitle track.
func makeRichMedia(t *testing.T, dir, container string) string {
	t.Helper()
	cover := filepath.Join(dir, "cover.png")
	if o, err := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=0x3366ff:size=64x64", "-frames:v", "1", cover).CombinedOutput(); err != nil {
		t.Fatalf("create cover: %v\n%s", err, o)
	}

	args := []string{"-y", "-loglevel", "error"}
	args = append(args,
		"-f", "lavfi", "-i", "testsrc=duration=5:size=96x72:rate=8",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=5",
		"-f", "lavfi", "-i", "sine=frequency=660:duration=5",
		"-i", cover)
	subIndex := -1
	if container == "mkv" {
		subs := filepath.Join(dir, "subs.srt")
		if err := os.WriteFile(subs, []byte("1\n00:00:00,000 --> 00:00:04,000\ndialogue\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", subs)
		subIndex = 4
	}
	args = append(args,
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-map", "3:v:0")
	if subIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:s:0", subIndex), "-c:s", "srt")
	}
	args = append(args,
		"-c:v:0", "libx264", "-c:v:1", "png", "-c:a", "aac",
		"-disposition:v:1", "attached_pic",
		"-metadata:s:a:0", "language=eng",
		"-metadata:s:a:1", "language=fre",
		"-metadata", "title=Rich Title",
		"-metadata", "artist=The Artist",
		"-metadata", "album=The Album",
		"-metadata", "creation_time=2024-01-02T03:04:05Z",
		"-t", "5")
	if subIndex >= 0 {
		args = append(args, "-metadata:s:s:0", "language=spa")
	}
	dest := filepath.Join(dir, "rich."+container)
	args = append(args, dest)
	if o, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("build rich %s fixture: %v\n%s", container, err, o)
	}
	return dest
}

// chapterizeMedia embeds metadataSeed into src and writes dest, returning dest.
func chapterizeMedia(t *testing.T, src, dest string) string {
	t.Helper()
	durationMs, err := media.GetVideoDurationMs(src)
	if err != nil {
		t.Fatalf("probe rich source: %v", err)
	}
	metaPath, err := media.WriteTempMetadata(media.BuildMetadata(metadataSeed, durationMs))
	if err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	defer os.Remove(metaPath)
	rp, err := media.StartRemux(src, metaPath, dest, fsutil.OutputExtension(dest), false)
	if err != nil {
		t.Fatalf("start remux: %v", err)
	}
	if err := rp.Wait(); err != nil {
		t.Fatalf("remux failed: %v", err)
	}
	if err := media.VerifyChapters(dest, metadataSeed, durationMs); err != nil {
		t.Fatalf("verify rich chapters: %v", err)
	}
	return dest
}

// requireStreamSignaturePreserved asserts every meaningful stream of src
// (codec identity + language, and the attached-picture flag on the cover)
// still exists in out with the same signature.
func requireStreamSignaturePreserved(t *testing.T, src, out string) {
	t.Helper()
	srcStreams := probeRichStreams(t, src)
	outStreams := probeRichStreams(t, out)
	has := func(typ, codec, lang string, attached bool) bool {
		for _, s := range outStreams {
			if s.CodecType != typ || s.CodecName != codec {
				continue
			}
			if lang != "" && s.Tags.Language != lang {
				continue
			}
			if attached && s.Disposition.AttachedPic == 0 {
				continue
			}
			return true
		}
		return false
	}
	for _, s := range srcStreams {
		attached := s.CodecType == "video" && s.CodecName == "png" && s.Disposition.AttachedPic != 0
		lang := s.Tags.Language
		switch s.CodecType {
		case "video", "audio", "subtitle":
			if !has(s.CodecType, s.CodecName, lang, attached) {
				t.Errorf("stream %d %s/%s lang=%q attached=%v lost or re-encoded",
					s.Index, s.CodecType, s.CodecName, lang, attached)
			}
		}
	}
}

// requireFormatTagsPreserved asserts the given global metadata tags survive
// with their original values. Key comparison is case-insensitive because the
// Matroska muxer normalizes some tag names (e.g. "artist" -> "ARTIST").
func requireFormatTagsPreserved(t *testing.T, out string, want map[string]string) {
	t.Helper()
	tags := probeFormatTags(t, out)
	fold := func(k string) string { return strings.ToLower(k) }
	got := make(map[string]string, len(want))
	for k, v := range tags {
		got[fold(k)] = v
	}
	for wantKey, wantVal := range want {
		if v := got[fold(wantKey)]; v != wantVal {
			t.Errorf("global tag %q = %q, want %q", wantKey, v, wantVal)
		}
	}
}

// roundTripDurationTolMs bounds how far a chapter strip round trip may shift the
// reported container duration. Stream-copy remuxing lets muxers (notably Matroska)
// re-estimate duration from the timestamps, so a small drift is expected.
const roundTripDurationTolMs = 500

// TestMetadataStreamsAcrossTools verifies that unusual container metadata —
// global title/artist/album tags, language-tagged audio (and MKV subtitle)
// tracks, and an attached-picture cover — survives chapter embedding (addch),
// extraction (getch), and chapter removal (rmch) intact on both supported
// container families.
func TestMetadataStreamsAcrossTools(t *testing.T) {
	for _, container := range []string{"mp4", "mkv"} {
		container := container
		t.Run(container, func(t *testing.T) {
			for _, tool := range []string{"ffmpeg", "ffprobe"} {
				if _, err := exec.LookPath(tool); err != nil {
					t.Skipf("skipping rich-metadata cross-tool test: %s not found in PATH", tool)
				}
			}

			dir := t.TempDir()
			rich := makeRichMedia(t, dir, container)
			addch := buildTool(t, "addch")
			rmch := buildTool(t, "rmch")
			getch := buildTool(t, "getch")

			wantTags := map[string]string{
				"title":  "Rich Title",
				"artist": "The Artist",
				"album":  "The Album",
			}
			requireFormatTagsPreserved(t, rich, wantTags)

			// addch embed keeps the rich metadata and adds the seed chapters.
			sidecar := filepath.Join(dir, "sidecar.txt")
			if err := os.WriteFile(sidecar, []byte(chapters.Format(metadataSeed)), 0o644); err != nil {
				t.Fatal(err)
			}
			chaptered := filepath.Join(dir, "rich-chapters."+container)
			if _, errBuf, code := runTool(t, addch, "-o", chaptered, sidecar, rich); code != 0 {
				t.Fatalf("addch rich embed failed code %d\n%s", code, errBuf)
			}
			if got := probeChapterModel(t, chaptered); !reflect.DeepEqual(got, metadataSeed) {
				t.Errorf("chaptered embed %+v, want seed %+v", got, metadataSeed)
			}
			requireStreamSignaturePreserved(t, rich, chaptered)
			requireFormatTagsPreserved(t, chaptered, wantTags)

			// getch extracts the exact canonical form.
			stdout, errBuf, code := runTool(t, getch, chaptered)
			if code != 0 {
				t.Fatalf("getch rich extract failed code %d\n%s", code, errBuf)
			}
			if stdout != chapters.Format(metadataSeed) {
				t.Errorf("getch rich output = %q, want %q", stdout, chapters.Format(metadataSeed))
			}

			// rmch strips chapters while keeping the rich metadata.
			noChapters := filepath.Join(dir, "rich-chapters-nochapters."+container)
			if _, errBuf, code := runTool(t, rmch, "-o", noChapters, chaptered); code != 0 {
				t.Fatalf("rmch rich removal failed code %d\n%s", code, errBuf)
			}
			if got := probeChapterModel(t, noChapters); len(got) != 0 {
				t.Errorf("rich removal left %d chapters", len(got))
			}
			requireStreamSignaturePreserved(t, rich, noChapters)
			requireFormatTagsPreserved(t, noChapters, wantTags)

			srcMs, err := media.GetVideoDurationMs(rich)
			if err != nil {
				t.Fatal(err)
			}
			outMs, err := media.GetVideoDurationMs(noChapters)
			if err != nil {
				t.Fatal(err)
			}
			if diffMs(srcMs, outMs) > roundTripDurationTolMs {
				t.Errorf("removed duration %d ms, want %d ms (within %d ms)", outMs, srcMs, roundTripDurationTolMs)
			}
		})
	}
}

func diffMs(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

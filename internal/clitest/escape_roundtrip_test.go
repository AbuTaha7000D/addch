package clitest

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
)

// TestEscapeSensitiveTitlesRoundTrip is the automated escape-corruption probe:
// an escape-sensitive title set (literal '%', backslash sequences, '=', ';',
// '#', double quotes, emoji, an inner tab, a trailing space that the parser
// must trim, and sub-second timestamps written at 1/2/3 fractional digits) is
// embedded by the real addch binary and extracted by the real getch binary.
// The extracted text must be byte-identical to the canonical serialization of
// the original parse (and re-parse to the same model), proving no unescaped
// '%' or '\' metadata corruption anywhere in the FFMETADATA1 round trip.
func TestEscapeSensitiveTitlesRoundTrip(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping escape round-trip: %s not found in PATH", tool)
		}
	}

	dir := t.TempDir()
	video := makeTestVideo(t, dir, "plain.mp4")

	content := "00:00:00.000 percent%=eq;hash# \"quote\" \\slash emoji -🎬 tab\tinside  \n" +
		"00:00:01.5 Half Second Single Digit\n" +
		"00:00:02.000 Zero Mmm Padding\n" +
		"00:00:02.9999 Truncated Beyond Ms\n"
	sidecar := filepath.Join(dir, "sidecar.txt")
	if err := os.WriteFile(sidecar, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := chapters.ParseFile(sidecar)
	if err != nil {
		t.Fatalf("parse sidecar: %v", err)
	}
	canonical := chapters.Format(parsed)

	addch := buildTool(t, "addch")
	getch := buildTool(t, "getch")
	out := filepath.Join(dir, "embedded.mp4")

	if _, errBuf, code := runTool(t, addch, "-o", out, sidecar, video); code != 0 {
		t.Fatalf("addch embed failed code %d\n%s", code, errBuf)
	}

	stdout, errBuf, code := runTool(t, getch, out)
	if code != 0 {
		t.Fatalf("getch extract failed code %d\n%s", code, errBuf)
	}
	if stdout != canonical {
		t.Errorf("getch stdout is not byte-identical to the canonical form of the parse:\n got %q\nwant %q", stdout, canonical)
	}

	extracted := filepath.Join(dir, "extracted.txt")
	if _, errBuf, code := runTool(t, getch, "-o", extracted, out); code != 0 {
		t.Fatalf("getch -o extract failed code %d\n%s", code, errBuf)
	}
	gotBytes, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != canonical {
		t.Errorf("getch -o file is not byte-identical to canonical form:\n got %q\nwant %q", gotBytes, canonical)
	}
	reparsed, err := chapters.Parse(strings.NewReader(stdout))
	if err != nil {
		t.Fatalf("extracted text must re-parse: %v", err)
	}
	if !reflect.DeepEqual(reparsed, parsed) {
		t.Errorf("Parse(extracted) = %+v, want original parse %+v", reparsed, parsed)
	}
}

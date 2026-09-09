package clitest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/fsutil"
)

// TestCrossToolPipelinePreservesInputAndCleansTemps is the Phase 8 item 4
// cross-command smoke: a real addch embed followed by a real getch extraction on
// the SAME fresh directory. The unmodified input video must carry an identical
// SHA-256 in, after addch, and after getch; the produced chaptered output must
// exist; and no addch metadata temp nor .addch-sidecar-* intermediate may
// survive either tool.
func TestCrossToolPipelinePreservesInputAndCleansTemps(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping cross-tool smoke test: %s not found in PATH", tool)
		}
	}

	dir := t.TempDir()
	metaTmp := filepath.Join(dir, "metatmp")
	if err := os.MkdirAll(metaTmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADDCH_METADATA_TMPDIR", metaTmp)

	video := makeTestVideo(t, dir, "plain.mp4")
	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n00:00:01 Second\n00:00:02 End\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash := func() string {
		t.Helper()
		data, err := os.ReadFile(video)
		if err != nil {
			t.Fatalf("read input for hashing: %v", err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	before := hash()

	addch := buildTool(t, "addch")
	if out, errBuf, code := runTool(t, addch, chaptersFile, video); code != 0 {
		t.Fatalf("addch embed: exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errBuf)
	}
	if got := hash(); got != before {
		t.Errorf("input video modified by addch: hash %s, want %s", got, before)
	}
	produced := fsutil.DefaultOutputPath(video)
	if _, err := os.Stat(produced); err != nil {
		t.Fatalf("expected produced output %s: %v", produced, err)
	}

	getch := buildTool(t, "getch")
	// getch on the ORIGINAL input must see a chapterless file (exit 0, empty
	// stdout): the input was never modified by addch, so no chapters are there.
	if out, errBuf, code := runTool(t, getch, video); code != 0 {
		t.Fatalf("getch on the untouched input: exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errBuf)
	} else if strings.Contains(out, "Intro") {
		t.Errorf("getch must not find chapters in the unmodified input, got:\n%s", out)
	}
	if got := hash(); got != before {
		t.Errorf("input video modified by getch: hash %s, want %s", got, before)
	}

	// The produced chaptered output also publishes its chapters to getch.
	if out, errBuf, code := runTool(t, getch, produced); code != 0 {
		t.Fatalf("getch on produced output: exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errBuf)
	} else if !strings.Contains(out, "Intro") {
		t.Errorf("getch on the produced output should print chapters, got:\n%s", out)
	}

	// Zero metadata temps survive either run, and no sidecar intermediate does.
	metaLeaks, err := filepath.Glob(filepath.Join(metaTmp, "addch-metadata-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(metaLeaks) != 0 {
		t.Errorf("addch metadata temp files survived: %v", metaLeaks)
	}
	sidecarLeaks, err := filepath.Glob(filepath.Join(dir, ".addch-sidecar-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sidecarLeaks) != 0 {
		t.Errorf(".addch-sidecar-* temp files survived: %v", sidecarLeaks)
	}
}

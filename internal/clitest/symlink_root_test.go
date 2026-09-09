package clitest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

// makeTestVideo creates a small real video via ffmpeg and returns its path.
func makeTestVideo(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=3:size=64x48:rate=8",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
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

// makeChapteredFixture creates a real chaptered video via the production media
// core and returns its path.
func makeChapteredFixture(t *testing.T, dir, name string) string {
	t.Helper()
	plain := makeTestVideo(t, dir, "plain-"+name)
	durationMs, err := media.GetVideoDurationMs(plain)
	if err != nil {
		t.Fatalf("probe plain video: %v", err)
	}
	chs := []chapters.Chapter{{Start: 0, Title: "Intro"}}
	meta := media.BuildMetadata(chs, durationMs)
	metaPath, err := media.WriteTempMetadata(meta)
	if err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	defer os.Remove(metaPath)

	dest := filepath.Join(dir, name)
	rp, err := media.StartRemux(plain, metaPath, dest, fsutil.OutputExtension(dest), false)
	if err != nil {
		t.Fatalf("start remux: %v", err)
	}
	if err := rp.Wait(); err != nil {
		t.Fatalf("remux failed: %v", err)
	}
	_ = os.Remove(plain)
	return dest
}

// TestRecursiveSymlinkRootAcrossTools is a cross-command smoke regression for
// the Phase 8 adversarial finding: a recursive batch whose root is a symlink to
// a directory must find and process the media file on every command instead of
// silently reporting "No media files found".
func TestRecursiveSymlinkRootAcrossTools(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping symlink-root smoke test: %s not found in PATH", tool)
		}
	}

	root := t.TempDir()
	for _, tool := range []string{"addch", "rmch", "getch"} {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			realDir := filepath.Join(root, tool+"-real")
			if err := os.MkdirAll(realDir, 0o755); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, tool+"-link")
			if err := os.Symlink(realDir, link); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}

			switch tool {
			case "addch":
				makeTestVideo(t, realDir, "clip.mp4")
				sidecar := fsutil.SidecarPath(filepath.Join(realDir, "clip.mp4"))
				if err := os.WriteFile(sidecar, []byte("00:00:00 Intro\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "rmch", "getch":
				makeChapteredFixture(t, realDir, "clip.mp4")
			}

			out, errBuf, code := runTool(t, buildTool(t, tool), "--recursive", link)
			if code != 0 {
				t.Fatalf("%s --recursive symlink root: exit %d\n%s", tool, code, errBuf)
			}
			// Batch reporting streams differ per tool (M3), so check both.
			combined := out + "\n" + errBuf
			if strings.Contains(combined, "No media files found") {
				t.Errorf("%s: symlink root must not report an empty discovery:\n%s", tool, combined)
			}

			outPath := filepath.Join(realDir, "clip-chapters.mp4")
			if tool == "rmch" {
				outPath = filepath.Join(realDir, "clip-nochapters.mp4")
			}
			if tool == "getch" {
				outPath = fsutil.SidecarPath(filepath.Join(realDir, "clip.mp4"))
			}
			if _, err := os.Stat(outPath); err != nil {
				t.Errorf("%s: expected processed output %q in the symlink target: %v", tool, outPath, err)
			}
		})
	}
}

package clitest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %q", dir)
		}
		dir = parent
	}
}

func buildTool(t *testing.T, tool string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), tool)
	// On Windows "go build" produces "<tool>.exe", so the returned path must
	// carry the extension for exec.LookPath to resolve it.
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+tool)
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", tool, err, out)
	}
	return bin
}

func runTool(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("running %s: %v", bin, err)
		}
	}
	return out.String(), errBuf.String(), code
}

func optionsBlock(help string) []string {
	lines := strings.Split(help, "\n")
	for i, l := range lines {
		if l != "Options:" {
			continue
		}
		var block []string
		for _, l := range lines[i+1:] {
			if strings.TrimSpace(l) == "" {
				return block
			}
			block = append(block, l)
		}
		return block
	}
	return nil
}

func commonOptionLines(block []string) []string {
	var out []string
	for _, l := range block {
		if strings.Contains(l, "--output") || strings.Contains(l, "--example") {
			continue
		}
		out = append(out, l)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHelpOptionsBlockIdenticalAcrossTools(t *testing.T) {
	specs := []struct {
		tool          string
		defaultSuffix string
	}{
		{"addch", "default: <video>-chapters.<ext>"},
		{"rmch", "default: <video>-nochapters.<ext>"},
		{"getch", "default: stdout"},
	}

	blocks := map[string][]string{}
	outputLines := map[string]string{}

	for _, spec := range specs {
		bin := buildTool(t, spec.tool)
		out, errBuf, code := runTool(t, bin, "--help")
		if code != 0 {
			t.Fatalf("%s --help: exit %d\nstdout:\n%s\nstderr:\n%s", spec.tool, code, out, errBuf)
		}
		block := optionsBlock(out)
		if len(block) == 0 {
			t.Fatalf("%s --help: no Options section found", spec.tool)
		}
		blocks[spec.tool] = block
		for _, l := range block {
			if strings.Contains(l, "--output") {
				outputLines[spec.tool] = l
			}
		}
		if outputLines[spec.tool] == "" {
			t.Fatalf("%s --help: no --output line in Options", spec.tool)
		}
	}

	for _, spec := range specs {
		if !strings.Contains(outputLines[spec.tool], spec.defaultSuffix) {
			t.Errorf("%s output line %q: want %q", spec.tool, outputLines[spec.tool], spec.defaultSuffix)
		}
	}

	want := commonOptionLines(blocks["addch"])
	for _, spec := range specs[1:] {
		got := commonOptionLines(blocks[spec.tool])
		if !equalStrings(got, want) {
			t.Errorf("%s Options common lines differ:\ngot:\n%s\nwant:\n%s", spec.tool, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}

	addchBlock := strings.Join(blocks["addch"], "\n")
	for _, tool := range []string{"rmch", "getch"} {
		if strings.Contains(strings.Join(blocks[tool], "\n"), "--example") {
			t.Errorf("%s must not document addch's --example flag:\n%s", tool, strings.Join(blocks[tool], "\n"))
		}
		if !strings.Contains(addchBlock, "--example") {
			t.Errorf("addch help must document its --example flag:\n%s", addchBlock)
		}
	}
}

func TestCheckStdoutByteIdenticalAcrossTools(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping --check identity test: %s not found in PATH", tool)
		}
	}
	outs := map[string]string{}
	for _, tool := range []string{"addch", "rmch", "getch"} {
		bin := buildTool(t, tool)
		out, errBuf, code := runTool(t, bin, "--check")
		if code != 0 {
			t.Fatalf("%s --check: exit %d\nstdout:\n%s\nstderr:\n%s", tool, code, out, errBuf)
		}
		if !strings.Contains(out, "System is ready.") {
			t.Errorf("%s --check: missing readiness line:\n%s", tool, out)
		}
		outs[tool] = out
	}
	if outs["addch"] != outs["rmch"] || outs["addch"] != outs["getch"] {
		t.Errorf("--check stdout must be byte-identical across tools\naddch:\n%s\nrmch:\n%s\ngetch:\n%s", outs["addch"], outs["rmch"], outs["getch"])
	}
}

func TestSamePathGuardWordingIdenticalAcrossTools(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping same-path guard test: %s not found in PATH", tool)
		}
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("dummy media; the guard fires before any probe"), 0o644); err != nil {
		t.Fatal(err)
	}
	chaptersFile := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chaptersFile, []byte("00:00:00 Intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := map[string][]string{
		"addch": {"-o", input, chaptersFile, input},
		"rmch":  {"-o", input, input},
		"getch": {"-o", input, input},
	}
	firstErrors := map[string]string{}
	for _, tool := range []string{"addch", "rmch", "getch"} {
		bin := buildTool(t, tool)
		out, errBuf, code := runTool(t, bin, args[tool]...)
		if code != 1 {
			t.Fatalf("%s with -o==input must exit 1, got %d\nstdout:\n%s\nstderr:\n%s", tool, code, out, errBuf)
		}
		first := ""
		for _, l := range strings.Split(errBuf, "\n") {
			if strings.Contains(l, "would overwrite the input video") {
				first = strings.TrimSpace(l)
				break
			}
		}
		if first == "" {
			t.Fatalf("%s: same-path guard error not found:\n%s", tool, errBuf)
		}
		firstErrors[tool] = first
	}
	for _, tool := range []string{"rmch", "getch"} {
		if firstErrors[tool] != firstErrors["addch"] {
			t.Errorf("same-path guard wording differs:\naddch: %q\n%s: %q", firstErrors["addch"], tool, firstErrors[tool])
		}
	}
}

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// H1 regression: even though getch now accepts an ffmpeg-less PATH, rmch strips
// chapters with ffmpeg and must still REQUIRE both tools. Re-exec the real
// binary (via the RMCH_TEST_BINARY hook) with a PATH exposing only ffprobe and
// prove --check refuses to proceed.

func makePathShim(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range tools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("could not locate %s for PATH shim: %v", tool, err)
		}
		if runtime.GOOS == "windows" {
			// Chocolatey exposes ffprobe/ffmpeg as a shimgen launcher in
			// <root>\bin that execs its real binary via a self-relative path
			// (<root>\lib\<pkg>\tools\<pkg>\bin\<tool>.exe). Relocating that
			// launcher into a temp PATH breaks the relative resolution, so stage
			// a self-contained copy of the real binary plus its DLLs instead.
			realBin, binDir := resolveRealTool(t, tool, real)
			writeToolCopy(t, realBin, binDir, filepath.Join(dir, tool+exeExt))
		} else if err := os.Symlink(real, filepath.Join(dir, tool+exeExt)); err != nil {
			t.Fatalf("could not symlink %s into PATH shim: %v", tool, err)
		}
	}
	return dir
}

// resolveRealTool returns the actual Windows executable behind a Chocolatey
// shimgen launcher together with the directory holding its runtime DLLs. Such
// launchers live in <root>\bin\<tool>.exe and exec a target resolved relative
// to their own location, so they break when relocated. The original path is
// returned unchanged when the tool is not a relocated Chocolatey launcher.
func resolveRealTool(t *testing.T, tool, from string) (bin, binDir string) {
	t.Helper()
	srcDir := filepath.Dir(from)
	if !strings.EqualFold(filepath.Base(srcDir), "bin") {
		return from, srcDir
	}
	for _, pat := range []string{
		filepath.Join(filepath.Dir(srcDir), "lib", "*", "tools", "*", "bin", tool+exeExt),
		filepath.Join(filepath.Dir(srcDir), "lib", "*", "*", "bin", tool+exeExt),
	} {
		if matches, _ := filepath.Glob(pat); len(matches) > 0 {
			return matches[0], filepath.Dir(matches[0])
		}
	}
	return from, srcDir
}

// writeToolCopy stages a self-contained copy of a Windows tool (the executable
// plus its adjacent DLLs) into the PATH shim so it runs standalone from the
// isolated directory. Sibling executables are deliberately not copied so the
// shim exposes exactly the requested tool and nothing else.
func writeToolCopy(t *testing.T, src, srcDir, dst string) {
	t.Helper()
	exeb, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("could not read %s for PATH shim: %v", src, err)
	}
	if err := os.WriteFile(dst, exeb, 0o755); err != nil {
		t.Fatalf("could not write %s for PATH shim: %v", dst, err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("could not list %s for PATH shim DLLs: %v", srcDir, err)
	}
	for _, ent := range entries {
		if strings.EqualFold(filepath.Ext(ent.Name()), ".dll") {
			dep, err := os.ReadFile(filepath.Join(srcDir, ent.Name()))
			if err != nil {
				t.Fatalf("could not read DLL %s for PATH shim: %v", ent.Name(), err)
			}
			if err := os.WriteFile(filepath.Join(filepath.Dir(dst), ent.Name()), dep, 0o644); err != nil {
				t.Fatalf("could not write DLL %s into PATH shim: %v", ent.Name(), err)
			}
		}
	}
}

func runRmchWithPath(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	return runRmchChild(t, args, path, false, stdout, stderr)
}

// runRmchFakeTool runs the real rmch binary with the fake-tool aliases in shim
// resolving first, while every other tool (in particular the real ffmpeg and
// ffprobe) still resolves from the normal environment PATH — the same
// resolution path the CLI uses in production. Fake alias names never collide
// with real tools in a fakeToolShim, so appending the real PATH is lossless.
func runRmchFakeTool(t *testing.T, args []string, path string, stdout, stderr io.Writer) int {
	t.Helper()
	return runRmchChild(t, args, path, true, stdout, stderr)
}

func runRmchChild(t *testing.T, args []string, path string, realPath bool, stdout, stderr io.Writer) int {
	t.Helper()
	cmd := exec.Command(exePath(t), args...)
	env := []string{"RMCH_TEST_BINARY=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "Path=") || strings.HasPrefix(kv, "RMCH_TEST_BINARY=") {
			continue
		}
		env = append(env, kv)
	}
	if realPath {
		env = append(env, "PATH="+path+string(os.PathListSeparator)+os.Getenv("PATH"))
	} else {
		env = append(env, "PATH="+path)
	}
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err == nil {
		return 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	} else {
		t.Fatalf("could not run rmch child: %v", err)
		return -1
	}
}

// TestRmchCheckRequiresBothTools proves rmch still refuses to run when ffmpeg
// is absent even though ffprobe is present.
func TestRmchCheckRequiresBothTools(t *testing.T) {
	requireTools(t)

	shim := makePathShim(t, "ffprobe")
	var out, errBuf bytes.Buffer
	code := runRmchWithPath(t, []string{"--check"}, shim, &out, &errBuf)
	if code == 0 {
		t.Fatalf("rmch --check with ffprobe-only PATH must exit nonzero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "ffmpeg") {
		t.Errorf("expected an ffmpeg requirement hint on stderr, got:\n%s", errBuf.String())
	}
	if strings.Contains(out.String(), "System is ready.") {
		t.Errorf("rmch must not report ready with ffprobe-only PATH, got:\n%s", out.String())
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"
)

// Contract tests lock in the Phase 7 unified CLI surface: a single-line
// unknown-flag error, shared short-circuit modes (--check/--version/-h), one
// usage template, and usage-error exit codes. They are hermetic: only
// parseArgs/run are exercised with buffered writers, so no FFmpeg/FFprobe
// process is ever spawned and no media file is touched. (--check is asserted at
// parse time only; its output shape comes from the shared
// media.CheckDependencies function, so it is identical across all three tools
// by construction.)

func TestContractUnknownFlagSingleError(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--bogus"}, &out, &errBuf); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	lines := strings.Split(strings.TrimSuffix(errBuf.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one stderr line, got %d: %q", len(lines), errBuf.String())
	}
	if !strings.HasPrefix(lines[0], "Error: flag provided but not defined: -bogus") {
		t.Errorf("stderr line: %q, want Error: flag provided but not defined: -bogus", lines[0])
	}
	if n := strings.Count(errBuf.String(), "flag provided but not defined"); n != 1 {
		t.Errorf("flag error phrase must appear exactly once, got %d in %q", n, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "\nflag provided but not defined") {
		t.Errorf("raw flag-package self-print line must be absent: %q", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on usage errors, got %q", out.String())
	}
}

func TestContractCheckShortCircuit(t *testing.T) {
	// --check needs no positional arguments and short-circuits at parse time.
	for _, args := range [][]string{{"--check"}, {"--check", "ignored.mp4"}} {
		var out, errBuf bytes.Buffer
		pa, err := parseArgs(args, &out, &errBuf)
		if err != nil {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
			continue
		}
		if pa == nil || !pa.check {
			t.Errorf("parseArgs(%v): check not set: %+v", args, pa)
		}
		if out.Len() != 0 || errBuf.Len() != 0 {
			t.Errorf("parseArgs(%v): expected no output, got stdout=%q stderr=%q", args, out.String(), errBuf.String())
		}
	}
}

func TestContractVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--version"}, &out, &errBuf); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "rmch dev\n"; out.String() != want {
		t.Errorf("version output = %q, want %q", out.String(), want)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr must stay empty for --version, got %q", errBuf.String())
	}
}

func TestContractHelpShortCircuit(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		var out, errBuf bytes.Buffer
		if code := run(args, &out, &errBuf); code != 0 {
			t.Errorf("run(%v): exit code = %d, want 0", args, code)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("run(%v): help text missing from stdout: %q", args, out.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("run(%v): stderr must stay empty, got %q", args, errBuf.String())
		}
	}
}

func TestContractHelpTemplate(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--help"}, &out, &errBuf); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	help := out.String()
	for _, want := range []string{
		"rmch — remove chapters",
		"rmch [options] <video-file>",
		"Usage:",
		"Options:",
		"-o, --output <file>",
		"<video>-nochapters.<ext>",
		" --overwrite",
		" --dir",
		" --recursive",
		" --check",
		" --version",
		"-h, --help",
		"Examples:",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help text missing %q", want)
		}
	}
}

func TestContractParseSingleFile(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(*parsedArgs) bool
	}{
		{"single positional", []string{"video.mp4"},
			func(p *parsedArgs) bool { return p.video == "video.mp4" }},
		{"short -o", []string{"-o", "out.mp4", "video.mp4"},
			func(p *parsedArgs) bool { return p.output == "out.mp4" && p.video == "video.mp4" }},
		{"long --output", []string{"--output", "out.mp4", "video.mp4"},
			func(p *parsedArgs) bool { return p.output == "out.mp4" && p.video == "video.mp4" }},
		{"batch dir", []string{"--dir", "./videos"},
			func(p *parsedArgs) bool { return p.dir && p.dirPath == "./videos" }},
		{"batch recursive", []string{"--recursive", "./videos"},
			func(p *parsedArgs) bool { return p.recursive && p.dirPath == "./videos" }},
	}
	for _, tc := range tests {
		var out, errBuf bytes.Buffer
		pa, err := parseArgs(tc.args, &out, &errBuf)
		if err != nil {
			t.Errorf("%s: parseArgs(%v): unexpected error: %v", tc.name, tc.args, err)
			continue
		}
		if !tc.check(pa) {
			t.Errorf("%s: parseArgs(%v): unexpected result %+v", tc.name, tc.args, pa)
		}
	}
}

func TestContractUsageErrorsExit1(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		errPart string
	}{
		{"missing arguments", nil, "missing required arguments:"},
		{"too many arguments", []string{"a.mp4", "b.mp4"}, "too many arguments"},
		{"dir and recursive", []string{"--dir", "--recursive"}, "mutually exclusive"},
		{"dir with output", []string{"--dir", "--output", "out.mp4"}, "--output cannot be combined with"},
		{"recursive with output", []string{"--recursive", "-o", "out.mp4"}, "--output cannot be combined with"},
	}
	for _, tc := range tests {
		var out, errBuf bytes.Buffer
		if code := run(tc.args, &out, &errBuf); code != 1 {
			t.Errorf("%s: exit code = %d, want 1", tc.name, code)
		}
		if !strings.Contains(errBuf.String(), tc.errPart) {
			t.Errorf("%s: stderr missing %q: %q", tc.name, tc.errPart, errBuf.String())
		}
	}
}

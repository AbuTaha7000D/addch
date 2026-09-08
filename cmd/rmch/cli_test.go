package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseArgsHelp checks that both --help and -h select the help short-circuit
// without an error (so run() exits 0) and that help text goes to stdout.
func TestParseArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		var out, errBuf bytes.Buffer
		pa, err := parseArgs(args, &out, &errBuf)
		if err != nil {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
			continue
		}
		if pa == nil || !pa.help {
			t.Errorf("parseArgs(%v): expected help=true, got %+v", args, pa)
			continue
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("parseArgs(%v): help text missing from stdout: %q", args, out.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("parseArgs(%v): expected no stderr output, got %q", args, errBuf.String())
		}
	}
}

func TestParseArgsVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"--version"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pa.version {
		t.Fatal("expected version=true")
	}
	if !strings.Contains(out.String(), "rmch") {
		t.Errorf("version output should contain 'rmch': %q", out.String())
	}
}

func TestParseArgsCheck(t *testing.T) {
	// --check short-circuits before positional validation; extra args and no
	// args are both accepted at parse time.
	for _, args := range [][]string{{"--check"}, {"--check", "ignored.mp4"}} {
		var out, errBuf bytes.Buffer
		pa, err := parseArgs(args, &out, &errBuf)
		if err != nil {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
			continue
		}
		if pa == nil || !pa.check {
			t.Errorf("parseArgs(%v): expected check=true, got %+v", args, pa)
		}
	}
}

func TestParseArgsTooFewArgs(t *testing.T) {
	for _, args := range [][]string{{}} {
		var out, errBuf bytes.Buffer
		_, err := parseArgs(args, &out, &errBuf)
		if err == nil {
			t.Errorf("parseArgs(%v): expected error for too few args", args)
			continue
		}
		if !strings.Contains(errBuf.String(), "Usage:") {
			t.Errorf("parseArgs(%v): usage should be printed on missing args", args)
		}
		if !strings.Contains(err.Error(), "missing required arguments") {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
		}
	}
}

func TestParseArgsTooManyArgs(t *testing.T) {
	var out, errBuf bytes.Buffer
	_, err := parseArgs([]string{"a.mp4", "b.mp4"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for too many args")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseArgsOutputFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"-o", "out.mp4", "video.mp4"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa.output != "out.mp4" {
		t.Errorf("output = %q, want out.mp4", pa.output)
	}
	if pa.video != "video.mp4" {
		t.Errorf("video = %q, want video.mp4", pa.video)
	}
}

func TestParseArgsLongOutputFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"--output", "out.mkv", "vid.mkv"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa.output != "out.mkv" || pa.video != "vid.mkv" {
		t.Errorf("output/video mismatch: %+v", pa)
	}
}

func TestParseArgsDirAndRecursiveMutuallyExclusive(t *testing.T) {
	var out, errBuf bytes.Buffer
	_, err := parseArgs([]string{"--dir", "--recursive"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for --dir and --recursive together")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseArgsDirWithOutput(t *testing.T) {
	for _, args := range [][]string{{"--dir", "--output", "out.mp4"}, {"--dir", "-o", "out.mp4"}} {
		var out, errBuf bytes.Buffer
		_, err := parseArgs(args, &out, &errBuf)
		if err == nil {
			t.Errorf("parseArgs(%v): expected error for --dir with --output", args)
			continue
		}
		if !strings.Contains(err.Error(), "--output cannot be combined with") {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
		}
	}
}

func TestParseArgsRecursiveWithOutput(t *testing.T) {
	var out, errBuf bytes.Buffer
	_, err := parseArgs([]string{"--recursive", "--output", "out.mp4"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for --recursive with --output")
	}
	if !strings.Contains(err.Error(), "--output cannot be combined with") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseArgsBatchModeMissingDirectory(t *testing.T) {
	for _, args := range [][]string{{"--dir"}, {"--recursive"}} {
		var out, errBuf bytes.Buffer
		_, err := parseArgs(args, &out, &errBuf)
		if err == nil {
			t.Errorf("parseArgs(%v): expected error for missing directory", args)
			continue
		}
		if !strings.Contains(err.Error(), "requires a directory") {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
		}
	}
}

func TestParseArgsBatchModeTooManyArguments(t *testing.T) {
	for _, args := range [][]string{{"--dir", "a", "b"}, {"--recursive", "a", "b"}} {
		var out, errBuf bytes.Buffer
		_, err := parseArgs(args, &out, &errBuf)
		if err == nil {
			t.Errorf("parseArgs(%v): expected error for too many args in batch mode", args)
			continue
		}
		if !strings.Contains(err.Error(), "exactly one directory") {
			t.Errorf("parseArgs(%v): unexpected error: %v", args, err)
		}
	}
}

func TestParseArgsDir(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"--dir", "./videos"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pa.dir || pa.recursive {
		t.Errorf("expected dir=true, recursive=false, got %+v", pa)
	}
	if pa.dirPath != "./videos" {
		t.Errorf("dirPath = %q, want ./videos", pa.dirPath)
	}
	if pa.video != "" {
		t.Errorf("expected empty video in batch mode, got %+v", pa)
	}
}

func TestParseArgsRecursive(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"--recursive", "./videos"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pa.recursive || pa.dir {
		t.Errorf("expected recursive=true, dir=false, got %+v", pa)
	}
	if pa.dirPath != "./videos" {
		t.Errorf("dirPath = %q, want ./videos", pa.dirPath)
	}
	if pa.video != "" {
		t.Errorf("expected empty video in batch mode, got %+v", pa)
	}
}

func TestParseArgsSingleFileSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"video.mp4"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa.video != "video.mp4" {
		t.Errorf("video = %q, want video.mp4", pa.video)
	}
	if pa.dir || pa.recursive || pa.output != "" {
		t.Errorf("expected only video set, got %+v", pa)
	}
}

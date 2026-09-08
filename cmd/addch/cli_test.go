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
	if !strings.Contains(out.String(), "addch") {
		t.Errorf("version output should contain 'addch': %q", out.String())
	}
}

func TestParseArgsTooFewArgs(t *testing.T) {
	for _, args := range [][]string{{}, {"only-one"}} {
		var out, errBuf bytes.Buffer
		_, err := parseArgs(args, &out, &errBuf)
		if err == nil {
			t.Errorf("parseArgs(%v): expected error for too few args", args)
			continue
		}
		if !strings.Contains(errBuf.String(), "Usage:") {
			t.Errorf("parseArgs(%v): usage should be printed on missing args", args)
		}
	}
}

func TestParseArgsTooManyArgs(t *testing.T) {
	var out, errBuf bytes.Buffer
	_, err := parseArgs([]string{"a", "b", "c"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for too many args")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseArgsOutputFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"-o", "out.mp4", "chapters.txt", "video.mp4"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa.output != "out.mp4" {
		t.Errorf("output = %q, want out.mp4", pa.output)
	}
	if pa.chapters != "chapters.txt" || pa.video != "video.mp4" {
		t.Errorf("chapters/video mismatch: %+v", pa)
	}
}

func TestParseArgsSingleDashShortOutput(t *testing.T) {
	// Both -o and --output must set the same value.
	var out, errBuf bytes.Buffer
	pa, err := parseArgs([]string{"-o=out.mkv", "ch.txt", "vid.mkv"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa.output != "out.mkv" {
		t.Errorf("output = %q, want out.mkv", pa.output)
	}
}

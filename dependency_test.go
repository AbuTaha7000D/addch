package main

import (
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		line string
		want *Version
	}{
		{"ffmpeg version 5.1.3 Copyright (c)", &Version{5, 1, 3}},
		{"ffprobe version 4.4.2-0+deb11u1", &Version{4, 4, 2}},
		{"ffmpeg version 6.0", &Version{6, 0, 0}},
		{"garbage no version", nil},
	}
	for _, c := range cases {
		got := parseVersion(c.line)
		if c.want == nil {
			if got != nil {
				t.Errorf("parseVersion(%q) = %+v, want nil", c.line, got)
			}
			continue
		}
		if got == nil || *got != *c.want {
			t.Errorf("parseVersion(%q) = %+v, want %+v", c.line, got, c.want)
		}
	}
}

func TestInstallHintMessages(t *testing.T) {
	// When everything is present, no hint is returned.
	di := dependencyInfo{ffmpegPath: "/usr/bin/ffmpeg", ffprobePath: "/usr/bin/ffprobe"}
	if h := installHint(di); h != "" {
		t.Errorf("expected no hint when present, got: %q", h)
	}

	// When ffmpeg is missing, the hint mentions ffmpeg and a command.
	diMiss := dependencyInfo{ffprobePath: "/usr/bin/ffprobe"}
	h := installHint(diMiss)
	if h == "" {
		t.Fatal("expected an install hint")
	}
	if !strings.Contains(h, "ffmpeg") {
		t.Errorf("hint should mention ffmpeg: %v", h)
	}

	// Both missing -> the message says "ffmpeg and ffprobe were".
	bothMissing := installHint(dependencyInfo{})
	if !strings.Contains(bothMissing, "were") {
		t.Errorf("multiple-missing message should use 'were': %v", bothMissing)
	}
}

func TestListReports(t *testing.T) {
	di := dependencyInfo{ffmpegPath: "/x/ffmpeg", ffprobePath: ""}
	lines := di.listReports()
	if len(lines) != 2 {
		t.Fatalf("expected 2 report lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "/x/ffmpeg") {
		t.Errorf("ffmpeg line missing path: %v", lines[0])
	}
	if !strings.Contains(lines[1], "not found") {
		t.Errorf("ffprobe line should say not found: %v", lines[1])
	}

	// A parsed version is reported alongside the path.
	diVer := dependencyInfo{ffmpegPath: "/x/ffmpeg", ffmpegVer: &Version{5, 1, 3}, ffprobePath: ""}
	linesVer := diVer.listReports()
	if !strings.Contains(linesVer[0], "5.1.3") {
		t.Errorf("ffmpeg line should include version: %v", linesVer[0])
	}
}

func TestReady(t *testing.T) {
	empty := dependencyInfo{}
	if empty.ready() {
		t.Error("empty dependency should not be ready")
	}
	di := dependencyInfo{ffmpegPath: "/a", ffprobePath: "/b"}
	if !di.ready() {
		t.Error("fully populated dependency should be ready")
	}
	bad := dependencyInfo{ffmpegPath: "/a", ffprobePath: "/b", ffmpegErr: errTest}
	if bad.ready() {
		t.Error("dependency with version error should not be ready")
	}
}

func TestCommandForOSRelease(t *testing.T) {
	cases := []struct{ id, want string }{
		{"fedora", "sudo dnf install ffmpeg"},
		{"rhel", "sudo dnf install ffmpeg"},
		{"centos", "sudo dnf install ffmpeg"},
		{"ubuntu", "sudo apt install ffmpeg"},
		{"debian", "sudo apt install ffmpeg"},
		{"arch", "sudo pacman -S ffmpeg"},
		{"opensuse", "sudo zypper install ffmpeg"},
		{"nixos", "nix-shell -p ffmpeg"},
		{"unknown-distro", ""},
	}
	for _, c := range cases {
		if got := commandForOSRelease(c.id); got != c.want {
			t.Errorf("commandForOSRelease(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestCommandForPackageManager(t *testing.T) {
	cases := []struct{ pm, want string }{
		{"dnf", "sudo dnf install ffmpeg"},
		{"apt-get", "sudo apt install ffmpeg"},
		{"pacman", "sudo pacman -S ffmpeg"},
		{"zypper", "sudo zypper install ffmpeg"},
		{"brew", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := commandForPackageManager(c.pm); got != c.want {
			t.Errorf("commandForPackageManager(%q) = %q, want %q", c.pm, got, c.want)
		}
	}
}

func TestInstallCommandPlatforms(t *testing.T) {
	if got := installCommand("windows"); got == "" {
		t.Error("windows should map to an install command")
	}
	if got := installCommand("darwin"); got != "brew install ffmpeg" {
		t.Errorf("darwin command = %q", got)
	}
	if got := installCommand("plan9"); got != "" {
		t.Errorf("unknown OS should return empty, got %q", got)
	}
}

var errTest = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "fake" }

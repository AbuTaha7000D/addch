package media

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
	di := DependencyInfo{FFmpegPath: "/usr/bin/ffmpeg", FFprobePath: "/usr/bin/ffprobe"}
	if h := di.InstallHint(); h != "" {
		t.Errorf("expected no hint when present, got: %q", h)
	}

	// When ffmpeg is missing, the hint mentions ffmpeg and a command.
	diMiss := DependencyInfo{FFprobePath: "/usr/bin/ffprobe"}
	h := diMiss.InstallHint()
	if h == "" {
		t.Fatal("expected an install hint")
	}
	if !strings.Contains(h, "ffmpeg") {
		t.Errorf("hint should mention ffmpeg: %v", h)
	}

	// Both missing -> the message says "ffmpeg and ffprobe were".
	bothMissing := (DependencyInfo{}).InstallHint()
	if !strings.Contains(bothMissing, "were") {
		t.Errorf("multiple-missing message should use 'were': %v", bothMissing)
	}
}

func TestListReports(t *testing.T) {
	di := DependencyInfo{FFmpegPath: "/x/ffmpeg", FFprobePath: ""}
	lines := di.ListReports()
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
	diVer := DependencyInfo{FFmpegPath: "/x/ffmpeg", FFmpegVer: &Version{5, 1, 3}, FFprobePath: ""}
	linesVer := diVer.ListReports()
	if !strings.Contains(linesVer[0], "5.1.3") {
		t.Errorf("ffmpeg line should include version: %v", linesVer[0])
	}
}

func TestReady(t *testing.T) {
	empty := DependencyInfo{}
	if empty.Ready() {
		t.Error("empty dependency should not be ready")
	}
	di := DependencyInfo{FFmpegPath: "/a", FFprobePath: "/b"}
	if !di.Ready() {
		t.Error("fully populated dependency should be ready")
	}
	bad := DependencyInfo{FFmpegPath: "/a", FFprobePath: "/b", FFmpegErr: errTest}
	if bad.Ready() {
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

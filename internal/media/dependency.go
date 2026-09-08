// Package media implements the FFmpeg/FFprobe interaction core for the addch
// toolkit: dependency checks, duration probing, chapter verification, and
// FFmpeg remux execution.
package media

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Version is a parsed dotted version number.
type Version struct {
	Major, Minor, Patch int
}

// String renders the version as "major.minor.patch".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// checkTool locates an executable in PATH and returns its path, or an empty
// string if it is not found.
func checkTool(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// parseVersion extracts the first dotted numeric version from a "version" line.
func parseVersion(line string) *Version {
	fields := strings.Fields(line)
	for _, f := range fields {
		if f == "" || f[0] < '0' || f[0] > '9' {
			continue
		}
		var v Version
		n, err := fmt.Sscanf(f, "%d.%d.%d", &v.Major, &v.Minor, &v.Patch)
		if err == nil && n == 3 {
			return &v
		}
		var v2 Version
		if _, err := fmt.Sscanf(f, "%d.%d", &v2.Major, &v2.Minor); err == nil {
			return &v2
		}
	}
	return nil
}

// DependencyInfo collects the presence and version of each required tool.
// It never runs any mutating command.
type DependencyInfo struct {
	FFmpegPath  string
	FFprobePath string
	FFmpegVer   *Version
	FFprobeVer  *Version
	FFmpegErr   error
	FFprobeErr  error
}

// CheckDependencies probes for ffmpeg and ffprobe.
func CheckDependencies() DependencyInfo {
	var di DependencyInfo
	di.FFmpegPath = checkTool("ffmpeg")
	di.FFprobePath = checkTool("ffprobe")

	if di.FFmpegPath != "" {
		out, err := exec.Command("ffmpeg", "-version").Output()
		if err != nil {
			di.FFmpegErr = err
		} else {
			di.FFmpegVer = parseVersion(string(out))
			if di.FFmpegVer == nil {
				di.FFmpegErr = fmt.Errorf("could not parse ffmpeg version output")
			}
		}
	}
	if di.FFprobePath != "" {
		out, err := exec.Command("ffprobe", "-version").Output()
		if err != nil {
			di.FFprobeErr = err
		} else {
			di.FFprobeVer = parseVersion(string(out))
			if di.FFprobeVer == nil {
				di.FFprobeErr = fmt.Errorf("could not parse ffprobe version output")
			}
		}
	}
	return di
}

// Ready reports whether all required tools are present and functional.
func (di DependencyInfo) Ready() bool {
	return di.FFmpegPath != "" && di.FFprobePath != "" &&
		di.FFmpegErr == nil && di.FFprobeErr == nil
}

// ListReports renders a line-by-line report of dependency status for --check.
// A present tool is marked with a check and its path (plus version when parsed);
// a missing tool is marked clearly so the output is not misleading.
func (di DependencyInfo) ListReports() []string {
	lines := []string{
		fmt.Sprintf("%s FFmpeg:  %s", statusMark(di.FFmpegPath != ""), presentOrMissing(di.FFmpegPath, di.FFmpegVer)),
		fmt.Sprintf("%s FFprobe: %s", statusMark(di.FFprobePath != ""), presentOrMissing(di.FFprobePath, di.FFprobeVer)),
	}
	return lines
}

func statusMark(present bool) string {
	if present {
		return "✓"
	}
	return "✗"
}

func presentOrMissing(path string, v *Version) string {
	if path == "" {
		return "not found"
	}
	if v != nil {
		return fmt.Sprintf("%s (%s)", path, v)
	}
	return path
}

// InstallHint returns a human-readable message describing how to install
// FFmpeg on the detected platform, including the exact command to run.
func (di DependencyInfo) InstallHint() string {
	var missing []string
	if di.FFmpegPath == "" {
		missing = append(missing, "ffmpeg")
	}
	if di.FFprobePath == "" {
		missing = append(missing, "ffprobe")
	}
	if len(missing) == 0 {
		return ""
	}

	osName := runtime.GOOS
	cmd := installCommand(osName)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"%s %s not found in PATH.\n\n",
		strings.Join(missing, " and "),
		pluralWere(len(missing)),
	))
	b.WriteString("addch requires FFmpeg and FFprobe to embed chapters.\n\n")
	if cmd != "" {
		b.WriteString("To install on this system (" + prettyOS(osName) + "), run:\n\n")
		b.WriteString("\t" + cmd + "\n\n")
		b.WriteString("Also make sure ffmpeg and ffprobe are available in your PATH,\nthen run addch again.\n")
	} else {
		b.WriteString("There is no known one-line install command for " + prettyOS(osName) + ".\n")
		b.WriteString("Please install FFmpeg from https://ffmpeg.org/download.html and ensure\nffmpeg and ffprobe are available in your PATH.\n")
	}
	return b.String()
}

func pluralWere(n int) string {
	if n > 1 {
		return "were"
	}
	return "was"
}

func prettyOS(os string) string {
	switch os {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return os
	}
}

// installCommand returns a recommended install command for the current OS, or
// "" if none is known.
func installCommand(os string) string {
	switch os {
	case "windows":
		return "winget install --id Gyan.FFmpeg --exact"
	case "darwin":
		return "brew install ffmpeg"
	case "linux":
		return linuxInstallCommand()
	default:
		return ""
	}
}

// commandForOSRelease maps a /etc/os-release "ID" value to an install command.
func commandForOSRelease(id string) string {
	switch id {
	case "fedora", "rhel", "centos", "rocky", "alma":
		return "sudo dnf install ffmpeg"
	case "debian", "ubuntu", "linuxmint", "pop":
		return "sudo apt install ffmpeg"
	case "arch", "manjaro", "endeavouros":
		return "sudo pacman -S ffmpeg"
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "suse":
		return "sudo zypper install ffmpeg"
	case "nixos":
		return "nix-shell -p ffmpeg"
	default:
		return ""
	}
}

// commandForPackageManager maps a detected package manager binary to an
// install command.
func commandForPackageManager(pm string) string {
	switch pm {
	case "dnf":
		return "sudo dnf install ffmpeg"
	case "apt-get":
		return "sudo apt install ffmpeg"
	case "pacman":
		return "sudo pacman -S ffmpeg"
	case "zypper":
		return "sudo zypper install ffmpeg"
	default:
		return ""
	}
}

// linuxInstallCommand detects the Linux package manager / distro family where
// practical and returns an appropriate install command.
func linuxInstallCommand() string {
	if id, ok := readOSReleaseID(); ok {
		if cmd := commandForOSRelease(id); cmd != "" {
			return cmd
		}
	}
	// Fall back to probing for an installed package manager.
	for _, pm := range []string{"dnf", "apt-get", "pacman", "zypper"} {
		if _, err := exec.LookPath(pm); err == nil {
			return commandForPackageManager(pm)
		}
	}
	return ""
}

func readOSReleaseID() (string, bool) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "ID=") {
			v := strings.TrimPrefix(line, "ID=")
			v = strings.Trim(v, `"'`)
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

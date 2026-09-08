package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/abutaha/addch/internal/chapters"
)

// escapeMetadataTitle escapes a chapter title for embedding in an FFMETADATA1
// file. FFmpeg's FFMETADATA1 reader treats backslash ('\') as an escape character,
// so a literal backslash in a title must be doubled ('\\') to survive the round-trip.
// Other characters commonly found in titles (spaces, '=', '&', '%', ';', '[', ']',
// Unicode) pass through unchanged.
func escapeMetadataTitle(title string) string {
	if !strings.ContainsRune(title, '\\') {
		return title
	}
	return strings.ReplaceAll(title, `\`, `\\`)
}

// buildMetadata returns the contents of an FFMETADATA1 file describing the
// given chapters. Each chapter's END is the next chapter's START, and the final
// chapter's END is the video duration. Timestamps are stored in milliseconds
// via TIMEBASE=1/1000.
func buildMetadata(chs []chapters.Chapter, durationMs int64) string {
	var b strings.Builder
	b.WriteString(";FFMETADATA1\n")

	// Copy the input so we do not mutate the caller's slice.
	sorted := make([]chapters.Chapter, len(chs))
	copy(sorted, chs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	for i := range sorted {
		var end int64
		if i+1 < len(sorted) {
			end = sorted[i+1].Start
		} else {
			end = durationMs
		}
		fmt.Fprintf(&b, "\n[CHAPTER]\n")
		fmt.Fprintf(&b, "TIMEBASE=1/1000\n")
		fmt.Fprintf(&b, "START=%d\n", sorted[i].Start)
		fmt.Fprintf(&b, "END=%d\n", end)
		fmt.Fprintf(&b, "title=%s\n", escapeMetadataTitle(sorted[i].Title))
	}
	return b.String()
}

// writeTempMetadata writes the metadata content to a secure temporary file and
// returns its path. The caller is responsible for removing the file.
func writeTempMetadata(content string) (string, error) {
	f, err := os.CreateTemp("", "addch-metadata-*.txt")
	if err != nil {
		return "", fmt.Errorf("could not create temporary metadata file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("could not write temporary metadata file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("could not write temporary metadata file: %w", err)
	}
	return path, nil
}

// isMP4Family is defined in output.go.

// buildFFmpegArgs returns the argument vector for the FFmpeg remux command.
// overwrite controls whether FFmpeg is allowed to replace the output without
// prompting (-y). +faststart is only added for MP4-family containers.
func buildFFmpegArgs(inputVideo, inputMeta, output string, outputExt string, overwrite bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if overwrite {
		args = append(args, "-y")
	}
	args = append(args,
		"-i", inputVideo,
		"-i", inputMeta,
		"-map", "0",
		"-map_metadata", "0",
		"-map_chapters", "1",
		"-c", "copy",
	)
	if isMP4Family(outputExt) {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, output)
	return args
}

// ffmpegRemux controls a running FFmpeg remux process. It wraps the child so the
// caller can both wait for normal completion and interrupt (kill + wait) the
// child, which is required for correct signal handling.
type ffmpegRemux struct {
	cmd    *exec.Cmd
	output *strings.Builder
	errC   chan error
}

// newFFmpegRemux wraps an already-started command and begins waiting for it in a
// dedicated goroutine. Exactly one goroutine calls cmd.Wait(), so the child is
// reaped exactly once. Separate from startFFmpegRemux to allow test injection of
// substitute commands.
func newFFmpegRemux(cmd *exec.Cmd) *ffmpegRemux {
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	r := &ffmpegRemux{cmd: cmd, output: &buf}
	r.errC = make(chan error, 1)
	go func() {
		r.errC <- cmd.Wait()
	}()
	return r
}

// startFFmpegRemux builds and starts the FFmpeg remux command. Call wait() to
// obtain its final error, or interrupt() to terminate and reap the child.
func startFFmpegRemux(inputVideo, inputMeta, output string, outputExt string, overwrite bool) (*ffmpegRemux, error) {
	cmd := exec.Command("ffmpeg", buildFFmpegArgs(inputVideo, inputMeta, output, outputExt, overwrite)...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("FFmpeg failed to start: %w", err)
	}
	return newFFmpegRemux(cmd), nil
}

// wait blocks until FFmpeg exits and returns a descriptive error on failure.
// It must be called at most once.
func (r *ffmpegRemux) wait() error {
	err := <-r.errC
	if err != nil {
		msg := strings.TrimSpace(r.output.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("FFmpeg failed to remux the video: %s", msg)
	}
	return nil
}

// done returns a channel that yields the remux result exactly once, when the
// child exits. It is useful for select-based waiting alongside a signal channel.
func (r *ffmpegRemux) done() <-chan error {
	return r.errC
}

// interrupt asks the FFmpeg child to terminate and blocks until it has fully
// exited (reaped). It returns after the child has stopped, so callers can safely
// clean up the output file without racing a still-writing process.
func (r *ffmpegRemux) interrupt() {
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	_ = r.wait()
}

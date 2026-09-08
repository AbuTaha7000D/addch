package media

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/abutaha/addch/internal/chapters"
)

// EscapeMetadataTitle escapes a chapter title for embedding in an FFMETADATA1
// file. FFmpeg's FFMETADATA1 reader treats backslash ('\') as an escape character,
// so a literal backslash in a title must be doubled ('\\') to survive the round-trip.
// Other characters commonly found in titles (spaces, '=', '&', '%', ';', '[', ']',
// Unicode) pass through unchanged.
func EscapeMetadataTitle(title string) string {
	if !strings.ContainsRune(title, '\\') {
		return title
	}
	return strings.ReplaceAll(title, `\`, `\\`)
}

// BuildMetadata returns the contents of an FFMETADATA1 file describing the
// given chapters. Each chapter's END is the next chapter's START, and the final
// chapter's END is the video duration. Timestamps are stored in milliseconds
// via TIMEBASE=1/1000.
func BuildMetadata(chs []chapters.Chapter, durationMs int64) string {
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
		fmt.Fprintf(&b, "title=%s\n", EscapeMetadataTitle(sorted[i].Title))
	}
	return b.String()
}

// WriteTempMetadata writes the metadata content to a secure temporary file and
// returns its path. The caller is responsible for removing the file.
func WriteTempMetadata(content string) (string, error) {
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

// IsMP4Family reports whether an output extension is part of the MP4 family of
// containers that support the +faststart flag.
func IsMP4Family(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".3gp", ".3g2":
		return true
	default:
		return false
	}
}

// BuildFFmpegArgs returns the argument vector for the FFmpeg remux command.
// overwrite controls whether FFmpeg is allowed to replace the output without
// prompting (-y). +faststart is only added for MP4-family containers.
func BuildFFmpegArgs(inputVideo, inputMeta, output string, outputExt string, overwrite bool) []string {
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
	if IsMP4Family(outputExt) {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, output)
	return args
}

// BuildStripArgs returns the argument vector for the FFmpeg chapter-stripping
// command (rmch). It copies all streams (stream copy, no re-encode), preserves
// metadata, and explicitly disables chapter copying with -map_chapters -1 (the
// empirically verified requirement: without it, FFmpeg copies the source
// chapters into the output, so the output would keep its chapters). overwrite
// controls whether FFmpeg is allowed to replace the output without prompting
// (-y). +faststart is only added for MP4-family containers.
func BuildStripArgs(inputVideo, output string, outputExt string, overwrite bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if overwrite {
		args = append(args, "-y")
	}
	args = append(args,
		"-i", inputVideo,
		"-map", "0",
		"-map_metadata", "0",
		"-map_chapters", "-1",
		"-c", "copy",
	)
	if IsMP4Family(outputExt) {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, output)
	return args
}

// RemuxProcess controls a running FFmpeg remux process. It wraps the child so the
// caller can both wait for normal completion and interrupt (kill + wait) the
// child, which is required for correct signal handling.
type RemuxProcess struct {
	cmd    *exec.Cmd
	output *strings.Builder
	errC   chan error
}

// NewRemuxProcess wraps an already-started command and begins waiting for it in a
// dedicated goroutine. Exactly one goroutine calls cmd.Wait(), so the child is
// reaped exactly once. Separate from StartRemux to allow test injection of
// substitute commands.
func NewRemuxProcess(cmd *exec.Cmd) *RemuxProcess {
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	r := &RemuxProcess{cmd: cmd, output: &buf}
	r.errC = make(chan error, 1)
	go func() {
		r.errC <- cmd.Wait()
	}()
	return r
}

// StartRemux builds and starts the FFmpeg remux command. Call Wait() to
// obtain its final error, or Interrupt() to terminate and reap the child.
func StartRemux(inputVideo, inputMeta, output string, outputExt string, overwrite bool) (*RemuxProcess, error) {
	cmd := exec.Command("ffmpeg", BuildFFmpegArgs(inputVideo, inputMeta, output, outputExt, overwrite)...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("FFmpeg failed to start: %w", err)
	}
	return NewRemuxProcess(cmd), nil
}

// StartStrip builds and starts the FFmpeg chapter-stripping command (rmch).
// Unlike StartRemux it has no metadata input; it strips via -map_chapters -1.
// Call Wait()/Done() for the final error, or Interrupt() to kill and reap.
func StartStrip(inputVideo, output string, outputExt string, overwrite bool) (*RemuxProcess, error) {
	cmd := exec.Command("ffmpeg", BuildStripArgs(inputVideo, output, outputExt, overwrite)...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("FFmpeg failed to start: %w", err)
	}
	return NewRemuxProcess(cmd), nil
}

// Wait blocks until FFmpeg exits and returns a descriptive error on failure.
// It must be called at most once.
func (r *RemuxProcess) Wait() error {
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

// Done returns a channel that yields the remux result exactly once, when the
// child exits. It is useful for select-based waiting alongside a signal channel.
func (r *RemuxProcess) Done() <-chan error {
	return r.errC
}

// Interrupt asks the FFmpeg child to terminate and blocks until it has fully
// exited (reaped). It returns after the child has stopped, so callers can safely
// clean up the output file without racing a still-writing process.
func (r *RemuxProcess) Interrupt() {
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	_ = r.Wait()
}

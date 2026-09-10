package media

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

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
//
// The directory defaults to os.TempDir(); tests that assert on temp-file
// cleanup can override it via the ADDCH_METADATA_TMPDIR environment variable
// to keep the count hermetic (immune to concurrent test binaries sharing the
// system temp directory).
func WriteTempMetadata(content string) (string, error) {
	dir := os.Getenv("ADDCH_METADATA_TMPDIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "addch-metadata-*.txt")
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
	begin  sync.Once
	action string // human-readable verb for error messages (e.g. "remux", "strip")
}

// NewRemuxProcess wraps a command so its stdout and stderr are captured into a
// single buffer. The writers are attached HERE so that os/exec captures the
// child's output from the first byte: the buffer must be wired before the
// command is started, which START starts with. The child is reaped exactly once
// on the first Wait()/Done() call (or an Interrupt), so callers never race to
// call cmd.Wait() themselves. Separate from StartRemux to allow test injection
// of substitute commands.
func NewRemuxProcess(cmd *exec.Cmd) *RemuxProcess {
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	r := &RemuxProcess{cmd: cmd, output: &buf}
	r.errC = make(chan error, 1)
	return r
}

// StartRemux builds and starts the FFmpeg remux command. Call Wait() to
// obtain its final error, or Interrupt() to terminate and reap the child.
func StartRemux(inputVideo, inputMeta, output string, outputExt string, overwrite bool) (*RemuxProcess, error) {
	cmd := exec.Command("ffmpeg", BuildFFmpegArgs(inputVideo, inputMeta, output, outputExt, overwrite)...)
	rp := NewRemuxProcess(cmd)
	rp.action = "remux"
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("FFmpeg failed to start: %w", err)
	}
	return rp, nil
}

// StartStrip builds and starts the FFmpeg chapter-stripping command (rmch).
// Unlike StartRemux it has no metadata input; it strips via -map_chapters -1.
// Call Wait()/Done() for the final error, or Interrupt() to kill and reap.
func StartStrip(inputVideo, output string, outputExt string, overwrite bool) (*RemuxProcess, error) {
	cmd := exec.Command("ffmpeg", BuildStripArgs(inputVideo, output, outputExt, overwrite)...)
	rp := NewRemuxProcess(cmd)
	rp.action = "strip"
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("FFmpeg failed to start: %w", err)
	}
	return rp, nil
}

// startReaping launches the goroutine that waits for the child and produces the
// final (stderr-enriched) error. It is idempotent, so multiple Wait()/Done()
// calls never double-reap the child.
func (r *RemuxProcess) startReaping() {
	r.begin.Do(func() {
		go func() {
			werr := r.cmd.Wait()
			if werr == nil {
				r.errC <- nil
				return
			}
			msg := strings.TrimSpace(r.output.String())
			if msg == "" {
				msg = werr.Error()
			}
			r.errC <- fmt.Errorf("FFmpeg failed to %s the video: %s", r.action, msg)
		}()
	})
}

// Wait blocks until FFmpeg exits and returns a descriptive error on failure.
// The result is delivered exactly once: call Wait once, or consume Done once;
// a second call after the result was received would block on the empty channel.
func (r *RemuxProcess) Wait() error {
	r.startReaping()
	return <-r.errC
}

// Done returns a channel that yields the remux result exactly once, when the
// child exits. The error is the same stderr-enriched one produced by Wait, so
// select-based waiters (alongside a signal channel) receive the diagnostic too.
func (r *RemuxProcess) Done() <-chan error {
	r.startReaping()
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

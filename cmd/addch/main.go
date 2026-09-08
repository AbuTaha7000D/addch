package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/abutaha/addch/internal/batch"
	"github.com/abutaha/addch/internal/chapters"
	"github.com/abutaha/addch/internal/fsutil"
	"github.com/abutaha/addch/internal/media"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns a process exit code (0 for success).
func run(args []string, stdout, stderr io.Writer) int {
	pa, err := parseArgs(args, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// Short-circuit modes.
	if pa.help || pa.version {
		return 0
	}
	if pa.example {
		if werr := writeExampleFile(); werr != nil {
			fmt.Fprintf(stderr, "Error: %v\n", werr)
			return 1
		}
		return 0
	}
	if pa.check {
		return runCheck(stdout, stderr)
	}

	// Batch and single-file modes are mutually exclusive (guaranteed by
	// parseArgs), so the dispatch below is unambiguous.
	if pa.dir || pa.recursive {
		return runBatch(pa, stdout, stderr)
	}

	return runEmbed(pa, stdout, stderr)
}

func runCheck(stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "Checking dependencies...")
	fmt.Fprintln(stdout)
	di := media.CheckDependencies()
	for _, l := range di.ListReports() {
		fmt.Fprintln(stdout, l)
	}
	if di.Ready() {
		fmt.Fprintln(stdout, "\nSystem is ready.")
		return 0
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stderr, di.InstallHint())
	return 1
}

// runBatch drives batch mode: candidate discovery followed by sequential
// processing through internal/batch.Runner. One signal watcher is registered
// before the runner starts (so no signal is missed in the window before a child
// starts) and cancels the batch context; the per-item action bridges that
// cancellation to RemuxProcess.Interrupt() so an active FFmpeg is killed and
// reaped, its partial output and temp metadata removed.
func runBatch(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no changes yet).
	di := media.CheckDependencies()
	if !di.Ready() {
		fmt.Fprint(stderr, di.InstallHint())
		return 1
	}
	fmt.Fprintln(stdout, "✓ Dependencies found")

	// 2. Discover candidates.
	candidates, err := fsutil.FindCandidates(pa.dirPath, pa.recursive)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 3. Nothing to do.
	if len(candidates) == 0 {
		fmt.Fprintf(stdout, "No media files with a matching .txt sidecar found in %s.\n", pa.dirPath)
		return 0
	}

	items := make([]string, len(candidates))
	for i, c := range candidates {
		items[i] = c.MediaPath
	}

	// 4. Signal handling for the whole batch, registered once before any
	// processing begins. A single watcher goroutine records the signal (for the
	// 130/143 exit code) and cancels the batch context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)

	var interruptSig os.Signal
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case sig := <-interrupted:
			interruptSig = sig
			cancel()
		case <-stopWatcher:
		}
	}()

	// 5. Sequentially process every candidate.
	runner := batch.NewRunner()
	summary := runner.Run(ctx, items, func(ctx context.Context, item string) (batch.ItemStatus, error, string) {
		return runBatchAction(ctx, pa, item)
	})

	// Clear the signal channel and wait for the watcher to exit so interruptSig
	// is final before we map it to an exit code.
	signal.Stop(interrupted)
	close(stopWatcher)
	<-watcherDone

	// 6. Per-item report + summary.
	for _, res := range summary.Results {
		line := fmt.Sprintf("  [%s] %s", res.Status, res.Item)
		if res.Reason != "" {
			line += " (" + res.Reason + ")"
		}
		if res.Err != nil {
			line += ": " + res.Err.Error()
		}
		fmt.Fprintln(stdout, line)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, summary.String())

	// 7. Exit code: a caught signal wins over failure counts.
	if interruptSig != nil {
		if interruptSig == syscall.SIGTERM {
			return 143
		}
		return 130
	}
	if summary.Failed > 0 {
		return 1
	}
	return 0
}

// runBatchAction embeds one discovered media file using its exact sidecar. It
// implements the FFprobe validation step that discovery intentionally omits and
// owns the ctx-to-RemuxProcess signal bridge.
func runBatchAction(ctx context.Context, pa *parsedArgs, mediaPath string) (batch.ItemStatus, error, string) {
	sidecarPath := fsutil.SidecarPath(mediaPath)
	output := fsutil.DefaultOutputPath(mediaPath)

	// Skip existing outputs unless --overwrite. A directory at the output path
	// still counts as "exists" and is skipped; any stat error is "not present".
	if !pa.overwrite {
		if _, err := os.Stat(output); err == nil {
			return batch.StatusSkipped, nil, "output exists (use --overwrite to replace)"
		}
	}

	// FFprobe validation (findCandidates deliberately does not probe).
	durationMs, err := media.GetVideoDurationMs(mediaPath)
	if err != nil {
		return batch.StatusFailed, err, ""
	}

	chs, err := chapters.ParseFile(sidecarPath)
	if err != nil {
		return batch.StatusFailed, err, ""
	}
	if err := chapters.Validate(chs); err != nil {
		return batch.StatusFailed, err, ""
	}
	if err := chapters.ValidateDuration(chs, durationMs); err != nil {
		return batch.StatusFailed, err, ""
	}

	meta := media.BuildMetadata(chs, durationMs)
	metaPath, err := media.WriteTempMetadata(meta)
	if err != nil {
		return batch.StatusFailed, err, ""
	}

	// Don't spawn a doomed child if cancellation already happened between the
	// runner's ctx check and now.
	select {
	case <-ctx.Done():
		os.Remove(metaPath)
		return batch.StatusSkipped, nil, "cancelled"
	default:
	}

	outputExt := fsutil.OutputExtension(output)
	rp, err := media.StartRemux(mediaPath, metaPath, output, outputExt, pa.overwrite)
	if err != nil {
		os.Remove(metaPath)
		return batch.StatusFailed, err, ""
	}

	// Bridge: a cancelled context during an active remux kills and reaps the
	// child, then removes the partial output and temp metadata. A context
	// cancel alone would never interrupt the OS child (batch has no knowledge
	// of RemuxProcess), so this action owns the bridge.
	select {
	case <-ctx.Done():
		rp.Interrupt()
		fsutil.CleanupFile(output)
		os.Remove(metaPath)
		return batch.StatusSkipped, nil, "cancelled"
	case err := <-rp.Done():
		os.Remove(metaPath) // always clean up temp metadata
		if err != nil {
			fsutil.CleanupFile(output)
			return batch.StatusFailed, err, ""
		}
	}

	if err := media.VerifyChapters(output, chs, durationMs); err != nil {
		fsutil.CleanupFile(output)
		return batch.StatusFailed, err, ""
	}
	return batch.StatusSuccess, nil, ""
}

func runEmbed(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no modifications yet).
	di := media.CheckDependencies()
	if !di.Ready() {
		fmt.Fprint(stderr, di.InstallHint())
		return 1
	}
	fmt.Fprintln(stdout, "✓ Dependencies found")

	// 2. Parse chapters.
	chs, err := chapters.ParseFile(pa.chapters)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 3. Structural validation.
	if err := chapters.Validate(chs); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "✓ Chapters validated")

	// 4. Input video existence.
	if _, err := os.Stat(pa.video); err != nil {
		fmt.Fprintf(stderr, "Error: video file not found: %q\n", pa.video)
		return 1
	}
	if fi, err := os.Stat(pa.video); err == nil && fi.IsDir() {
		fmt.Fprintf(stderr, "Error: %q is a directory, not a video file\n", pa.video)
		return 1
	}

	// 5. Determine output path.
	output := pa.output
	if output == "" {
		output = fsutil.DefaultOutputPath(pa.video)
	}

	// Guard against overwriting the video input itself.
	if fsutil.SamePath(output, pa.video) {
		fmt.Fprintf(stderr, "Error: output path %q would overwrite the input video; choose a different --output\n", output)
		return 1
	}
	// Guard against overwriting the user's chapter file (another input).
	if fsutil.SamePath(output, pa.chapters) {
		fmt.Fprintf(stderr, "Error: output path %q would overwrite the chapter file; choose a different --output\n", output)
		return 1
	}

	// 6. Refuse to overwrite existing output unless --overwrite.
	if _, err := os.Stat(output); err == nil && !pa.overwrite {
		fmt.Fprintf(stderr,
			"Error: output file already exists: %q\nUse --overwrite to replace it.\n", output)
		return 1
	}

	// 7. Get video duration.
	durationMs, err := media.GetVideoDurationMs(pa.video)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 8. Validate chapters against duration.
	if err := chapters.ValidateDuration(chs, durationMs); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "✓ Video duration checked")

	// 9. Generate metadata and remux.
	meta := media.BuildMetadata(chs, durationMs)
	metaPath, err := media.WriteTempMetadata(meta)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	outputExt := fsutil.OutputExtension(output)

	// 9b. Run the remux with signal handling. The signal channel is registered
	// BEFORE the child FFmpeg process is started. This is required: if a
	// SIGINT/SIGTERM arrived in the window after the child starts but before
	// signal.Notify is registered, Go's default handler would terminate addch
	// immediately without reaping the child, leaving an orphaned FFmpeg running.
	// With the channel live up front, any such signal is routed to our handler,
	// which kills the child, waits for it to fully exit (reaping it exactly
	// once), then removes the partial output and temp metadata — never leaving
	// an orphan process or racing the writer.
	fmt.Fprintln(stdout, "→ Embedding chapters...")

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)

	rp, err := media.StartRemux(pa.video, metaPath, output, outputExt, pa.overwrite)
	if err != nil {
		signal.Stop(interrupted)
		os.Remove(metaPath)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	select {
	case sig := <-interrupted:
		rp.Interrupt() // kill, then wait for the child to actually stop
		signal.Stop(interrupted)
		fsutil.CleanupFile(output)
		os.Remove(metaPath)
		fmt.Fprintln(stderr, "\nInterrupted; no output was written.")
		// Follow the conventional shell exit codes for a caught signal:
		// SIGINT -> 130 (128+2), SIGTERM -> 143 (128+15).
		if sig == syscall.SIGTERM {
			return 143
		}
		return 130
	case err = <-rp.Done():
		signal.Stop(interrupted)
	}

	os.Remove(metaPath) // always clean up temp metadata
	if err != nil {
		fsutil.CleanupFile(output)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 10. Verify.
	if err := media.VerifyChapters(output, chs, durationMs); err != nil {
		fsutil.CleanupFile(output)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "✓ Chapters embedded successfully")
	fmt.Fprintln(stdout, "✓ Verification passed")
	fmt.Fprintf(stdout, "\nOutput:\n%s\n", output)
	return 0
}

func writeExampleFile() error {
	name := "example_chapters.txt"
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("example file %q already exists in the current directory", name)
	}
	if err := os.WriteFile(name, []byte(exampleContent), 0o644); err != nil {
		return fmt.Errorf("could not write example file %q: %w", name, err)
	}
	fmt.Printf("Wrote %s\n", name)
	return nil
}

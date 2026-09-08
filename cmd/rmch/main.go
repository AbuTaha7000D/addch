package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/abutaha/addch/internal/batch"
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
	if pa.check {
		return runCheck(stdout, stderr)
	}

	// Batch and single-file modes are mutually exclusive (guaranteed by
	// parseArgs), so the dispatch below is unambiguous.
	if pa.dir || pa.recursive {
		return runBatch(pa, stdout, stderr)
	}

	return runRemoval(pa, stdout, stderr)
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

// runRemoval strips all chapter markers from a single media file. It follows
// the same ordering, wording, and exit-code rules as addch's runEmbed, applied
// to the remove side: dependencies first, then input existence, output
// resolution, same-path guard, existing-output guard, duration probe, and a
// no-chapters refusal (never emit a redundant identical copy). The signal
// channel is registered BEFORE the FFmpeg child starts; on a signal the child
// is killed and reaped exactly once, and the partial output is removed.
func runRemoval(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no modifications yet).
	di := media.CheckDependencies()
	if !di.Ready() {
		fmt.Fprint(stderr, di.InstallHint())
		return 1
	}
	fmt.Fprintln(stdout, "✓ Dependencies found")

	// 2. Input video existence.
	if _, err := os.Stat(pa.video); err != nil {
		fmt.Fprintf(stderr, "Error: video file not found: %q\n", pa.video)
		return 1
	}
	if fi, err := os.Stat(pa.video); err == nil && fi.IsDir() {
		fmt.Fprintf(stderr, "Error: %q is a directory, not a video file\n", pa.video)
		return 1
	}

	// 3. Determine output path.
	output := pa.output
	if output == "" {
		output = fsutil.DefaultNoChaptersOutputPath(pa.video)
	}

	// 4. Guard against overwriting the input video itself.
	if fsutil.SamePath(output, pa.video) {
		fmt.Fprintf(stderr, "Error: output path %q would overwrite the input video; choose a different --output\n", output)
		return 1
	}

	// 5. Refuse to overwrite existing output unless --overwrite.
	if _, err := os.Stat(output); err == nil && !pa.overwrite {
		fmt.Fprintf(stderr,
			"Error: output file already exists: %q\nUse --overwrite to replace it.\n", output)
		return 1
	}

	// 6. Probe duration (also validates that the input is real, probed media).
	if _, err := media.GetVideoDurationMs(pa.video); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 7. Refuse to process inputs that have no chapters to remove; this avoids
	// producing a redundant byte-identical -nochapters copy.
	count, err := media.CountChapters(pa.video)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if count == 0 {
		fmt.Fprintf(stderr, "Error: %q has no chapters to remove.\n", pa.video)
		return 1
	}

	// 8. Run the strip with signal handling. The signal channel is registered
	// BEFORE the child FFmpeg process is started (same rationale as addch: a
	// signal landing between start and Notify would terminate us without
	// reaping the child, orphaning FFmpeg).
	fmt.Fprintln(stdout, "→ Removing chapters...")

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)

	outputExt := fsutil.OutputExtension(output)
	rp, err := media.StartStrip(pa.video, output, outputExt, pa.overwrite)
	if err != nil {
		signal.Stop(interrupted)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	select {
	case sig := <-interrupted:
		rp.Interrupt() // kill, then wait for the child to actually stop
		signal.Stop(interrupted)
		fsutil.CleanupFile(output)
		fmt.Fprintln(stderr, "\nInterrupted; no output was written.")
		if sig == syscall.SIGTERM {
			return 143
		}
		return 130
	case err = <-rp.Done():
		signal.Stop(interrupted)
	}

	if err != nil {
		fsutil.CleanupFile(output)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 9. Verify the output has zero chapters; remove invalid output on failure.
	if err := media.VerifyNoChapters(output); err != nil {
		fsutil.CleanupFile(output)
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "✓ Chapters removed successfully")
	fmt.Fprintln(stdout, "✓ Verification passed")
	fmt.Fprintf(stdout, "\nOutput:\n%s\n", output)
	return 0
}

// runBatch drives batch mode: media discovery followed by sequential processing
// through internal/batch.Runner. One signal watcher is registered before the
// runner starts (so no signal is missed in the window before a child starts)
// and cancels the batch context; the per-item action bridges that cancellation
// to RemuxProcess.Interrupt() so an active FFmpeg is killed and reaped, its
// partial output removed.
func runBatch(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no changes yet).
	di := media.CheckDependencies()
	if !di.Ready() {
		fmt.Fprint(stderr, di.InstallHint())
		return 1
	}
	fmt.Fprintln(stdout, "✓ Dependencies found")

	// 2. Discover media files (rmch has no sidecar concept).
	files, err := fsutil.FindMediaFiles(pa.dirPath, pa.recursive)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 3. Nothing to do.
	if len(files) == 0 {
		fmt.Fprintf(stdout, "No media files found in %s.\n", pa.dirPath)
		return 0
	}

	// 4. Signal handling for the whole batch, registered once before any
	// processing begins.
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

	// 5. Sequentially process every discovered file.
	runner := batch.NewRunner()
	summary := runner.Run(ctx, files, func(ctx context.Context, item string) (batch.ItemStatus, error, string) {
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

// runBatchAction removes chapters from one discovered media file. It implements
// the per-item checks (existing output, duration probe, no-chapters skip) and
// owns the ctx-to-RemuxProcess signal bridge so a cancel kills+reaps the child
// and removes the partial output.
func runBatchAction(ctx context.Context, pa *parsedArgs, mediaPath string) (batch.ItemStatus, error, string) {
	output := fsutil.DefaultNoChaptersOutputPath(mediaPath)

	// Skip existing outputs unless --overwrite. A directory at the output path
	// still counts as "exists" and is skipped; any stat error is "not present".
	if !pa.overwrite {
		if _, err := os.Stat(output); err == nil {
			return batch.StatusSkipped, nil, "output exists (use --overwrite to replace)"
		}
	}

	// FFprobe duration validation (discovery deliberately does not probe).
	if _, err := media.GetVideoDurationMs(mediaPath); err != nil {
		return batch.StatusFailed, err, ""
	}

	// Skip inputs with no chapters to remove, so the batch does not emit a
	// redundant identical copy.
	count, err := media.CountChapters(mediaPath)
	if err != nil {
		return batch.StatusFailed, err, ""
	}
	if count == 0 {
		return batch.StatusSkipped, nil, "no chapters to remove"
	}

	// Don't spawn a doomed child if cancellation already happened between the
	// runner's ctx check and now.
	select {
	case <-ctx.Done():
		return batch.StatusSkipped, nil, "cancelled"
	default:
	}

	outputExt := fsutil.OutputExtension(output)
	rp, err := media.StartStrip(mediaPath, output, outputExt, pa.overwrite)
	if err != nil {
		return batch.StatusFailed, err, ""
	}

	// Bridge: a cancelled context during an active strip kills and reaps the
	// child, then removes the partial output.
	select {
	case <-ctx.Done():
		rp.Interrupt()
		fsutil.CleanupFile(output)
		return batch.StatusSkipped, nil, "cancelled"
	case err := <-rp.Done():
		if err != nil {
			fsutil.CleanupFile(output)
			return batch.StatusFailed, err, ""
		}
	}

	if err := media.VerifyNoChapters(output); err != nil {
		fsutil.CleanupFile(output)
		return batch.StatusFailed, err, ""
	}
	return batch.StatusSuccess, nil, ""
}

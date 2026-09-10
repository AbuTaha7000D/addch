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
// stdout receives only chapter data (or the output of the interactive flags);
// every progress line, report, and error goes to stderr.
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

	return runExtract(pa, stdout, stderr)
}

func runCheck(stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "Checking dependencies...")
	fmt.Fprintln(stdout)
	di := media.CheckDependencies()
	for _, l := range di.ListReports() {
		fmt.Fprintln(stdout, l)
	}
	if di.ReadyFFprobe() {
		fmt.Fprintln(stdout, "\nSystem is ready.")
		return 0
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stderr, di.FFprobeInstallHint())
	return 1
}

// runExtract extracts the chapters of a single media file. It follows the same
// ordering and exit-code rules as addch/rmch's single-file paths — dependencies
// first, then input existence, then (only when a file output is requested)
// same-path and existing-output guards — but probe-first for the data it needs.
// In stdout mode the chapters are printed verbatim; with --output they are
// written atomically. A file with no chapters is a success that produces no
// output. The signal channel is registered BEFORE the atomic write; because the
// write runs to completion synchronously, its temporary file is always cleaned
// up (renamed or removed) before we act on the signal, so no interruption can
// orphan a partial artifact.
func runExtract(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no modifications yet). getch only needs
	// ffprobe: it reads chapters and never runs ffmpeg.
	di := media.CheckDependencies()
	if !di.ReadyFFprobe() {
		fmt.Fprint(stderr, di.FFprobeInstallHint())
		return 1
	}
	fmt.Fprintln(stderr, "✓ Dependencies found")

	// 2. Input media existence.
	if _, err := os.Stat(pa.video); err != nil {
		fmt.Fprintf(stderr, "Error: video file not found: %q\n", pa.video)
		return 1
	}
	if fi, err := os.Stat(pa.video); err == nil && fi.IsDir() {
		fmt.Fprintf(stderr, "Error: %q is a directory, not a video file\n", pa.video)
		return 1
	}

	// 3. stdout mode: the chapters are printed and stderr carries only progress.
	if pa.output == "" {
		fmt.Fprintln(stderr, "→ Extracting chapters...")
		chs, err := media.ProbeChapters(pa.video)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		// A file with no chapters is a success with empty stdout (and therefore
		// no file to write in batch mode either).
		if len(chs) == 0 {
			return 0
		}
		// A failed writer (e.g. a closed pipe or full disk) must surface as an
		// error: the diagnostic goes to stderr, stdout receives nothing more,
		// and the exit code is nonzero so consumers cannot mistake the partial
		// output for success.
		if err := chapters.Write(stdout, chs); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}

	// 4. File-output mode: guards first, then the probe.
	output := pa.output

	// 4a. Guard against overwriting the input media itself.
	if fsutil.SamePath(output, pa.video) {
		fmt.Fprintf(stderr, "Error: output path %q would overwrite the input video; choose a different --output\n", output)
		return 1
	}

	// 4b. Refuse to overwrite an existing output unless --overwrite.
	if _, err := os.Stat(output); err == nil && !pa.overwrite {
		fmt.Fprintf(stderr,
			"Error: output file already exists: %q\nUse --overwrite to replace it.\n", output)
		return 1
	}

	fmt.Fprintln(stderr, "→ Extracting chapters...")
	chs, err := media.ProbeChapters(pa.video)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if len(chs) == 0 {
		// A file with no chapters is a success, but no empty file is created.
		fmt.Fprintf(stderr, "No chapters found in %q; nothing written.\n", pa.video)
		return 0
	}

	// 5. Atomic write with signal handling. The channel is registered before the
	// write; the write itself is synchronous, so by the time we inspect the
	// channel the temporary file has either been renamed into place or removed.
	// On a signal the target (if it was about to be replaced) is removed and we
	// report the canonical interruption exit code.
	fmt.Fprintln(stderr, "→ Writing chapters...")
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)

	writeErr := fsutil.WriteFileAtomic(output, []byte(chapters.Format(chs)))

	var interruptSig os.Signal
	select {
	case interruptSig = <-interrupted:
	default:
	}
	signal.Stop(interrupted)

	if interruptSig != nil {
		fsutil.CleanupFile(output)
		fmt.Fprintln(stderr, "\nInterrupted; no output was written.")
		if interruptSig == syscall.SIGTERM {
			return 143
		}
		return 130
	}
	if writeErr != nil {
		fmt.Fprintf(stderr, "Error: %v\n", writeErr)
		return 1
	}

	// 6. Success: stdout stays empty in file-output mode, and the confirmation
	// goes to stderr so the chapter data (if any) is never mixed with it.
	fmt.Fprintf(stderr, "✓ Chapters written to %s\n", output)
	return 0
}

// runBatch drives batch mode: media discovery followed by sequential processing
// through internal/batch.Runner. One signal watcher is registered before the
// runner starts (so no signal is missed in the window before the first item)
// and cancels the batch context; cancelled items are reported as skipped and
// the process exits 130/143. Chapter data is never written to stdout; the
// per-item report and totals go to stderr.
func runBatch(pa *parsedArgs, stdout, stderr io.Writer) int {
	// 1. Dependency check (fail fast, no changes yet). getch only needs
	// ffprobe: it reads chapters and never runs ffmpeg.
	di := media.CheckDependencies()
	if !di.ReadyFFprobe() {
		fmt.Fprint(stderr, di.FFprobeInstallHint())
		return 1
	}
	fmt.Fprintln(stderr, "✓ Dependencies found")

	// 2. Discover media files. getch, like rmch, has no sidecar requirement for
	// discovery: every file is a candidate for extraction, and the sidecar only
	// appears as the output.
	files, err := fsutil.FindMediaFiles(pa.dirPath, pa.recursive)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// 3. Nothing to do.
	if len(files) == 0 {
		fmt.Fprintf(stderr, "No media files found in %s.\n", pa.dirPath)
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

	// 6. Per-item report + summary on stderr (stdout stays empty).
	for _, res := range summary.Results {
		line := fmt.Sprintf("  [%s] %s", res.Status, res.Item)
		if res.Reason != "" {
			line += " (" + res.Reason + ")"
		}
		if res.Err != nil {
			line += ": " + res.Err.Error()
		}
		fmt.Fprintln(stderr, line)
	}
	fmt.Fprintln(stderr)
	fmt.Fprintln(stderr, summary.String())

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

// runBatchAction extracts chapters from one discovered media file into its
// sidecar ("<name>.txt"). A pre-existing sidecar is skipped unless --overwrite,
// and a file with no chapters is skipped without creating an empty sidecar. The
// sidecar is written atomically so a reader never sees a partial file; a
// cancelled context is honored before the write, so interrupted items cannot
// leave a partial artifact.
func runBatchAction(ctx context.Context, pa *parsedArgs, mediaPath string) (batch.ItemStatus, error, string) {
	sidecar := fsutil.SidecarPath(mediaPath)

	// Skip existing sidecars unless --overwrite. A directory at the sidecar path
	// still counts as "exists" and is skipped; any stat error is "not present".
	if !pa.overwrite {
		if _, err := os.Stat(sidecar); err == nil {
			return batch.StatusSkipped, nil, "output exists (use --overwrite to replace)"
		}
	}

	chs, err := media.ProbeChapters(mediaPath)
	if err != nil {
		return batch.StatusFailed, err, ""
	}
	if len(chs) == 0 {
		// No empty sidecar for chapterless media.
		return batch.StatusSkipped, nil, "no chapters"
	}

	// Don't write a doomed sidecar if cancellation already happened between the
	// runner's ctx check and now.
	select {
	case <-ctx.Done():
		return batch.StatusSkipped, nil, "cancelled"
	default:
	}

	if err := fsutil.WriteFileAtomic(sidecar, []byte(chapters.Format(chs))); err != nil {
		return batch.StatusFailed, err, ""
	}
	return batch.StatusSuccess, nil, ""
}

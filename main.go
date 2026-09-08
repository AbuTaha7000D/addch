package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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

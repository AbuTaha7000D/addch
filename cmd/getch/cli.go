package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// version is set at build time via -ldflags "-X main.version=...". getch is a
// separate package from cmd/addch and cmd/rmch, so its main.version symbol does
// not clash.
var version = "dev"

// usageText documents the getch CLI. The stdout/stderr split is part of getch's
// contract: chapter data is the only thing ever written to stdout, so it can be
// piped directly into a file or another program; every diagnostic (progress,
// reports, errors) goes to stderr. The exceptions are --check, --version, and
// --help, which are interactive by nature and legitimately use stdout.
const usageText = `getch — extract chapters from a video/audio file without re-encoding.

Usage:
  getch [options] <media-file>

Extract chapter markers from <media-file> using FFprobe. The media file is
never modified. Chapters are written to stdout (or to a file with -o), one
"HH:MM:SS[.mmm] Title" line per chapter, in the order FFprobe reports them.
A file with no chapters produces empty stdout and exits 0. Diagnostics and
per-item batch reports go to stderr; stdout carries only chapter data.

Options:
  -o, --output <file>  Custom output path, written atomically (default: stdout)
      --overwrite      Replace the output file if it already exists
      --dir            Batch mode: extract chapters for each media file directly inside a directory
      --recursive      Batch mode: extract chapters for a directory and all nested subdirectories
      --check          Verify ffmpeg/ffprobe availability and exit
      --version        Print version and exit
  -h, --help           Print this help and exit

In batch mode each media file's chapters are written next to it as
"<name>.txt" (the addch sidecar), except when the file has no chapters or a
sidecar already exists.

Examples:
  getch "My Course.mp4"
  getch -o chapters.txt "My Course.mp4"
  getch --dir ./videos
  getch --recursive ./videos
`

type parsedArgs struct {
	check     bool
	version   bool
	help      bool
	overwrite bool
	dir       bool
	recursive bool
	dirPath   string
	output    string
	video     string
}

// parseArgs parses the command-line arguments. It returns the parsed struct or
// an error. Errors related to --help/--version/--check short-circuit and are
// flagged via the returned struct fields.
func parseArgs(args []string, stdout, stderr io.Writer) (*parsedArgs, error) {
	fs := flag.NewFlagSet("getch", flag.ContinueOnError)
	// The flag package prints the raw parse error ("flag provided but not
	// defined: ...") itself on ContinueOnError; discard that so run()'s single
	// "Error: ..." line is the only stderr line for an unknown flag.
	fs.SetOutput(io.Discard)
	// Help text is written to stdout by the success paths below; the flag
	// package's default Usage (used for -h and unknown flags) also invokes this,
	// so make it a no-op here to avoid duplicate output.
	fs.Usage = func() {}

	check := fs.Bool("check", false, "")
	versionFlag := fs.Bool("version", false, "")
	help := fs.Bool("help", false, "")
	overwrite := fs.Bool("overwrite", false, "")
	dir := fs.Bool("dir", false, "")
	recursive := fs.Bool("recursive", false, "")
	output := fs.String("output", "", "")
	fs.StringVar(output, "o", "", "")

	if err := fs.Parse(args); err != nil {
		// -h/-help are not registered flags; the flag package reports flag.ErrHelp
		// for them. Treat them exactly like --help: print usage and exit 0, without
		// the "flag: help requested" error that would otherwise be surfaced.
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usageText)
			return &parsedArgs{help: true}, nil
		}
		return nil, err
	}

	pa := &parsedArgs{
		check:     *check,
		version:   *versionFlag,
		help:      *help,
		overwrite: *overwrite,
		dir:       *dir,
		recursive: *recursive,
		output:    *output,
	}

	if pa.help {
		fmt.Fprint(stdout, usageText)
		return pa, nil
	}
	if pa.version {
		fmt.Fprintf(stdout, "getch %s\n", version)
		return pa, nil
	}
	if pa.check {
		return pa, nil
	}

	rest := fs.Args()

	// Batch-mode validation. These rules run before the single-file positional
	// argument logic and only apply when a batch flag was given.
	switch {
	case pa.dir && pa.recursive:
		return nil, fmt.Errorf("--dir and --recursive are mutually exclusive; use exactly one.")
	case (pa.dir || pa.recursive) && pa.output != "":
		return nil, fmt.Errorf("--output cannot be combined with --dir/--recursive batch mode.")
	case pa.dir || pa.recursive:
		switch {
		case len(rest) == 0:
			return nil, fmt.Errorf("batch mode requires a directory argument.")
		case len(rest) != 1:
			return nil, fmt.Errorf("batch mode takes exactly one directory argument (got %d): %v", len(rest), rest)
		}
		pa.dirPath = rest[0]
		return pa, nil
	}

	if len(rest) == 0 {
		fmt.Fprint(stderr, usageText)
		return nil, fmt.Errorf("missing required arguments: <video-file>")
	}
	if len(rest) > 1 {
		return nil, fmt.Errorf("too many arguments: expected <video-file>, got %d", len(rest))
	}
	// --overwrite is only meaningful with a writable output target. In single-file
	// mode the default output is stdout, which cannot be overwritten, so combining
	// the two without --output is a usage error (in batch mode the sidecar is the
	// target and --overwrite is valid; that branch returned above).
	if pa.overwrite && pa.output == "" {
		return nil, fmt.Errorf("--overwrite requires --output in single-file mode (stdout cannot be overwritten).")
	}
	pa.video = rest[0]
	return pa, nil
}

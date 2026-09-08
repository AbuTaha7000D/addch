package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

//go:embed example_chapters.txt
var exampleContent string

const usageText = `addch — embed chapters into a video without re-encoding.

Usage:
  addch [options] <chapters-file> <video-file>

Embed chapters defined in <chapters-file> into <video-file> using FFmpeg
stream copy (no re-encoding). The original video is never modified.

Arguments:
  chapters-file    Path to a text file with one chapter per line:
                   HH:MM:SS Chapter title  (or HH:MM:SS.mmm)
  video-file       Path to the input video.

Options:
  -o, --output <file>  Custom output path (default: <video>-chapters.<ext>)
      --overwrite      Overwrite the output file if it already exists
      --dir            Batch mode: process the media files directly inside a directory
      --recursive      Batch mode: process a directory and all nested subdirectories
      --example        Write example_chapters.txt and exit
      --check          Verify ffmpeg/ffprobe availability and exit
      --version        Print version and exit
  -h, --help           Print this help and exit

Examples:
  addch chapters.txt "My Course.mp4"
  addch --dir ./videos
  addch --recursive ./videos
  addch --example
`

type parsedArgs struct {
	example   bool
	check     bool
	version   bool
	help      bool
	overwrite bool
	dir       bool
	recursive bool
	output    string
	chapters  string
	video     string
}

// parseArgs parses the command-line arguments. It returns the parsed struct or
// an error. Errors related to --help/--version/--example/--check short-circuit
// and are flagged via the returned struct fields.
func parseArgs(args []string, stdout, stderr io.Writer) (*parsedArgs, error) {
	fs := flag.NewFlagSet("addch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Help text is written to stdout by the success paths below; the flag
	// package's default Usage (used for -h and unknown flags) also invokes this,
	// so make it a no-op here to avoid duplicate output.
	fs.Usage = func() {}

	example := fs.Bool("example", false, "")
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
		example:   *example,
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
		fmt.Fprintf(stdout, "addch %s\n", version)
		return pa, nil
	}
	if pa.example || pa.check {
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
		if len(rest) != 0 {
			return nil, fmt.Errorf("batch mode takes no positional arguments (got %d): %v", len(rest), rest)
		}
		return pa, nil
	}

	if len(rest) < 2 {
		fmt.Fprint(stderr, usageText)
		return nil, fmt.Errorf("missing required arguments: <chapters-file> and <video-file>")
	}
	if len(rest) > 2 {
		return nil, fmt.Errorf("too many arguments: expected <chapters-file> and <video-file>, got %d", len(rest))
	}
	pa.chapters = rest[0]
	pa.video = rest[1]
	return pa, nil
}

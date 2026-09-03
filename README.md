# addch

Embed custom chapters into video files without re-encoding.

`addch` reads a simple text file of chapter definitions, validates it, generates
the required FFmpeg metadata internally, and remuxes the video using stream copy
so the quality of the original video is preserved. The original video is never
modified.

## Requirements

- **FFmpeg** and **FFprobe** available in your `PATH`.

Install them if needed:

| Platform | Command |
| --- | --- |
| Fedora / RHEL / CentOS | `sudo dnf install ffmpeg` |
| Debian / Ubuntu | `sudo apt install ffmpeg` |
| Arch | `sudo pacman -S ffmpeg` |
| openSUSE | `sudo zypper install ffmpeg` |
| macOS (Homebrew) | `brew install ffmpeg` |
| Windows (Winget) | `winget install --id Gyan.FFmpeg --exact` |

> After installing, make sure both the `ffmpeg` and `ffprobe` programs are
> available in your `PATH`. Do not rename the FFmpeg binaries — keep their
> original names. You can confirm with `addch --check`.

## Installation

### Prebuilt binaries

Download the release binary for your platform from the
[Releases page](https://github.com/abutaha/addch/releases). You do **not** need to
install Go to use a prebuilt binary — just download the matching file for your
system, make it executable (macOS/Linux), and run it.

| Platform            | Binary                    |
| ------------------- | ------------------------- |
| Linux x86_64        | `addch-linux-amd64`       |
| Linux ARM64         | `addch-linux-arm64`       |
| macOS Intel         | `addch-darwin-amd64`      |
| macOS Apple Silicon | `addch-darwin-arm64`      |
| Windows x86_64      | `addch-windows-amd64.exe` |
| Windows ARM64       | `addch-windows-arm64.exe` |

You still need `ffmpeg` and `ffprobe` available in your `PATH` (see
[Requirements](#requirements)); the addch binary itself is self-contained.

Release assets include a `SHA256SUMS.txt` file so you can verify the checksum of
any downloaded binary.

### Build from source

```sh
make build        # produces ./addch
make install      # go install
```


## Usage

```sh
addch chapters.txt "My Course.mp4"
```

This creates `My Course-chapters.mp4` in the same directory, with the chapters
embedded, leaving the original untouched.

### Chapter file format

One chapter per line:

```
HH:MM:SS Chapter title
```

Millisecond precision is also supported (mixed freely in one file):

```
HH:MM:SS.mmm Chapter title
```

Empty lines are ignored. Both LF and CRLF line endings are accepted, as is a
UTF-8 BOM.

Example:

```
00:00:00 Introduction
00:05:30 Setting Up
00:18:45 Core Concepts
00:42:10 Advanced Topics
01:15:00 Wrap-Up
```

Generate a valid example to get started:

```sh
addch --example     # writes example_chapters.txt
```

### Options

```
Usage: addch [options] <chapters-file> <video-file>

  -o, --output <file>   Custom output path (default: <video>-chapters.<ext>)
      --overwrite       Overwrite the output file if it already exists
      --example         Write example_chapters.txt and exit
      --check           Verify ffmpeg/ffprobe availability and exit
      --version         Print version and exit
  -h, --help            Print help and exit
```

### Examples

```sh
# Basic usage: writes "video-chapters.mp4"
addch chapters.txt video.mp4

# Custom output path
addch -o output.mp4 chapters.txt video.mp4

# Overwrite an existing output file
addch --overwrite chapters.txt video.mp4

# Write a sample chapter file to get started
addch --example

# Check that ffmpeg/ffprobe are installed and on PATH
addch --check

# Print the version
addch --version
```

Note: options must be placed before the two positional arguments
(`<chapters-file>` and `<video-file>`), e.g. `addch --overwrite chapters.txt
video.mp4`, not after them.

Paths containing spaces or Unicode work in all of the above; just quote them
if your shell requires it (e.g. `addch chapters.txt "My Course.mp4"`).

### Example session

```sh
$ addch chapters.txt "My Course.mp4"
✓ Dependencies found
✓ Chapters validated
✓ Video duration checked
→ Embedding chapters...
✓ Chapters embedded successfully
✓ Verification passed

Output:
My Course-chapters.mp4
```

## How it works

1. Validate the chapter file and the video.
2. Read the video duration with `ffprobe`.
3. Generate an `FFMETADATA1` file internally (no manual FFmpeg syntax needed).
4. Remux with stream copy (no re-encoding) and embed the chapters:

   ```sh
   ffmpeg -i input -i chapters.meta \
     -map 0 -map_metadata 0 -map_chapters 1 \
     -c copy [-movflags +faststart] output
   ```

5. Verify the result with `ffprobe` (chapter count, titles, start and end times).

`+faststart` is added for MP4-family containers; it is omitted for others.

## Validation

`addch` fails before touching your video when it detects problems such as:

- invalid timestamp format
- missing or invalid chapter title
- chapters not in chronological order
- duplicate timestamps
- a first chapter that does not start at `00:00:00`
- a chapter that starts after the video ends
- the output file already exists (use `--overwrite`)
- missing FFmpeg / FFprobe

## Development

```sh
make test    # unit + integration tests
make vet     # static analysis
make dist    # cross-compile all platforms into ./dist
```

Integration tests require `ffmpeg` and `ffprobe`; they are skipped when those
tools are unavailable. CI runs unit and integration tests on Linux, macOS, and
Windows.

## License

`addch` is released under the [MIT License](LICENSE).

FFmpeg and FFprobe are **external dependencies** of `addch`. They are not bundled
with or linked into `addch`; you install them separately. `ffprobe` ships as part
of standard FFmpeg distributions, so installing FFmpeg normally provides both
executables. FFmpeg and FFprobe have their own licensing terms, which depend on
how they are built/configured (FFmpeg can be built under the LGPL or GPL,
depending on the enabled components). Refer to FFmpeg's own documentation for
details on its license.

## Author

addch is maintained by [Mahmoud Ehab](https://github.com/AbuTaha7000D)
(eng.mahmoud.e.hussein@gmail.com).

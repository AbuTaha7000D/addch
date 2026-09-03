# addch

Add custom chapters to your videos without re-encoding.

`addch` reads a simple text file of chapter definitions, validates it, generates
the required FFmpeg metadata internally, and remuxes your video using **stream
copy** (no re-encoding). The quality of the original video is preserved, and the
original file is **never modified** — `addch` always writes a new output file.

---

## Quick Start

This walkthrough assumes you have never used `addch` before. Follow the steps in
order and you will embed chapters into your first video in a few minutes.

**Step 1 — Install FFmpeg**

`addch` needs two helper programs, `ffmpeg` and `ffprobe`, installed on your
computer. Installing `ffmpeg` normally provides both. See [Requirements](#requirements)
for exact commands for your operating system.

**Step 2 — Download the right `addch` binary**

Go to the [Releases page](https://github.com/AbuTaha7000D/addch/releases), open
the latest release (for example `v0.1.2`), and download the file that matches
your operating system and architecture (CPU type).

Pick your row from this table:

| Your operating system | Download this file |
| --------------------- | ------------------ |
| **Linux** — Intel/AMD 64-bit PC or server (most common) | `addch-linux-amd64` |
| **Linux** — 64-bit ARM (e.g. many Raspberry Pi models) | `addch-linux-arm64` |
| **macOS** — Intel processor | `addch-darwin-amd64` |
| **macOS** — Apple Silicon (M1, M2, M3, M4) | `addch-darwin-arm64` |
| **Windows** — Intel/AMD 64-bit PC | `addch-windows-amd64.exe` |
| **Windows** — 64-bit ARM | `addch-windows-arm64.exe` |

Don't worry if you aren't sure what `amd64` or `arm64` means:

- **amd64** (also called `x86_64`) is the CPU type used by the vast majority of
  PCs and laptops — Intel and AMD 64-bit processors.
- **arm64** is the CPU type used by Apple Silicon Macs (M1 and newer) and by many
  smaller/ARM devices such as the Raspberry Pi.

If in doubt on a normal PC, choose the **`amd64`** file for your operating system.

**Step 3 — Install and run the binary**

On **Linux / macOS**, the downloaded file is a binary but does not yet have
execute permission, so your system won't let you run it yet. Make it executable,
then (optionally) move it into a directory on your `PATH` so you can run `addch`
from anywhere. Open a terminal and run:

```sh
# Make the binary executable
chmod +x addch-linux-amd64

# (Optional) Move it to a directory on your PATH, e.g. /usr/local/bin
sudo mv addch-linux-amd64 /usr/local/bin/addch

# Verify it is installed
addch --version
```

If you skipped the `mv` step, run it from its download folder with `./addch`
instead of `addch`.

On **Windows**, download the `.exe` file. You can run it by double-clicking it,
but since it is a command-line tool you will normally run it from a terminal
(PowerShell or Command Prompt), for example:

```powershell
# Navigate to the folder where you downloaded the .exe
cd C:\Users\YourName\Downloads
.\addch-windows-amd64.exe --version
```

For convenient use from any folder, add that download folder to your Windows
`PATH` environment variable; then you can type `addch` by itself.

**Step 4 — Verify everything is working**

```sh
addch --check
```

This checks that `ffmpeg` and `ffprobe` are available and on your `PATH`. You
should see a message like `System is ready.` and exit with success.

**Step 5 — Create your chapter file**

A *chapter file* is a plain text file with one chapter per line. Create a file
named `chapters.txt` and put a line in it for every chapter you want:

```
00:00:00 Introduction
00:05:30 Setting Up
00:18:45 Core Concepts
```

The format on each line is `HH:MM:SS Chapter title` — hours:minutes:seconds,
followed by a space, followed by the chapter's title.

Want a ready-made example to experiment with? Run:

```sh
addch --example
```

This writes a valid `example_chapters.txt` into your current folder.

**Step 6 — Embed the chapters into a video**

Place your video and your chapter file in the same folder, then run:

```sh
addch chapters.txt "My Course.mp4"
```

**Step 7 — Find your output video**

`addch` creates a new file next to your original, with `-chapters` inserted
before the extension. It does **not** touch your original video. So for
`My Course.mp4`, you get:

```
My Course-chapters.mp4
```

That's it — the new file has your chapters embedded and your original video is
untouched. Use `--output` (see [Options](#options)) if you want a different
name or location.

---

## Requirements

`addch` requires the **FFmpeg** and **FFprobe** programs to be installed and
available in your `PATH`. It does **not** bundle them — you install them
separately, and you need Go only if you want to [build from source](#build-from-source).

Install FFmpeg/FFprobe:

| Platform | Command |
| --- | --- |
| Debian / Ubuntu | `sudo apt install ffmpeg` |
| Fedora / RHEL / CentOS | `sudo dnf install ffmpeg` |
| Arch Linux | `sudo pacman -S ffmpeg` |
| openSUSE | `sudo zypper install ffmpeg` |
| macOS (Homebrew) | `brew install ffmpeg` |
| Windows (Winget) | `winget install --id Gyan.FFmpeg --exact` |

> After installing, make sure both `ffmpeg` and `ffprobe` are available in your
> `PATH`. Do not rename the FFmpeg binaries — keep their original names. You can
> confirm everything is ready with `addch --check`.

---

## Installation

### Prebuilt binaries

Download the release binary for your platform from the
[Releases page](https://github.com/AbuTaha7000D/addch/releases). You do **not**
need to install Go to use a prebuilt binary — just download the matching file
for your system, make it executable (macOS/Linux), and run it. See the
[Quick Start](#quick-start) for the full step-by-step.

| Platform            | Binary                    |
| ------------------- | ------------------------- |
| Linux x86_64 (amd64) | `addch-linux-amd64`      |
| Linux ARM64          | `addch-linux-arm64`      |
| macOS Intel (amd64)  | `addch-darwin-amd64`     |
| macOS Apple Silicon  | `addch-darwin-arm64`     |
| Windows x86_64 (amd64) | `addch-windows-amd64.exe` |
| Windows ARM64        | `addch-windows-arm64.exe` |

The `addch` binary itself is self-contained; you still need `ffmpeg` and
`ffprobe` available in your `PATH` (see [Requirements](#requirements)).

Each release also includes a `SHA256SUMS.txt` file so you can verify the checksum
of any downloaded binary and confirm it was not corrupted in transit.

### Build from source

If you prefer to build the binary yourself (requires Go):

```sh
make build        # produces ./addch
make install      # go install
```

---

## Usage

```sh
addch chapters.txt "My Course.mp4"
```

This embeds the chapters from `chapters.txt` into `My Course.mp4` and writes a
new file, `My Course-chapters.mp4`, in the same directory. Your original video is
left untouched.

The general syntax is:

```
addch [options] <chapters-file> <video-file>
```

There are always **two** required arguments: the chapter file and the video file.
Options go before those two arguments.

---

## Chapter file format

One chapter per line:

```
HH:MM:SS Chapter title
```

Millisecond precision is also supported, and can be mixed freely in one file:

```
HH:MM:SS.mmm Chapter title
```

Empty lines are ignored. Both LF and CRLF line endings are accepted, as is a
UTF-8 BOM at the start of the file.

Example:

```
00:00:00 Introduction
00:05:30 Setting Up
00:18:45 Core Concepts
00:42:10 Advanced Topics
01:15:00 Wrap-Up
```

To generate a valid example file to start from:

```sh
addch --example     # writes example_chapters.txt
```

---

## Options

```
Usage: addch [options] <chapters-file> <video-file>

  -o, --output <file>   Custom output path (default: <video>-chapters.<ext>)
      --overwrite       Overwrite the output file if it already exists
      --example         Write example_chapters.txt and exit
      --check           Verify ffmpeg/ffprobe availability and exit
      --version         Print version and exit
  -h, --help            Print help and exit
```

Notes:

- Options must be placed before the two positional arguments
  (`<chapters-file>` and `<video-file>`). For example:
  `addch --overwrite chapters.txt video.mp4`, not after them.
- `addch` will not overwrite an existing output file unless you use
  `--overwrite`.
- `addch` will never overwrite your input video or your chapter file.
- Paths containing spaces or Unicode work everywhere; just quote them if your
  shell requires it (e.g. `addch chapters.txt "My Course.mp4"`).

---

## Examples

```sh
# Basic usage: writes "video-chapters.mp4"
addch chapters.txt video.mp4

# Custom output path and name
addch -o output.mp4 chapters.txt video.mp4

# Overwrite an existing output file
addch --overwrite chapters.txt video.mp4

# Write a sample chapter file to get started
addch --example

# Check that ffmpeg/ffprobe are installed and on PATH
addch --check

# Print the version
addch --version

# Print help
addch --help
```

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

---

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

---

## How it works

1. Check that FFmpeg and FFprobe are available.
2. Parse and validate the chapter file.
3. Validate the video and read its duration with `ffprobe`.
4. Generate an `FFMETADATA1` file internally (no manual FFmpeg syntax needed).
5. Remux with stream copy (no re-encoding) and embed the chapters:

   ```sh
   ffmpeg -i input -i chapters.meta \
     -map 0 -map_metadata 0 -map_chapters 1 \
     -c copy [-movflags +faststart] output
   ```

6. Verify the result with `ffprobe` (chapter count, titles, start and end times).

`+faststart` is added for MP4-family containers; it is omitted for others.

---

## Development

```sh
make test    # unit + integration tests
make vet     # static analysis
make dist    # cross-compile all platforms into ./dist
```

Integration tests require `ffmpeg` and `ffprobe`; they are skipped when those
tools are unavailable. CI runs unit and integration tests on Linux, macOS, and
Windows.

---

## License

`addch` is released under the [MIT License](LICENSE).

FFmpeg and FFprobe are **external dependencies** of `addch`. They are not bundled
with or linked into `addch`; you install them separately. `ffprobe` ships as part
of standard FFmpeg distributions, so installing FFmpeg normally provides both
executables. FFmpeg and FFprobe have their own licensing terms, which depend on
how they are built/configured (FFmpeg can be built under the LGPL or GPL,
depending on the enabled components). Refer to FFmpeg's own documentation for
details on its license.

---

## Author

addch is maintained by [Mahmoud Ehab](https://github.com/AbuTaha7000D)
(eng.mahmoud.e.hussein@gmail.com).

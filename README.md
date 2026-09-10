# addch — chapter toolkit

Add, remove, and extract chapters in your videos without re-encoding.

The repository is one Go module that builds **three standalone binaries**:

| Binary  | Operation                        | Output |
| ------- | -------------------------------- | ------ |
| `addch` | **Add** chapters to a video      | `*-chapters.<ext>` |
| `rmch`  | **Remove** chapters from a video | `*-nochapters.<ext>` |
| `getch` | **Extract** chapters from a video | stdout or `<name>.txt` sidecar |

`addch` and `rmch` use **stream copy**, so the original video is **never
modified** — they always write new output files. `getch` only reads chapters
(`ffprobe`-only) and never writes video. All three share one internal core but
remain independent executables.

---

## Quick Start

**Step 1 — Install FFmpeg**

`addch` and `rmch` need `ffmpeg` and `ffprobe`; `getch` needs `ffprobe` only
(`getch` is ffprobe-only and never runs `ffmpeg`). Installing FFmpeg normally
provides both. See [Requirements](#requirements) for your operating system.

**Step 2 — Download the right binary**

Each release publishes `addch`, `rmch`, and `getch` binaries for every platform.
Go to the [Releases page](https://github.com/AbuTaha7000D/addch/releases), open
the latest release, and download the binaries that match your operating system
and architecture (CPU type).

| Your operating system | `addch` | `rmch` | `getch` |
| --------------------- | ------- | ------ | ------- |
| **Linux** — Intel/AMD 64-bit PC or server (most common) | `addch-linux-amd64` | `rmch-linux-amd64` | `getch-linux-amd64` |
| **Linux** — 64-bit ARM (e.g. many Raspberry Pi models) | `addch-linux-arm64` | `rmch-linux-arm64` | `getch-linux-arm64` |
| **macOS** — Intel processor | `addch-darwin-amd64` | `rmch-darwin-amd64` | `getch-darwin-amd64` |
| **macOS** — Apple Silicon (M1, M2, M3, M4) | `addch-darwin-arm64` | `rmch-darwin-arm64` | `getch-darwin-arm64` |
| **Windows** — Intel/AMD 64-bit PC | `addch-windows-amd64.exe` | `rmch-windows-amd64.exe` | `getch-windows-amd64.exe` |
| **Windows** — 64-bit ARM | `addch-windows-arm64.exe` | `rmch-windows-arm64.exe` | `getch-windows-arm64.exe` |

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
chmod +x addch-linux-amd64 rmch-linux-amd64 getch-linux-amd64

# (Optional) Move them to a directory on your PATH, e.g. /usr/local/bin
sudo mv addch-linux-amd64 /usr/local/bin/addch
sudo mv rmch-linux-amd64 /usr/local/bin/rmch
sudo mv getch-linux-amd64 /usr/local/bin/getch

# Verify they are installed
addch --version
rmch --version
getch --version
```

If you skipped the `mv` step, run them from their download folders with `./addch`
(etc.) instead of `addch`.

On **Windows**, download the `.exe` files. You can run them by double-clicking
them, but since they are command-line tools you will normally run them from a
terminal (PowerShell or Command Prompt), for example:

```powershell
# Navigate to the folder where you downloaded the .exe files
cd C:\Users\YourName\Downloads
.\addch-windows-amd64.exe --version
```

For convenient use from any folder, add that download folder to your Windows
`PATH` environment variable; then you can type `addch` by itself.

**Step 4 — Verify everything is working**

```sh
addch --check
```

This checks that the required tools are available and on your `PATH`. When both
`ffmpeg` and `ffprobe` are installed, all three tools print the same report and
exit with success:

```sh
rmch --check    # same output
getch --check   # same output
```

`getch` depends on **ffprobe only** and never invokes `ffmpeg`. So if `ffmpeg`
is missing from your `PATH` but `ffprobe` is present, `getch --check` still
succeeds, while `addch --check` and `rmch --check` fail with an install hint.

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

This writes a valid `example_chapters.txt` into your current folder. `getch`
produces files in this same format, so you can round-trip chapters between the
tools.

**Step 6 — Add chapters to a video**

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
untouched. Use `--output` (see [Options](#options-reference)) if you want a
different name or location. `rmch` works the same way except its output is
`My Course-nochapters.mp4`; `getch` prints the chapters to your terminal (or a
file with `-o`).

---

## Requirements

The toolkit uses **only the Go standard library**; FFmpeg and FFprobe remain
external runtime requirements that are not bundled.

- **`addch` and `rmch`** require both **FFmpeg** and **FFprobe**.
- **`getch`** requires **FFprobe** only — it reads chapters via ffprobe and never
  runs `ffmpeg`.

You need Go only if you want to [build from source](#build-from-source).

Install FFmpeg/FFprobe:

| Platform | Command |
| --- | --- |
| Debian / Ubuntu | `sudo apt install ffmpeg` |
| Fedora / RHEL / CentOS | `sudo dnf install ffmpeg` |
| Arch Linux | `sudo pacman -S ffmpeg` |
| openSUSE | `sudo zypper install ffmpeg` |
| macOS (Homebrew) | `brew install ffmpeg` |
| Windows (Winget) | `winget install --id Gyan.FFmpeg --exact` |

> After installing, make sure `ffmpeg` and `ffprobe` are available in your
> `PATH`. Do not rename the FFmpeg binaries — keep their original names. You can
> confirm everything is ready with `addch --check` / `rmch --check` /
> `getch --check`.

---

## Installation

### Prebuilt binaries

Download the release binaries for your platform from the
[Releases page](https://github.com/AbuTaha7000D/addch/releases). You do **not**
need to install Go to use the prebuilt binaries — just download the matching
files for your system, make them executable (macOS/Linux), and run them. See the
[Quick Start](#quick-start) for the full step-by-step.

| Platform | Binaries |
| ------------------- | --------------------------------------------- |
| Linux x86_64 (amd64) | `addch-linux-amd64`, `rmch-linux-amd64`, `getch-linux-amd64` |
| Linux ARM64          | `addch-linux-arm64`, `rmch-linux-arm64`, `getch-linux-arm64` |
| macOS Intel (amd64)  | `addch-darwin-amd64`, `rmch-darwin-amd64`, `getch-darwin-amd64` |
| macOS Apple Silicon  | `addch-darwin-arm64`, `rmch-darwin-arm64`, `getch-darwin-arm64` |
| Windows x86_64 (amd64) | `addch-windows-amd64.exe`, `rmch-windows-amd64.exe`, `getch-windows-amd64.exe` |
| Windows ARM64        | `addch-windows-arm64.exe`, `rmch-windows-arm64.exe`, `getch-windows-arm64.exe` |

The binaries are self-contained; you still need the external programs listed in
[Requirements](#requirements).

Each release also includes a `SHA256SUMS.txt` file so you can verify the checksum
of any downloaded binary and confirm it was not corrupted in transit.

### Build from source

If you prefer to build the binaries yourself (requires Go):

```sh
make build        # produces ./addch ./rmch ./getch
make install      # go install all three
```

`make build` and `make install` build and install all three binaries (`addch`,
`rmch`, and `getch`). To build or install a single one individually:

```sh
go build -o rmch ./cmd/rmch
go install ./cmd/getch
```

---

## Usage

The three tools share one command structure: options go before the positional
arguments, `-o`/`--output` spell the output (defaults differ per tool), and the
batch flags `--dir`/`--recursive` are mutually exclusive.

### addch — add chapters

```sh
addch chapters.txt "My Course.mp4"
```

This embeds the chapters from `chapters.txt` into `My Course.mp4` and writes a
new file, `My Course-chapters.mp4`, in the same directory. Your original video is
left untouched.

```
addch [options] <chapters-file> <video-file>
```

### rmch — remove chapters

```sh
rmch "My Course.mp4"
```

This strips every chapter marker from `My Course.mp4` and writes a new file,
`My Course-nochapters.mp4`, in the same directory. A video with no chapters is
refused (there would be nothing to remove).

```
rmch [options] <video-file>
```

### getch — extract chapters

```sh
getch "My Course.mp4"
```

This prints the chapters of `My Course.mp4` to stdout, one
`HH:MM:SS[.mmm] Title` line per chapter, in the order FFprobe reports them.
A file with no chapters produces empty stdout and exits 0. Diagnostics and
progress always go to stderr, so stdout carries only chapter data and can be
piped directly:

```sh
getch "My Course.mp4" > chapters.txt        # exact addch sidecar format
getch -o chapters.txt "My Course.mp4"       # same, written atomically
```

```
getch [options] <media-file>
```

### Batch mode

All three tools can process many files in one run. `--dir` processes only the
files directly inside a directory; `--recursive` also descends into nested
subdirectories. The two flags are mutually exclusive.

```sh
addch --dir ./videos            # add: needs a matching .txt sidecar per file
addch --recursive ./videos      # same, recursively
rmch --dir ./videos
getch --recursive ./videos      # writes <name>.txt sidecars (except no-chapter files)
```

Batch processing is **sequential** and **continues after individual errors**.
Existing outputs are skipped unless you pass `--overwrite`, and none of the tools
ever re-process their own generated `-chapters`/`-nochapters` outputs. Each run
ends with a summary line like:

```
Total: N | Succeeded: N | Skipped: N | Failed: N
```

---

## Options reference

| Option | Meaning | addch | rmch | getch |
| ------ | ------- | :---: | :---: | :---: |
| `-o, --output <file>` | Custom output path | default `<video>-chapters.<ext>` | default `<video>-nochapters.<ext>` | default `stdout` |
| `--overwrite` | Replace the output file if it already exists | ✓ | ✓ | ✓ |
| `--dir` | Batch mode: files directly inside a directory | ✓ | ✓ | ✓ |
| `--recursive` | Batch mode: directory and all subdirectories | ✓ | ✓ | ✓ |
| `--example` | Write `example_chapters.txt` and exit | ✓ | — | — |
| `--check` | Verify FFmpeg/FFprobe availability (`addch`/`rmch` need both; `getch` needs `ffprobe` only) | ✓ | ✓ | ✓ |
| `--version` | Print the version and exit | ✓ | ✓ | ✓ |
| `-h, --help` | Print this help and exit | ✓ | ✓ | ✓ |

Notes:

- Options must be placed before the positional arguments. For example:
  `addch --overwrite chapters.txt video.mp4`, not after them.
- `--dir` and `--recursive` are **mutually exclusive**; supplying both is a usage
  error.
- `--output` cannot be combined with `--dir`/`--recursive` batch mode.
- All tools will not overwrite an existing output file unless you use
  `--overwrite`.
- `getch` in single-file mode requires `--overwrite` together with `--output`,
  because its default target is stdout (which cannot be overwritten).
- No tool ever overwrites your input video (or your chapter file).
- Paths containing spaces or Unicode work everywhere; just quote them if your
  shell requires it (e.g. `addch chapters.txt "My Course.mp4"`).

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

`addch` reads this format, and `getch` writes it, so the two are
round-trip-compatible.

To generate a valid example file to start from:

```sh
addch --example     # writes example_chapters.txt
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

All three tools verify their dependencies first, then operate with **stream
copy** so your original files are never modified and no video is re-encoded.

1. **`addch`** parses and validates the chapter file, validates the video and
   reads its duration with `ffprobe`, generates an `FFMETADATA1` file internally
   (no manual FFmpeg syntax needed), remuxes with stream copy while embedding the
   chapters, then verifies the result with `ffprobe`:

   ```sh
   ffmpeg -i input -i chapters.meta \
     -map 0 -map_metadata 0 -map_chapters 1 \
     -c copy [-movflags +faststart] output
   ```

   `+faststart` is added for MP4-family containers; it is omitted for others.

2. **`rmch`** strips every chapter marker with a stream-copy remux that maps out
   all chapters (`-map_chapters -1`), then verifies with `ffprobe` that the
   output has zero chapters.

3. **`getch`** runs **ffprobe only** — it never invokes `ffmpeg`. It asks FFprobe
   for the chapter list and renders it to stdout (or an atomic sidecar file).

### Supported containers

Chapter embedding is **empirically verified** for **MP4**, **MKV**, and
**M4A/AAC** (`addch`). Removal and extraction are verified on **MP4** and **MKV**.
Additional containers are supported only after they have real FFmpeg/FFprobe
integration tests — see `TOOLKIT_SCOPE.md`.

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

The toolkit is released under the [MIT License](LICENSE).

FFmpeg and FFprobe are **external dependencies**. They are not bundled with or
linked into any binary; you install them separately. `ffprobe` ships as part of
standard FFmpeg distributions, so installing FFmpeg normally provides both
executables. FFmpeg and FFprobe have their own licensing terms, which depend on
how they are built/configured (FFmpeg can be built under the LGPL or GPL,
depending on the enabled components). Refer to FFmpeg's own documentation for
details on its license.

---

## Author

The toolkit is maintained by [Mahmoud Ehab](https://github.com/AbuTaha7000D)
(eng.mahmoud.e.hussein@gmail.com).
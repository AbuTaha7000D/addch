# TOOLKIT_SCOPE.md

Operational scope and approved v0.2 decisions for the `addch` chapter-management
toolkit. This document records decisions that were reviewed and approved before
implementation; it is not a design proposal. Changes to these decisions require
a separate reviewed task.

---

## Binaries

The repository will produce **three independent binaries** from one Go module:

- `addch` — add chapters
- `rmch` — remove chapters
- `getch` — extract chapters

They share one internal core but remain separate executables.

## Shared core

Shared code belongs only in these packages:

- `internal/chapters`
- `internal/media`
- `internal/fsutil`
- `internal/batch`

No extra abstraction layers (dependency injection, unnecessary interfaces,
service/usecase/handler layers, speculative abstractions) unless a later
reviewed task proves they are necessary.

## Copy-out guarantee

All media-changing operations use **copy-out** and never modify the original
input. The **default output path** is adjacent to the input (a new file next to
the original). A tool may provide an explicit custom output path; a custom
output path must preserve all input-overwrite safety checks.

## Dependencies

The project uses **only the Go standard library**. FFmpeg and FFprobe remain
**external runtime requirements**; the project does not bundle them and does not
install them.

## Directory modes

- `--dir` processes only the files directly inside the given directory
  (shallow).
- `--recursive` processes the directory and all nested subdirectories.
- Supplying both `--dir` and `--recursive` together is a **usage error**, not a
  list that silently picks one.

## Batch execution

- Batch processing is **sequential**.
- Batch processing **continues after per-item errors**; one failed item does not
  stop the remaining items.
- Existing outputs are **skipped** unless the user explicitly supplies
  `--overwrite`.

## Batch discovery

Batch discovery follows this pipeline:

1. A **curated media-extension allowlist** (cheap filename filter).
2. An **exact `<stem>.txt` sidecar match** — the chapter file is derived from
   the video's stem by replacing only the final extension.
3. **FFprobe validation** of the actual media.

Generated `-chapters` and `-nochapters` outputs are **excluded** from batch
discovery to prevent re-processing of previously generated files.

## Signals — release blocker

Ctrl+C (SIGINT) and SIGTERM behavior is a release blocker. On interruption the
toolkit must:

- stop scheduling new work,
- terminate and reap any active FFmpeg child process,
- clean up partial and temporary files,
- then exit with code **130** (SIGINT) or **143** (SIGTERM).

## Container support

`addch` chapter embedding is **empirically verified only** for the **MP4**,
**MKV**, and **M4A/AAC** containers (Phase 2 FFmpeg verification; see
`FFMPEG_VERIFICATION.md`).

Each additional container requires its **own** real FFmpeg/FFprobe integration
test before it may be added to the support matrix.

This matrix applies **only to `addch`**. It does not yet claim `rmch` or `getch`
compatibility with any container, because those commands are not implemented or
verified.

---

## Standing constraints

- Do not weaken existing v0.1.0 safety guarantees (no shell invocation,
  argument-slice execution, secure temp files, cleanup on failure/interruption,
  no partial outputs, no accidental input overwrite, symlink-aware same-path
  checks, stream copy, explicit overwrite).
- The existing command `addch chapters.txt video.mp4` must continue to work
  exactly as before.

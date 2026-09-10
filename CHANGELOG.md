# Changelog

All notable changes to the addch chapter toolkit are documented in this file.

## Unreleased

### v0.2.0-rc.1

The v0.2.0 release candidate ships the three-tool chapter toolkit: `addch`
(chapter embedding, carried forward from v0.1.x), plus the new `rmch` and
`getch` commands, with a unified CLI, batch processing, and an archive-based
packaging pipeline for the documented six-platform matrix.

> Note: an approved source refactor is staged in the working tree
> (`cmd/*`, `internal/*`) but is **uncommitted**; it is intentionally absent
> from this changelog and will ship with the v0.2.0 final release.

#### Added

- `rmch` — remove chapters with stream copy, `*-nochapters.<ext>` default
  output, and FFprobe zero-chapter verification (`1ccd5b1`).
- `getch` — FFprobe-only chapter extraction to stdout or `-o` sidecar; empty
  stdout and exit 0 for chapterless media (`ca34074`).
- Chapter TXT grammar specification — `GRAMMAR.md` (`e49c6d9`).
- v0.2 scope and CLI-policy decisions — `TOOLKIT_SCOPE.md` (`8beae49`).
- Empirical FFmpeg/FFprobe round-trip evidence for MP4, MKV, and M4A/AAC —
  `FFMPEG_VERIFICATION.md` plus integration tests
  (`992a226`, `1c1985a`, `9f22159`, `c245fe9`, `236078f`).
- Shared internal core (`internal/chapters`, `internal/media`,
  `internal/fsutil`, `internal/batch`) and entry points relocated under `cmd/`
  (`5222d1a`, `221b68e`, `91aad81`, `ad1e46a`, `9dc3710`).
- Batch mode `--dir` / `--recursive` for all tools: curated media-extension
  allowlist, exact `<stem>.txt` sidecar matching, FFprobe validation, and
  exclusion of generated `*-chapters` / `*-nochapters` outputs; sequential and
  error-tolerant processing with `Total | Succeeded | Skipped | Failed`
  summaries; SIGINT/SIGTERM exit 130/143 with cleanup
  (`931d072`, `02b0259`, `38d0e50`, `a75bb7b`, `f9372b9`).
- Consistent CLI UX across the three tools, cross-tool help/`--check`
  contract tests, and standardized `getch` usage text
  (`588098b`, `aee49f5`, `fe1067d`).
- GoReleaser pipeline (`.goreleaser.yaml`): 36 README-named archives
  (tar.gz/zip) + `SHA256SUMS.txt`, tag-injected `--version`, git-based
  changelog, and license metadata; pre-publish version gate in the release
  workflow (`f3491f2`, `4858dad`); Makefile/release tooling ships all three
  binaries (`f352b03`).
- Packaged-binary E2E suite for the release toolchain (`scripts/e2e`)
  (`30c107b`).

#### Fixed

- `getch` reports stdout write failures and exits non-zero instead of
  silently exiting 0 (`87d6a6c`).
- Recursive batch discovery aborts with an error on traversal failures
  instead of reporting a false success (`2d6ca27`).
- MOV/M4V (QuickTime) removed from batch discovery after real FFmpeg 8.1.2
  evidence showed `rmch` cannot strip their residual text chapter track
  (rc 183); `addch` embeds and `getch` extracts them single-file
  (`c2401b2`).
- Symlinked batch roots are resolved before the recursive walk, and output
  cleanup never removes a directory (`8436669`, `e69f646`).
- Metadata temp files are scoped via `ADDCH_METADATA_TMPDIR` for hermetic
  parallel tests (`f641409`); lying-FFmpeg dependencies are rejected
  (`c1e854d`, `8c12b4b`).
- `release.yml` built from the module root (which failed with "no Go files");
  now builds each of `./cmd/addch`, `./cmd/rmch`, `./cmd/getch` (`f352b03`).

#### Docs

- README clarified across the board and expanded with add/remove/extract and
  batch examples (`d703695`, `c413873`, `cb4144e`).
- Verification claims reconciled in `GRAMMAR.md`, `TOOLKIT_SCOPE.md`, and
  `FFMPEG_VERIFICATION.md` — no false "unimplemented" statements; round-trip
  claims scoped per requirement and container (`bdcec4b`).
- Quick Start rewritten for archive-based download/extract/run, and the
  `getch --output` / `--overwrite` note corrected (`1b3e6ac`).
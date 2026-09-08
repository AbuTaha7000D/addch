# addch Toolkit v0.2.0 Roadmap

This is the working checklist for the v0.2.0 chapter toolkit. Update it at the
end of every approved task so the next task is always explicit.

## Status legend

- `[ ]` Not started
- `[~]` In progress or awaiting the user's commit
- `[x]` Complete and committed by the user

## Current position

- Current branch: `feature/chapter-toolkit`
- Baseline tests: passing (`go test ./...`)
- Current phase: **Phase 2 complete** — next: Phase 3 (shared core refactor)
- Last completed deliverable: Phase 2 empirical verification, documented in
  `FFMPEG_VERIFICATION.md` and finalized for M4A/AAC in `9f22159`, completed
  by the round-trip proof `TestRoundTripParseEqualsProbe` in
  `integration_test.go` (committed in `236078f`).

## Phase 0 — Baseline and scope decisions

- [x] Run the v0.1.0 test suite and establish a green baseline.
- [x] Create and switch to branch `feature/chapter-toolkit`.
- [x] Document the explicitly supported media scope in `TOOLKIT_SCOPE.md`: only
  containers covered by project integration tests; do not make untested
  general-media claims. (Committed as `8beae49`.)
- [x] Record the CLI policy in `TOOLKIT_SCOPE.md`: `--dir` and `--recursive`
  are mutually exclusive; batch processing is sequential and continues after
  individual errors; output replacement requires `--overwrite`. (Committed as
  `8beae49`.)
- [x] Phase exit: baseline, branch, and scope decisions are committed.

## Phase 1 — Chapter TXT grammar contract

- [x] Draft `GRAMMAR.md` only; do not change source code, tests, README, build
  files, or project structure.
- [x] Specify encoding, BOM, LF/CRLF, empty and blank lines, timestamps,
  delimiters, ordering, first-at-zero validation, titles, escaping, comments,
  and whitespace behavior.
- [x] Specify the TXT-semantic and future media round-trip contracts, including
  foreign-media limitations.
- [x] Complete architecture/QA review.
- [x] Commit `GRAMMAR.md` (`e49c6d9`).
- [x] Phase exit: grammar contract is committed.

## Phase 2 — Empirical FFmpeg verification

- [x] Create MP4 and MKV fixtures. (Committed in `992a226`.)
- [x] Cover Unicode, spaces, `=`, `;`, `#`, quotes, literal backslash sequences,
  and other escaping-sensitive titles. (Committed in `992a226`.)
- [x] Prove the FFMETADATA construction and escaping behavior through real
  FFmpeg/FFprobe integration tests for MP4 and MKV. (Committed in `992a226`.)
- [x] Verify M4A/AAC audio chapter embedding through real FFmpeg/FFprobe,
  including time-base conversion, end-chain behavior, and copy-out safety.
  (Committed in `1c1985a`.)
- [x] Update FFmpeg findings and the addch-only support matrix to include the
  M4A/AAC evidence. (Committed in `9f22159`.)
- [x] Prove round-trip behavior: parsed input equals parsed extraction, and
  media chapters match at the supported precision. (Committed in `236078f`.)
- [x] Document observed MP4/MKV FFmpeg behavior affecting the contract in
  `FFMPEG_VERIFICATION.md`. (Committed in `c245fe9`.)
- [x] Phase exit: integration results are documented and all ambiguity is
  settled by tests.

## Phase 3 — Shared core refactor

- [x] Create `internal/chapters` for parsing, validation, and serialization.
  (Committed in `221b68e`.)
- [x] Create `internal/media` for FFmpeg/FFprobe, remuxing, and verification.
  (Committed in `91aad81`.)
- [x] Create `internal/fsutil` for paths, matching, and discovery. (Committed in `5222d1a`.)
- [x] Create `internal/batch` for orchestration and result reporting.
  (Committed in `ad1e46a`.)
- [ ] Move the add command entry point to `cmd/addch/main.go`.
- [ ] Preserve v0.1 behavior and keep all existing tests passing.
- [ ] Phase exit: refactor is behavior-preserving and tested.

## Phase 4 — addch batch mode

- [ ] Add shallow `--dir` and recursive `--recursive` modes.
- [ ] Reject using `--dir` and `--recursive` together.
- [ ] Discover candidates using: supported media extension + exact matching
  `<stem>.txt` sidecar + FFprobe validation.
- [ ] Match sidecars correctly with Unicode and spaces.
- [ ] Skip generated `-chapters` and `-nochapters` outputs to prevent chaining.
- [ ] Process sequentially, continue after errors, and report success/skipped/
  failed totals.
- [ ] Handle Ctrl+C/SIGTERM: stop scheduling, kill and reap current FFmpeg,
  clean temporary and partial files, then exit 130/143.
- [ ] Phase exit: batch and interrupt integration tests pass.

## Phase 5 — rmch

- [ ] Create the standalone `rmch` binary.
- [ ] Support a single file, `--dir`, and `--recursive`.
- [ ] Use copy-out output named `*-nochapters.<ext>` and require `--overwrite`
  for replacement.
- [ ] Preserve streams, metadata, subtitles, and audio with stream copy where
  supported.
- [ ] Verify with FFprobe that output has zero chapters; remove invalid output
  on verification failure.
- [ ] Test MP4 and MKV removal and preservation assertions.
- [ ] Phase exit: remove and verification tests pass.

## Phase 6 — getch

- [ ] Create the standalone `getch` binary.
- [ ] Write chapter data to stdout and diagnostics to stderr.
- [ ] Support `-o`, `--dir`, and `--recursive`.
- [ ] Return empty stdout and exit 0 for media without chapters; do not create
  empty sidecar files in output or directory modes.
- [ ] Document faithful extraction and addch validation limits for foreign
  media.
- [ ] Add round-trip integration tests and CLI stream tests.
- [ ] Phase exit: extraction contract is proven by tests.

## Phase 7 — Unified UX

- [ ] Standardize usage text, error messages, and exit codes across all tools.
- [ ] Standardize naming, overwrite policy, summaries, and `--check`.
- [ ] Update README with add, remove, extract, and batch examples.
- [ ] Add CLI contract tests.
- [ ] Phase exit: all three commands present a consistent interface.

## Phase 8 — Adversarial QA

- [ ] Test Unicode and space-containing paths, symlinks, and path collisions.
- [ ] Test mismatched sidecars, existing outputs, unusual metadata, timestamp
  boundaries, and escape-sensitive titles.
- [ ] Add failure-injection coverage.
- [ ] Prove original files are never modified and temporary files are cleaned.
- [ ] Phase exit: blocker-focused regression suite passes.

## Phase 9 — Packaging

- [ ] Add GoReleaser configuration for all three binaries and supported
  platforms.
- [ ] Produce archives, checksums, changelog, and license metadata.
- [ ] Build and inspect a local release snapshot.
- [ ] Phase exit: package snapshot is ready for release review.

## Phase 10 — Release candidate

- [ ] Run end-to-end tests against packaged binaries and real FFmpeg.
- [ ] Review documentation and compatibility.
- [ ] Prepare `v0.2.0-rc` release metadata.
- [ ] Phase exit: release gates are complete.

## Phase 11 — Final release

- [ ] Update final version and changelog.
- [ ] Review the final diff and obtain `READY TO COMMIT`.
- [ ] User creates the final commit, tag, and release according to project
  policy.
- [ ] Phase exit: v0.2.0 is released.

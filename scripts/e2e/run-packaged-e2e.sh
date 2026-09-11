#!/usr/bin/env bash
#
# run-packaged-e2e.sh — end-to-end acceptance suite for the PACKAGED
# addch / rmch / getch binaries of a GoReleaser snapshot.
#
# The suite mirrors the semantics enforced by the committed source tests
# (cmd/*/integration_test.go, batch_test.go, contract_test.go) but executes
# the *archived* linux/amd64 binaries from a snapshot, so the artifacts that
# would actually ship in a release are the things under test.
#
# Usage:
#   scripts/e2e/run-packaged-e2e.sh [binaries-dir] [corpus-dir] [expected-version]
#
#   binaries-dir     Directory containing the executable addch, rmch, getch
#                    binaries. Default: dist/bin under the repo root; if that
#                    is missing, a fresh goreleaser snapshot is built and the
#                    linux/amd64 ZIP (addch-linux-amd64.zip) is extracted into
#                    it.
#   corpus-dir       Read-only media corpus to copy per run. Default:
#                    manualTestsFolder next to the repo root.
#   expected-version Exact version every `--version` must print (e.g.
#                    "v0.1.2-SNAPSHOT-4858dad"). Default: taken from the addch
#                    binary; the other two must match it exactly.
#
# The suite only READS the corpus; all work happens in a fresh temp dir that is
# removed on exit. The corpus's aggregate sha256 is asserted identical before
# and after the run, so the originals are provably untouched. The script exits
# 0 only when every check passes; any mismatch prints a FAIL line and exits 1.
#
# Prerequisites on PATH: ffmpeg, ffprobe, python3, unzip, sha256sum, bash 4+.
# When the default binaries layout is missing, also: go and network access (to
# fetch the goreleaser toolchain via `go run`, which leaves go.mod untouched).

set -u
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# ---------------------------------------------------------------------------
# Argument resolution
# ---------------------------------------------------------------------------
ARG_BIN="${1:-}"
ARG_CORPUS="${2:-}"
ARG_EXPECTED="${3:-}"

if [ -n "$ARG_CORPUS" ]; then
  CORPUS="$ARG_CORPUS"
else
  CORPUS="$ROOT/manualTestsFolder"
fi

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }
note() { printf '%s\n' "$*"; }

# ---------------------------------------------------------------------------
# Required tools
# ---------------------------------------------------------------------------
for cmd in ffmpeg ffprobe python3 unzip sha256sum; do
  command -v "$cmd" >/dev/null 2>&1 || fail "required tool '$cmd' not found in PATH"
done

# ---------------------------------------------------------------------------
# Corpus (always canonicalized to an absolute path so aggregates are stable
# regardless of the caller's CWD). The proof hashes are computed from the
# corpus's parent directory using the relative "manualTestsFolder" prefix, the
# exact form the project's verification uses, so the published aggregate
# (71b3bbdd…) is reproduced.
# ---------------------------------------------------------------------------
if [ -n "$ARG_CORPUS" ]; then
  CORPUS="$(cd "$ARG_CORPUS" && pwd)" || fail "corpus dir not accessible: $ARG_CORPUS"
else
  CORPUS="$ROOT/manualTestsFolder"
fi
CORPUS_BASE="$(dirname "$CORPUS")"

corpus_agg() { # -> aggregate sha256 of the corpus (relative-path convention)
  local base rel
  base="$CORPUS_BASE"
  rel="$(basename "$CORPUS")"
  ( cd "$base" && find "$rel" -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}' )
}
[ -d "$CORPUS" ] || fail "corpus directory not found: $CORPUS"
AGG_BEFORE="$(corpus_agg)"
note "corpus (before): $AGG_BEFORE"

# ---------------------------------------------------------------------------
# Fresh temp workdir, cleaned up on every exit path.
# ---------------------------------------------------------------------------
WORK="$(mktemp -d -t addch-e2e.XXXXXX)"

cleanup() {
  local a
  a="$(corpus_agg 2>/dev/null)"
  rm -rf "$WORK"
  if [ -n "$AGG_BEFORE" ] && [ "$a" = "$AGG_BEFORE" ]; then
    printf 'CORPUS UNTOUCHED: %s (before == after)\n' "$AGG_BEFORE"
  else
    printf 'FAIL: corpus changed! before=%s after=%s\n' "${AGG_BEFORE:-?}" "${a:-?}" >&2
    trap - EXIT
    exit 1
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Binaries: resolve the directory, or build a fresh snapshot + extract.
# ---------------------------------------------------------------------------
if [ -n "$ARG_BIN" ]; then
  BIN="$ARG_BIN"
  [ -x "$BIN/addch" ] && [ -x "$BIN/rmch" ] && [ -x "$BIN/getch" ] \
    || fail "binaries dir '$BIN' must contain executable addch, rmch, getch"
else
  BIN="$ROOT/dist/bin"
  echo "NOTE: no binaries dir given; using $BIN"
  if [ ! -x "$BIN/addch" ] || [ ! -x "$BIN/rmch" ] || [ ! -x "$BIN/getch" ]; then
    command -v go >/dev/null 2>&1 || fail "go not found; cannot build the default snapshot"
    echo "NOTE: building a fresh goreleaser snapshot (this does not touch go.mod)"
    (cd "$ROOT" && GOTOOLCHAIN=auto \
      go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean) \
      || fail "goreleaser snapshot failed"
    DIST="$ROOT/dist"
    (cd "$DIST" && sha256sum -c SHA256SUMS.txt >/dev/null 2>&1) \
      || fail "dist SHA256SUMS.txt failed its own checksum verification"
    mkdir -p "$BIN"
    unzip -q "$DIST/addch-linux-amd64.zip" -d "$BIN" || fail "extract addch-linux-amd64.zip"
    for t in addch rmch getch; do
      [ -x "$BIN/$t" ] || fail "addch-linux-amd64.zip is missing executable $t"
    done
  fi
fi

# ---------------------------------------------------------------------------
# Expected version (default: read from the packaged addch binary).
# ---------------------------------------------------------------------------
if [ -n "$ARG_EXPECTED" ]; then
  EXPECTED="$ARG_EXPECTED"
else
  EXPECTED="$( "$BIN/addch" --version 2>/dev/null | sed 's/^addch //' )"
  case "$EXPECTED" in
    v[0-9]*.[0-9]*.[0-9]*) ;;
    *) fail "cannot derive a snapshot version from addch --version ('${EXPECTED:-<empty>}')" ;;
  esac
fi

# ---------------------------------------------------------------------------
# Small helpers
# ---------------------------------------------------------------------------
chapter_count() { # $1 = media file -> prints number of embedded chapters
  ffprobe -v error -show_chapters -print_format json "$1" \
    | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("chapters",[])))'
}
chapter_titles() { # $1 = media file -> prints titles, one per line
  ffprobe -v error -show_chapters -print_format json "$1" \
    | python3 -c 'import json,sys;print("\n".join(c.get("tags",{}).get("title","") for c in json.load(sys.stdin).get("chapters",[])))'
}
assert_eq() { # $1=label  $2=got  $3=want
  [ "$2" = "$3" ] || fail "$1: got '$2', want '$3'"
}
assert_rc() { # $1=label  $2=got_rc  $3=want_rc
  assert_eq "$1" "$2" "$3"
}
assert_contains() { # $1=label  $2=haystack  $3=needle
  case "$2" in
    *"$3"*) ;;
    *) fail "$1: output does not contain '$3'" ;;
  esac
}
assert_file_eq() { # $1=label  $2=file  $3=expected-content
  assert_eq "$1+content" "$(cat "$2")" "$3"
}
assert_file_diff() { # $1=label  $2=fileA  $3=fileB  (diff must be empty)
  if ! diff -u "$2" "$3" >/dev/null; then fail "$1: files differ"; fi
}

# ---------------------------------------------------------------------------
# Workdir prep: copy the corpus (originals stay read-only).
# ---------------------------------------------------------------------------
cp -R "$CORPUS"/. "$WORK/" || fail "cannot copy corpus into the temp workdir"

# --- Controlled sidecar contents -------------------------------------------
AR_VALID='00:00:00 مقدمة
00:00:45 بداية
00:03:00 خاتمة
'
ASCII_VALID='00:00:00 Chapter 1
00:00:45 Chapter 2
00:03:00 Chapter 3
'
BAD_DURATION='00:00:00 Start
00:05:00 Beyond Duration
'

# --- Controlled media fixtures ---------------------------------------------
# single_ar.*        Arabic-named media + valid Arabic sidecar.
cp "$WORK/فيديو اغنية.mp4" "$WORK/single_ar.mp4" || fail "cp Arabic video"
printf '%s' "$AR_VALID" > "$WORK/single_ar.txt"
# single_ascii       ASCII-named, CHAPTERLESS media.
cp "$WORK/Test Video 3.mp4" "$WORK/single_ascii.mp4" || fail "cp chapterless video"
# ow.*               overwrite-test pair.
cp "$WORK/Test Video 3.mp4" "$WORK/ow.mp4" || fail "cp overwrite video"
printf '%s' "$ASCII_VALID" > "$WORK/ow.txt"
# Batch dirs (kept isolated so every count is deterministic).
mkdir -p "$WORK/batch/addch_good" "$WORK/batch/addch_bad" "$WORK/batch/getch" "$WORK/rec/sub/deep"

# ---------------------------------------------------------------------------
# [1/13] --check on every tool -> dependency-OK line, rc 0
# ---------------------------------------------------------------------------
for t in addch rmch getch; do
  OUT="$("$BIN/$t" --check 2>&1)"; RC=$?
  assert_rc "$t --check" "$RC" 0
  assert_contains "$t --check" "$OUT" "System is ready."
  pass "$t --check -> rc 0, System is ready"
done

# ---------------------------------------------------------------------------
# [2/13] --version on every tool -> "<tool> v0.1.2-SNAPSHOT-<sha>", rc 0
# ---------------------------------------------------------------------------
for t in addch rmch getch; do
  OUT="$("$BIN/$t" --version)"; RC=$?
  assert_rc "$t --version" "$RC" 0
  assert_eq "$t --version" "$OUT" "$t $EXPECTED"
  pass "$t --version -> '$t $EXPECTED'"
done

# ---------------------------------------------------------------------------
# [3/13] single-file addch with a valid in-duration Arabic sidecar
# ---------------------------------------------------------------------------
"$BIN/addch" "$WORK/single_ar.txt" "$WORK/single_ar.mp4" >"$WORK/s3.out" 2>"$WORK/s3.err"; RC=$?
assert_rc "addch single-file (Arabic)" "$RC" 0
[ -f "$WORK/single_ar-chapters.mp4" ] || fail "addch: expected output single_ar-chapters.mp4"
assert_eq "addch chapter count" "$(chapter_count "$WORK/single_ar-chapters.mp4")" 3
assert_eq "addch chapter titles" "$(chapter_titles "$WORK/single_ar-chapters.mp4")" "$(printf '%s' "$AR_VALID" | sed 's/^[0-9:]\{8\} //; s/ $//')"
pass "addch Arabic single-file -> 3 chapters embedded (مقدمة/بداية/خاتمة)"

# ---------------------------------------------------------------------------
# [4/13] getch WITHOUT -o -> chapter data on stdout ONLY, byte-identical
# ---------------------------------------------------------------------------
"$BIN/getch" "$WORK/single_ar-chapters.mp4" >"$WORK/s4.out" 2>"$WORK/s4.err"; RC=$?
assert_rc "getch stdout mode" "$RC" 0
[ -s "$WORK/s4.err" ] || fail "getch: expected diagnostics on stderr"
assert_file_eq "getch stdout == sidecar" "$WORK/s4.out" "$(printf '%s' "$AR_VALID")"
pass "getch (no -o) -> stdout is chapter data only, byte-identical to the sidecar"

# ---------------------------------------------------------------------------
# [5/13] getch -o out.txt -> byte-identical round-trip
# ---------------------------------------------------------------------------
"$BIN/getch" -o "$WORK/round.txt" "$WORK/single_ar-chapters.mp4" >"$WORK/s5.out" 2>"$WORK/s5.err"; RC=$?
assert_rc "getch -o" "$RC" 0
[ -s "$WORK/s5.out" ] && fail "getch -o: stdout must stay empty (file output mode)"
assert_file_diff "getch -o round-trip" "$WORK/round.txt" "$WORK/single_ar.txt"
pass "getch -o round-trip -> diff clean"

# ---------------------------------------------------------------------------
# [6/13] getch on a chapterless file -> EMPTY stdout, rc 0
# ---------------------------------------------------------------------------
"$BIN/getch" "$WORK/single_ascii.mp4" >"$WORK/s6.out" 2>"$WORK/s6.err"; RC=$?
assert_rc "getch chapterless" "$RC" 0
[ -s "$WORK/s6.out" ] && fail "getch chapterless: stdout must be empty"
pass "getch on chapterless file -> empty stdout, rc 0"

# ---------------------------------------------------------------------------
# [7/13] rmch on a chaptered file -> -nochapters output, zero chapters
# ---------------------------------------------------------------------------
"$BIN/rmch" "$WORK/single_ar-chapters.mp4" >"$WORK/s7.out" 2>"$WORK/s7.err"; RC=$?
assert_rc "rmch chaptered" "$RC" 0
[ -f "$WORK/single_ar-chapters-nochapters.mp4" ] || fail "rmch: expected -nochapters output"
assert_eq "rmch chapter count after strip" "$(chapter_count "$WORK/single_ar-chapters-nochapters.mp4")" 0
pass "rmch chaptered -> -nochapters output with zero chapters"

# ---------------------------------------------------------------------------
# [8/13] rmch on a chapterless file -> clean refusal, rc != 0, no output
# ---------------------------------------------------------------------------
"$BIN/rmch" "$WORK/single_ascii.mp4" >"$WORK/s8.out" 2>"$WORK/s8.err"; RC=$?
[ "$RC" -ne 0 ] || fail "rmch chapterless: expected non-zero rc"
assert_contains "rmch chapterless message" "$(cat "$WORK/s8.err")" "has no chapters to remove"
[ -e "$WORK/single_ascii-nochapters.mp4" ] && fail "rmch chapterless must not create an output"
pass "rmch chapterless -> refused (rc $RC), no output file"

# ---------------------------------------------------------------------------
# [9/13] --dir batches: correct succeeded/skipped/failed counts, rc per policy
# ---------------------------------------------------------------------------
# --- addch --dir (good): x.mp4 + y.mkv valid; w.mkv output pre-exists -------
cp "$WORK/Test Video 2.mp4" "$WORK/batch/addch_good/x.mp4"
cp "$WORK/Test Video 1 (Copy 2).mkv" "$WORK/batch/addch_good/y.mkv"
cp "$WORK/Test Video 1 (Copy 2).mkv" "$WORK/batch/addch_good/w.mkv"
printf '%s' "$ASCII_VALID" > "$WORK/batch/addch_good/x.txt"
printf '%s' "$ASCII_VALID" > "$WORK/batch/addch_good/y.txt"
printf '%s' "$ASCII_VALID" > "$WORK/batch/addch_good/w.txt"
printf 'pre-existing output\n' > "$WORK/batch/addch_good/w-chapters.mkv"

"$BIN/addch" --dir "$WORK/batch/addch_good" >"$WORK/s9a.out" 2>"$WORK/s9a.err"; RC=$?
assert_rc "addch --dir (good) overall rc" "$RC" 0
assert_contains "addch --dir (good) summary" "$(cat "$WORK/s9a.out")" "Total: 3 | Succeeded: 2 | Skipped: 1 | Failed: 0"
[ -f "$WORK/batch/addch_good/x-chapters.mp4" ] || fail "addch --dir: x-chapters.mp4 missing"
[ -f "$WORK/batch/addch_good/y-chapters.mkv" ] || fail "addch --dir: y-chapters.mkv missing"
pass "addch --dir -> Total: 3 | Succeeded: 2 | Skipped: 1 | Failed: 0, rc 0"

# --- getch --dir: alpha.mp4 + beta.mkv chaptered, gamma.mp4 chapterless -----
"$BIN/addch" -o "$WORK/batch/getch/alpha.mp4" "$WORK/single_ar.txt" "$WORK/Test Video 2.mp4" \
  >"$WORK/s9b0.out" 2>"$WORK/s9b0.err"; RC=$?
assert_rc "addch -o alpha.mp4 (fixture setup)" "$RC" 0
"$BIN/addch" -o "$WORK/batch/getch/beta.mkv" "$WORK/single_ar.txt" "$WORK/Test Video 1 (Copy 2).mkv" \
  >"$WORK/s9b1.out" 2>"$WORK/s9b1.err"; RC=$?
assert_rc "addch -o beta.mkv (fixture setup)" "$RC" 0
cp "$WORK/Test Video 3.mp4" "$WORK/batch/getch/gamma.mp4"

"$BIN/getch" --dir "$WORK/batch/getch" >"$WORK/s9b.out" 2>"$WORK/s9b.err"; RC=$?
assert_rc "getch --dir (first run)" "$RC" 0
[ -s "$WORK/s9b.out" ] && fail "getch --dir: stdout must stay empty"
assert_contains "getch --dir first-run summary" "$(cat "$WORK/s9b.err")" "Total: 3 | Succeeded: 2 | Skipped: 1 | Failed: 0"
assert_contains "getch --dir first-run skip reason" "$(cat "$WORK/s9b.err")" "(no chapters)"
[ -f "$WORK/batch/getch/alpha.txt" ] || fail "getch --dir: alpha.txt missing"
[ -f "$WORK/batch/getch/beta.txt" ] || fail "getch --dir: beta.txt missing"
assert_file_diff "getch --dir alpha.txt round-trip" "$WORK/batch/getch/alpha.txt" "$WORK/single_ar.txt"
assert_file_diff "getch --dir beta.txt round-trip" "$WORK/batch/getch/beta.txt" "$WORK/single_ar.txt"
[ -e "$WORK/batch/getch/gamma.txt" ] && fail "getch --dir: chapterless must not create gamma.txt"
pass "getch --dir -> chaptered succeeded (2), chapterless skipped, rc 0"

"$BIN/getch" --dir "$WORK/batch/getch" >"$WORK/s9c.out" 2>"$WORK/s9c.err"; RC=$?
assert_rc "getch --dir (second run)" "$RC" 0
assert_contains "getch --dir second-run summary" "$(cat "$WORK/s9c.err")" "Total: 3 | Succeeded: 0 | Skipped: 3 | Failed: 0"
pass "getch --dir rerun -> existing sidecars skipped, rc 0"

# --- rmch --dir on the same chaptered directory ------------------------------
"$BIN/rmch" --dir "$WORK/batch/getch" >"$WORK/s9d.out" 2>"$WORK/s9d.err"; RC=$?
assert_rc "rmch --dir (first run)" "$RC" 0
assert_contains "rmch --dir first-run summary" "$(cat "$WORK/s9d.out")" "Total: 3 | Succeeded: 2 | Skipped: 1 | Failed: 0"
[ -f "$WORK/batch/getch/alpha-nochapters.mp4" ] || fail "rmch --dir: alpha-nochapters.mp4 missing"
[ -f "$WORK/batch/getch/beta-nochapters.mkv" ] || fail "rmch --dir: beta-nochapters.mkv missing"
[ -e "$WORK/batch/getch/gamma-nochapters.mp4" ] && fail "rmch --dir: chapterless must not create an output"
pass "rmch --dir -> chaptered stripped (2), chapterless skipped, rc 0"

"$BIN/rmch" --dir "$WORK/batch/getch" >"$WORK/s9e.out" 2>"$WORK/s9e.err"; RC=$?
assert_rc "rmch --dir (second run)" "$RC" 0
assert_contains "rmch --dir second-run summary" "$(cat "$WORK/s9e.out")" "Total: 3 | Succeeded: 0 | Skipped: 3 | Failed: 0"
pass "rmch --dir rerun -> existing outputs skipped, rc 0"

# --- generated outputs are excluded from a later scan ------------------------
"$BIN/getch" --dir "$WORK/batch/addch_good" >"$WORK/s9f.out" 2>"$WORK/s9f.err"; RC=$?
assert_rc "getch --dir (exclusion scan)" "$RC" 0
assert_contains "generated-output exclusion summary" "$(cat "$WORK/s9f.err")" "Total: 3 | Succeeded: 0 | Skipped: 3 | Failed: 0"
case "$(cat "$WORK/s9f.err")" in
  *-chapters*) fail "getch --dir counted a '-chapters' generated output" ;;
esac
pass "generated '-chapters' outputs excluded from later batch scans"

"$BIN/getch" --dir "$WORK/batch/getch" >"$WORK/s9g.out" 2>"$WORK/s9g.err"; RC=$?
assert_rc "getch --dir (nochapters exclusion scan)" "$RC" 0
assert_contains "nochapters exclusion summary" "$(cat "$WORK/s9g.err")" "Total: 3 | Succeeded: 0 | Skipped: 3 | Failed: 0"
case "$(cat "$WORK/s9g.err")" in
  *-nochapters*) fail "getch --dir counted a '-nochapters' generated output" ;;
esac
pass "generated '-nochapters' outputs excluded from later batch scans"

# ---------------------------------------------------------------------------
# [10/13] batch continuation: one invalid sidecar fails but the batch survives
# ---------------------------------------------------------------------------
cp "$WORK/Test Video 3.mp4" "$WORK/batch/addch_bad/a_fail.mp4"
cp "$WORK/Test Video 2.mp4" "$WORK/batch/addch_bad/b.mp4"
cp "$WORK/Test Video 1 (Copy 2).mkv" "$WORK/batch/addch_bad/c.mkv"
printf '%s' "$BAD_DURATION" > "$WORK/batch/addch_bad/a_fail.txt"
printf '%s' "$ASCII_VALID" > "$WORK/batch/addch_bad/b.txt"
printf '%s' "$ASCII_VALID" > "$WORK/batch/addch_bad/c.txt"

"$BIN/addch" --dir "$WORK/batch/addch_bad" >"$WORK/s10.out" 2>"$WORK/s10.err"; RC=$?
assert_rc "addch --dir (bad sidecar) overall rc" "$RC" 1
assert_contains "addch --dir (bad) summary" "$(cat "$WORK/s10.out")" "Total: 3 | Succeeded: 2 | Skipped: 0 | Failed: 1"
assert_contains "addch --dir (bad) failure line" "$(cat "$WORK/s10.out")" "[failed]"
assert_contains "addch --dir (bad) failure reason" "$(cat "$WORK/s10.out")" "exceeds the video duration"
[ -f "$WORK/batch/addch_bad/b-chapters.mp4" ] || fail "addch --dir: batch must continue after the failed item"
[ -f "$WORK/batch/addch_bad/c-chapters.mkv" ] || fail "addch --dir: batch must continue after the failed item"
pass "addch --dir -> exactly 1 failed (timestamp beyond duration), batch continued, rc 1"

# ---------------------------------------------------------------------------
# [11/13] --recursive (nested childFolder) + --dir/--recursive mutual exclusion
# ---------------------------------------------------------------------------
cp "$WORK/Test Video 2.mp4" "$WORK/rec/a.mp4"
cp "$WORK/Test Video 1 (Copy 2).mkv" "$WORK/rec/sub/b.mkv"
cp "$WORK/Test Video 3.mp4" "$WORK/rec/sub/deep/c.mp4"
printf '%s' "$ASCII_VALID" > "$WORK/rec/a.txt"
printf '%s' "$ASCII_VALID" > "$WORK/rec/sub/b.txt"
printf '%s' "$ASCII_VALID" > "$WORK/rec/sub/deep/c.txt"

"$BIN/addch" --recursive "$WORK/rec" >"$WORK/s11a.out" 2>"$WORK/s11a.err"; RC=$?
assert_rc "addch --recursive" "$RC" 0
assert_contains "addch --recursive summary" "$(cat "$WORK/s11a.out")" "Total: 3 | Succeeded: 3 | Skipped: 0 | Failed: 0"
[ -f "$WORK/rec/a-chapters.mp4" ] || fail "recursive: a-chapters.mp4 missing (depth 0)"
[ -f "$WORK/rec/sub/b-chapters.mkv" ] || fail "recursive: sub/b-chapters.mkv missing (depth 1)"
[ -f "$WORK/rec/sub/deep/c-chapters.mp4" ] || fail "recursive: sub/deep/c-chapters.mp4 missing (depth 2)"
pass "addch --recursive -> nested childFolder media processed (3/3)"

"$BIN/addch" --dir "$WORK/rec" >"$WORK/s11b.out" 2>"$WORK/s11b.err"; RC=$?
assert_rc "addch --dir shallow" "$RC" 0
assert_contains "addch --dir shallow summary" "$(cat "$WORK/s11b.out")" "Total: 1 | Succeeded: 0 | Skipped: 1 | Failed: 0"
pass "addch --dir shallow -> only the top-level file is a candidate"

for t in addch rmch getch; do
  # Both flags must precede the positional: Go's flag package stops scanning at
  # the first non-flag argument, so `--dir --recursive <dir>` is the form that
  # hits the mutual-exclusion rule.
  "$BIN/$t" --dir --recursive "$WORK/rec" >"$WORK/s11x.out" 2>"$WORK/s11x.err"; RC=$?
  [ "$RC" -ne 0 ] || fail "$t --dir --recursive: expected non-zero rc"
  assert_contains "$t --dir --recursive message" "$(cat "$WORK/s11x.err")" "--dir and --recursive are mutually exclusive"
  pass "$t --dir --recursive -> refused (rc $RC)"
done

# ---------------------------------------------------------------------------
# [12/13] --overwrite: refused without it, allowed with it
# ---------------------------------------------------------------------------
"$BIN/addch" "$WORK/ow.txt" "$WORK/ow.mp4" >"$WORK/s12a.out" 2>"$WORK/s12a.err"; RC=$?
assert_rc "addch (initial)" "$RC" 0
[ -f "$WORK/ow-chapters.mp4" ] || fail "addch: ow-chapters.mp4 missing"

"$BIN/addch" "$WORK/ow.txt" "$WORK/ow.mp4" >"$WORK/s12b.out" 2>"$WORK/s12b.err"; RC=$?
[ "$RC" -ne 0 ] || fail "addch rerun: expected non-zero rc"
assert_contains "addch rerun message" "$(cat "$WORK/s12b.err")" "already exists"
pass "addch rerun without --overwrite -> refused (rc $RC)"

"$BIN/addch" --overwrite "$WORK/ow.txt" "$WORK/ow.mp4" >"$WORK/s12c.out" 2>"$WORK/s12c.err"; RC=$?
assert_rc "addch --overwrite rerun" "$RC" 0
pass "addch --overwrite rerun -> rc 0"

# ---------------------------------------------------------------------------
# All checks passed
# ---------------------------------------------------------------------------
note
note "ALL CHECKS PASSED against $BIN (version '$EXPECTED')"
exit 0
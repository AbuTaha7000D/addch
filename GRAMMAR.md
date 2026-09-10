# Chapter TXT Grammar (GRAMMAR.md)

This document is the authoritative, normative specification for the **chapter
TXT format** used by `addch`, `rmch`, and `getch`. It is written from (a) the
current, audited behavior of the `addch` v0.1.0 implementation and (b) the
approved v0.2.0 contract, which keeps this grammar unchanged.

It covers:

* what a well-formed chapter file looks like (syntax),
* the exact rules of the parser (`internal/chapters` in v0.2),
* the exact rules of validation,
* the canonical output format (including what `getch` must produce),
* the precise round-trip guarantee, split into a **TXT-semantic** tier
  (grammar-guaranteed) and a **media/FFprobe** tier (a Phase-2-verified
  contract requirement), plus the explicit limits of both.

> This document is a specification, not an implementation guide. Any source
> code must conform to the rules below; where the code and this document
> disagree, the code is wrong.

---

## 1. Overview and Encoding

A chapter file is a **text** file containing zero or more chapter definitions,
one per line.

* **Normative encoding:** the format is **defined** as UTF-8 text; non-ASCII
  titles (including Arabic and any other Unicode) are represented as raw UTF-8
  bytes. This is the domain within which the round-trip guarantees of
  [§10](#10-round-trip) hold.
* **Encoding validation on input (v0.1 behavior):** the current parser is
  **byte-oriented** and performs **no encoding validation whatsoever**. Arbitrary
  byte sequences — including invalid UTF-8 — are accepted and pass through
  unmodified, particularly in titles. In particular, a file is **not** rejected
  for containing invalid UTF-8. Timestamps must still consist of ASCII
  digits/`:`/`.` (see [§3](#3-timestamps)); Unicode is validated only to the
  extent that non-ASCII bytes fail the timestamp grammar.
* **Output encoding:** output produced by `getch` is authored as UTF-8 (from
  the byte strings FFprobe returns); byte fidelity is otherwise preserved
  (see [§10.1](#101-txt-semantic-round-trip-grammar-guaranteed)).
* **Line endings:** both LF (`\n`) and CRLF (`\r\n`) are accepted on input.
  `bufio.Scanner` strips the terminating `\r?\n`; the parser additionally
  strips any trailing `\r` from each line before processing. Output produced by
  `getch` uses LF only (see [§9](#9-canonical-output--getch-contract)).
  CR-only line endings (a lone `\r` as the line separator) are **not**
  supported.
* **Byte order mark (BOM):** a UTF-8 BOM (`U+FEFF`) at the very start of the
  file (before the first line) is accepted and stripped. A BOM anywhere else is
  treated as ordinary content.
* **Line length / buffer:** implementations MUST be able to read lines up to
  at least **1 MiB** in length. A single physical line longer than the maximum
  supported token size causes a hard error (`bufio.ErrTooLong`).

The format is **line-oriented** and deliberately has **no escape syntax**.
Sequences such as `\n`, `\t`, `\\` in the text have **no special meaning** and
are preserved literally as part of the title (see [§7](#7-title-contents)).
Newlines are represented only as actual line breaks between chapter entries.

---

## 2. Grammar

The grammar is defined over **bytes**, not Unicode code points. Whitespace in
this section is therefore defined precisely (see below).

> **Whitespace definitions used in this spec:**
>
> * `ascii-ws` — the ASCII space (`U+0020`) and the ASCII horizontal tab
>   (`U+0009`). Only these two bytes ever act as the **timestamp/title
>   delimiter** ([§4](#4-line-structure-and-title-separation)).
> * `uni-ws` — any Unicode whitespace character (implementation-defined as the
>   set recognized by Go's `unicode.IsSpace`). `uni-ws` is used **only** for the
>   blank-line test and for trimming the title; it is **never** a delimiter.
>   `uni-ws` is a *Unicode-classification concept applied at the string level*,
>   not a strict byte-level ABNF pattern: the grammar is defined over bytes,
>   but the "blank line" and "trimming" decisions are made by the
>   implementation with a Unicode-aware whitespace test.

In ABNF-like notation:

```text
chapter-file    = bom? [ line *( eol line ) [ eol ] ]
bom             = %xEF.BB.BF            ; UTF-8 BOM, only at start of file
line            = blank / definition
blank           = *uni-ws               ; empty physical line (created by
                                        ; consecutive eols) or whitespace-only
                                        ; line; skipped
definition      = timestamp sep title
sep             = 1*ascii-ws            ; first ASCII space/tab is the delimiter
eol             = "\n" / "\r\n"

timestamp       = hh ":" mm ":" ss [ "." frac ]
hh              = 2digit
mm              = 2digit
ss              = 2digit
frac            = 1*digit               ; any number of ASCII digits; only the
                                        ; first three are used (milliseconds);
                                        ; any remaining digits are truncated
digit           = %x30-39               ; ASCII digits only
title           = <any sequence of non-newline bytes, not beginning or ending
                   with uni-ws after trimming — see §4>
```

### 2.1 End-of-file semantics

* **Optional trailing line ending:** the final line of the file need not be
  terminated by an `eol`. A file ending with the last chapter on its final
  line, with or without a trailing newline, is valid (both are accepted).
* **Empty file:** a zero-byte file is grammatically valid at parse level and
  parses to **zero chapters**. Because an empty chapter list is rejected by
  validation ([§6](#6-validation-rule-set) rule 1), an empty file is
  semantically **invalid** as a chapter file.
* **Whitespace-only file:** a file containing only blank lines (including a
  lone BOM, or only `uni-ws`) likewise parses to zero chapters and is rejected
  by validation.
* **Consecutive line endings** produce an empty physical line (zero characters
  between two `eol`s), which is a `blank` line and is skipped, as is any line
  consisting only of `uni-ws`.

### 2.2 Leading whitespace

* A line that is **entirely** whitespace (`blank`) is skipped.
* Any **other** line must begin directly with a `timestamp` token. **Leading
  whitespace (ASCII or Unicode) before a timestamp is an error**:
  - An ASCII space/tab at offset 0 places the delimiter at index 0, which the
    parser rejects (split requires the delimiter at a positive offset).
  - A leading Unicode whitespace character (e.g. NBSP `U+00A0`) is not a
    delimiter; it remains part of the timestamp token and causes a timestamp
    parse error.

---

## 3. Timestamps

### 3.1 Syntax

A timestamp is `HH:MM:SS`, optionally followed by `.` and one or more ASCII
digits (only the first three are significant — see [§3.3](#33-fractional-seconds)):

```text
00:00:00
00:00:00.500
00:01:23.456
99:59:59
```

### 3.2 Components

Each of `HH`, `MM`, `SS` must be **exactly two ASCII digits** `[0-9]`:

| Component | Range      | Constraint                                |
|-----------|------------|-------------------------------------------|
| `HH`      | `00` – `99`| exactly two digits, leading zero required  |
| `MM`      | `00` – `59`| exactly two digits                         |
| `SS`      | `00` – `59`| exactly two digits                         |

* Only **ASCII** digits are accepted. Non-ASCII digit characters (e.g.
  Arabic-Indic digits) are not valid.
* A **leading zero is required**: `0:00:00` or `5:30` are **invalid**.
* Partial timestamps (`5:30`, `:30`) are **invalid**; all three components are
  required.
* `HH` beyond `99` is not representable; timestamps with `HH > 99` (or
  `MM > 59`, `SS > 59`) are invalid. This bounds the tool to a maximum
  representable timestamp of `99:59:59` (= 359,999,000 ms). See
  [§10.3](#103-explicitly-not-guaranteed) for the consequence on `getch`
  output.

### 3.3 Fractional seconds

* `HH:MM:SS.mmm` adds millisecond precision.
* The fractional part is **one or more ASCII digits**; only the first three
  digits are interpreted as milliseconds, and any subsequent digits are
  truncated (not rounded):
  - `.5` → 500 ms
  - `.05` → 50 ms
  - `.005` → 5 ms
  - `.500` → 500 ms
  - `.1234` → 123 ms (the trailing `4` is truncated, not rounded)
* **Truncation is deterministic and silent**: extra digits beyond the third are
  *ignored* and do **not** cause an error, **provided they are ASCII digits**.
* A fractional part containing a **non-digit** anywhere — including beyond the
  millisecond precision — is **invalid** (e.g. `.1234x`, `.9999a`).
* A `.` with no following digit (`.`) or followed by a non-digit (`.a`) is
  **invalid**.

### 3.4 Internal representation

Internally, a timestamp is stored as a signed integer number of **milliseconds**
since the start of the media. The maximum valid internal value corresponds to
`99:59:59.999`.

---

## 4. Line Structure and Title Separation

### 4.1 The delimiter is ASCII space/tab only

On each non-blank line, the parser splits at the **first ASCII space
(`U+0020`) or ASCII tab (`U+0009`)**; the search operates byte-wise:

1. Find the first occurrence of `ascii-ws`.
2. If there is **no** such byte, or it occurs at **offset 0**, the line is
   **invalid**.
3. Otherwise, the **timestamp token** is everything before that byte, and the
   **title portion** is everything from that byte onward.

### 4.2 Unicode whitespace is never a delimiter

Unicode whitespace characters (NBSP `U+00A0`, EM SPACE `U+2003`, and the rest
of `unicode.IsSpace`) do **not** separate timestamp from title:

* If a non-ASCII whitespace character appears between the timestamp and the
  title **without** a preceding ASCII space/tab, the line has no delimiter and
  is **invalid**.
* If a non-ASCII whitespace character appears **before** the timestamp, it
  stays inside the timestamp token and causes a timestamp parse error.

### 4.3 Trimming

While the *delimiter* is ASCII-only, the *trimming* of the title portion is
**Unicode-aware**:

* The title portion is trimmed of leading and trailing whitespace using the
  full Unicode whitespace set (`uni-ws`). Therefore:
  - Leading ASCII delimiters/spaces/tabs and any leading Unicode whitespace are
    removed together.
  - Trailing Unicode whitespace (e.g. a trailing NBSP) is removed.
  - A title can **never begin or end with whitespace** of any kind.
* Interior whitespace — ASCII or Unicode — is **preserved** as part of the
  title.
* The **blank-line test** is also Unicode-aware: a line consisting only of
  `uni-ws` is skipped.
* If, after trimming, the title portion is empty, the line is **invalid**
  (a timestamp with no title).

> **Consequence:** `00:00:00 A` and `00:00:00\u00A0A` both yield the title `A`,
> but `00:00:00\u00A0A` **without** the preceding ASCII space is invalid. The
> delimiter and the trimming set are deliberately different concepts and must
> not be conflated.

---

## 5. Parse Errors

The following are **errors** (the whole file is rejected; no partial results
are used):

* A line whose timestamp is structurally invalid (see [§3](#3-timestamps)),
  through either a malformed timestamp token or a delimiter at offset 0
  (leading whitespace).
* A line with no delimiter and no timestamp (i.e. non-blank content that is
  not a valid definition).
* A line that is a timestamp followed by only whitespace (no title).
* A single physical line that exceeds the maximum supported token size
  (1 MiB).
* A file that cannot be read (missing file, permission denied, I/O error).

**Invalid UTF-8 is not a parse error** (see [§1](#1-overview-and-encoding)):
the parser performs no encoding validation.

**Error locations:** content parse errors that occur *after a line has been
read* — a structurally invalid timestamp, a missing title, or leading
whitespace before a timestamp — are reported with the **1-based line number**
of that line. Errors at the file level (file open/read failure) and
scanner/token-size errors (a physical line exceeding the maximum supported
size) are reported **without a line number**.

---

## 6. Validation Rule Set

After parsing the file into an ordered list of chapters, the following
validation rules are applied **in order**; the first violation is reported and
rejects the file:

1. **Non-empty:** the file must contain at least one chapter. An empty file
   (zero chapters, including whitespace-only files) is **invalid**.
2. **No newline in title:** a title must not contain a `\n` or `\r` byte.
3. **No trailing backslash:** a title must **not end with a backslash** (`\`).
   This is a **media-layer** limitation: the TXT format itself can hold such a
   title (see [§10.1](#101-txt-semantic-round-trip-grammar-guaranteed)), but
   FFmpeg's FFMETADATA format cannot represent it, so `addch` rejects it at
   validation to avoid corrupt output. **A backslash anywhere else in the title
   is allowed.**
4. **Distinct start times:** no two chapters may share the same start
   timestamp.
5. **Chronological order:** chapter start timestamps must be **strictly
   increasing** (each subsequent chapter starts later than the previous).
6. **First at zero:** the first chapter must start exactly at `00:00:00`
   (0 ms).

### 6.1 Duration constraints

A chapter's start timestamp must be **≤ the video duration** (in milliseconds).
A chapter starting exactly at the duration is allowed (it forms a zero-length
trailing chapter). A chapter starting after the video ends is **invalid**.

This rule is applied against the *specific* target video at add/embed time and
is therefore part of the operation, not the plain-text grammar.

---

## 7. Title Contents

With the two exceptions below, **any** text may appear in a title and is
preserved byte-for-byte:

| Allowed | Example |
|---------|---------|
| Spaces (interior) | `My Course` |
| Tabs (interior) | `a\tb` |
| Colons, `=`, `;`, `#` | `Module 3: Advanced` |
| Quotes | `say "hi"` |
| Backslash (non-final) | `Back\slash` |
| Arabic and any Unicode | `مقدمة` |
| Literal `\n`, `\t`, `\r` (as 2-char text) | `line\nbreak` |

**Prohibited / not representable:**

* A **real newline** inside a title — impossible by construction (line-oriented
  format; there is no escape language, so `\n` in text is a literal two-byte
  sequence, not a newline).
* A title that **begins or ends with whitespace**, ASCII or Unicode — impossible
  by construction (trimming, [§4.3](#43-trimming)).
* A title that is empty (after trimming) — invalid.

**Backslash at the end of a title** is parseable at the TXT level but is
rejected by **validation** (rule 6 #3 above) because the media layer
(FFMETADATA) cannot represent it. It is therefore not a valid input to an
*embed* operation, although the TXT grammar itself can hold it.

---

## 8. Comment / Blank Handling

* **Blank lines** (lines consisting only of `uni-ws`, per the Unicode-aware
  definition in [§4.3](#43-trimming)) are skipped and never become chapters.
* **Comments** are **not supported** by the grammar. Any line that is not a
  valid chapter definition is a parse error. There is no comment syntax; a
  line beginning with `#` or `;` is treated as content (and, lacking a valid
  leading timestamp, is an error).

---

## 9. Canonical Output (getch — contract)

> **Status note:** `getch` is implemented (Phase 2) and its canonical-output
> contract is verified against real FFmpeg/FFprobe by the command integration
> tests. The rules in this section are therefore both contract requirements and
> verified shipping behavior.

`getch` MUST emit the canonical form, which is also the form re-parseable by
`addch`/`rmch`:

* **Encoding:** UTF-8, authored from the byte strings FFprobe returns.
* **No BOM.**
* **Line endings:** LF only. (The parser accepts LF and CRLF, but output is LF
  for determinism.)
* One chapter per line: `HH:MM:SS` (or `HH:MM:SS.mmm` if the timestamp has a
  non-zero millisecond remainder), a space, then the title verbatim.

Examples:

```text
00:00:00 Introduction
00:00:01.250 Getting Started
00:05:30 Part Two
99:59:59 Final Notes
```

Timestamp formatting rules:

* Components are always zero-padded to two digits.
* The fractional part is emitted **only when the millisecond remainder is
  non-zero** (`.500` for 500 ms, omitted for a whole number of seconds).
* The title is written **verbatim** (no escaping).

> **Note on HH width:** formatting is not capped at 99 hours; a very long
> foreign file could yield `100:00:00+`, which would **not** re-parse with
> `addch` (which caps `HH` at 99). This is out of contract — see
> [§10.3](#103-explicitly-not-guaranteed).

---

## 10. Round-Trip

The round-trip story is deliberately split into two tiers, because the two
tiers are guaranteed by different mechanisms and are in different maturity
states:

* **TXT-semantic round-trip** is a property of this grammar and its canonical
  form alone; for well-formed UTF-8 input it holds **today**, independent of
  FFmpeg.
* **Media/FFprobe round-trip** depends on `addch`, `getch`, `rmch` (and the
  FFMETADATA escaping) engaging FFmpeg/FFprobe correctly. It is a **contract
  requirement** ([§10.2](#102-mediaffprobe-round-trip-contract-requirement--phase-2-verified))
  that the Phase 2 implementation verifies with real FFmpeg/FFprobe; the
  evidence is recorded in `FFMPEG_VERIFICATION.md`, scoped to the containers
  that suite tests.

### 10.1 TXT semantic round-trip (grammar-guaranteed)

For **any well-formed UTF-8 file that the parser accepts**, the following
holds:

```text
parse(canon(parse(f)))  ==  parse(f)
```

The parser itself is **byte-transparent**: it accepts files containing invalid
UTF-8 and preserves title bytes verbatim ([§1](#1-overview-and-encoding)),
without rejecting them at parse time. However, the canonical output form is
*defined* as UTF-8 ([§9](#9-canonical-output-getch--contract)), so the
canonical round-trip guarantee above is **scoped to well-formed UTF-8 input**.
For invalid-UTF-8 or non-UTF-8 content the parser still applies the byte-level
rules (delimiter, trimming) and preserves what it parses, but **no
canonicalization or round-trip claim is made** for such content (see
[§10.3](#103-explicitly-not-guaranteed)).

i.e., canonical serialization ([§9](#9-canonical-output-getch--contract))
followed by parsing is the **identity on the parsed chapter list**. Concretely,
for every line, the canonical output restores the exact `(start_ms, title)`
pair:

* **Timestamps at millisecond precision** round-trip exactly. `00:01:23.456`
  stays `.456`; `.5` canonicalizes to `.500`; the fractional part is emitted
  only when non-zero. Parsing the canonical form yields the same `start_ms`.
* **Titles round-trip byte-for-byte**: canonical output writes the title
  verbatim, and re-parsing yields the identical title bytes. This includes all
  Unicode, `=`, `;`, `#`, quotes, interior whitespace, non-final backslashes,
  and **even titles ending in a backslash** (the TXT layer has no escape
  syntax, so nothing is lost).
* **Blank lines** in the original file are not preserved; they are dropped by
  canonicalization. That is a *textual* difference with no semantic effect on
  the chapter list.

This tier **guarantees** the chain:

```text
chapters.txt ──parse──▶ list ──canonicalize──▶ canonical.txt ──parse──▶ same list
```

It does **not** require byte-equality of the text files. The comparison that
matters is equality of the parsed `(start_ms, title)` lists.

### 10.2 Media/FFprobe round-trip (contract requirement — Phase-2-verified)

The following are **verified behavior** of the current implementation, each
scoped to the containers its own proof exercises; the authoritative
per-command support matrix is TOOLKIT_SCOPE.md §Container support. The
media-layer behavior of `rmch`/`getch` was not established by `addch` v0.1.0,
so v0.2 made these **contract requirements**: the implementation MUST prove
them with integration tests before any claim of media-level losslessness is
made. That proof now exists in the Phase 2 real-FFmpeg/FFprobe integration
suite (`FFMPEG_VERIFICATION.md`), and the verified container scope is recorded
with each requirement below:

* **R1 — addch → getch fidelity:** `getch video.mp4` returns exactly the
  millisecond start times and exact title bytes that `addch` embedded in that
  video (media→TXT direction).
  **Verified scope:** MP4/MKV — `TestExtractMP4` / `TestExtractMKV`
  (`cmd/getch`).
* **R2 — full chain equality:** for

  ```text
  txt1 ──addch──▶ v1 ──getch──▶ txt2 ──addch──▶ v2
  ```

  the chapters observed by FFprobe in `v1` and `v2` are equal in their ordered
  **`(start_ms, title)`** pairs. This is the general comparison that holds
  regardless of the target media.
  **Verified scope:** MP4/MKV — `TestExtractRoundTripFullChain` (`cmd/getch`).

  **End times are a separate, same-duration claim.** A chapter's `end_ms` is
  not part of the TXT grammar: `addch` derives each chapter's `end_ms` from the
  next chapter's `start_ms`, and the *final* chapter's `end_ms` from the target
  media duration. Therefore `end_ms` equality between `v1` and `v2` is
  guaranteed only when both `addch` operations use **the same source media**
  (and hence an equal duration), or two sources with an explicitly equal
  duration. Under unequal durations the final chapters' `end_ms` differ by
  construction, which is expected and not a round-trip failure; the
  `(start_ms, title)` comparison in R2 remains the authoritative check.
* **R3 — media-layer escaping:** FFmpeg's FFMETADATA escaping round-trips all
  of the title characters in [§7](#7-title-contents) (Arabic, spaces, `=`, `;`,
  `#`, quotes, literal `\n`/`\t`/`\r` text, non-final backslash). The
  trailing-backslash case is excluded by construction (rejected by
  validation, rule 6 #3).
  **Verified scope:** MP4/MKV/M4A — the addch round-trip proofs
  (`TestChapterRoundTripMP4AndMKV`, `TestChapterRoundTripM4A`,
  `TestRoundTripParseEqualsProbe`).
* **R4 — parser-level addition:** `ParseChaptersFile(extracted.txt)` must
  produce a chapter list identical to `ParseChaptersFile(original.txt)` (this
  ties the media tier back to the TXT tier).
  **Verified scope:** MP4/MKV — the getch extraction proofs listed under R1.

Compliance with R1, R2, and R4 is verified for **MP4/MKV** by the getch
extraction proofs; compliance with **R3** — and `addch` embedding itself — is
verified for **MP4/MKV/M4A** by the addch round-trip proofs, while `rmch` strip
is verified for **MP4/MKV**. See `FFMPEG_VERIFICATION.md` and the per-command
support matrix in TOOLKIT_SCOPE.md §Container support.

### 10.3 Explicitly NOT guaranteed

The following are **out of contract** for round-tripping in all tiers:

* **Sub-millisecond precision** in foreign files: `getch` truncates to
  milliseconds. Sub-ms precision is not preserved.
* **Foreign files whose first chapter does not start at `00:00:00`**, or whose
  chapter set is non-monotonic / has duplicate timestamps. `getch` is a
  *faithful dumper*: it extracts and emits whatever timestamps FFprobe reports,
  **without normalizing or "fixing"** them. Such output may be rejected by
  `addch`'s validator on re-import. The user is responsible for editing the
  extracted text to satisfy the validation rules in [§6](#6-validation-rule-set).
* **Timestamps with `HH > 99`** emitted by `getch` (possible for very long
  foreign media) re-importing into `addch`, which caps hours at 99.
* **Byte-equality** of the text files — only chapter-list equality is
  guaranteed ([§10.1](#101-txt-semantic-round-trip-grammar-guaranteed)), and
  media-level equality is verified only for the containers proven by the Phase
  2 integration-test suite
  ([§10.2](#102-mediaffprobe-round-trip-contract-requirement--phase-2-verified)).
* **Invalid-UTF-8 or non-UTF-8 byte sequences**: no guarantee is made that such
  content round-trips through the media layer (R1–R3), nor that it round-trips
  through canonical serialization (see [§10.1](#101-txt-semantic-round-trip-grammar-guaranteed),
  which is scoped to well-formed UTF-8). The parser itself is byte-transparent
  and preserves such bytes verbatim
  ([§1](#1-overview-and-encoding)); all normative guarantees are scoped to
  well-formed UTF-8.

---

## 11. Compliance and Invariants for Implementations

Any implementation of this grammar MUST:

1. Accept and ignore a leading UTF-8 BOM on input; never emit a BOM on output.
2. Accept LF and CRLF on input; emit LF on output.
3. Treat the **timestamp/title delimiter as ASCII space and tab only**;
   Unicode whitespace must never act as a delimiter.
4. Apply **trimming of titles and the blank-line test using the full Unicode
   whitespace set**, so titles never begin or end with any whitespace, and
   whitespace-only lines are skipped.
5. Perform **no encoding validation on input**; parse byte-wise and preserve
   title bytes verbatim (normative guarantees are scoped to well-formed UTF-8).
6. Report content parse errors (structurally invalid timestamp, missing
   title) with the 1-based line number of the offending line; file I/O and
   scanner/token-size errors may be reported without a line number.
7. Reject a file with zero chapters.
8. Enforce exactly two ASCII digits per timestamp component, the `00`–`99` /
   `00`–`59` / `00`–`59` ranges, and a fractional part of one or more ASCII
   digits (of which only the first three are significant).
9. Truncate (never round) fractional precision beyond 3 digits, but reject any
   non-digit in the fractional part.
10. Reject duplicate, non-increasing, and non-zero-first start timestamps, as
    well as titles ending in a backslash.
11. Apply the duration check against the specific target video only at embed
    time.
12. Emit titles verbatim with no escaping, and never invent escape syntax.
13. Treat the media/FFprobe round-trip as established **only for containers
    proven by the Phase 2 integration-test suite**; the proof lives in the
    real-FFmpeg/FFprobe test coverage referenced by
    `FFMPEG_VERIFICATION.md`, not in this grammar document.

---

## 12. Version Notes

* **v0.1.0 (historical):** this grammar was fully implemented by the `addch`
  parser, validator, and output formatting.
* **v0.2.0 (approved — Phase-2-verified):** the grammar is **unchanged**. `rmch`
  and `getch` consume and (for `getch`) produce the same format. The `getch`
  producer behavior in [§9](#9-canonical-output-getch--contract) and the
  media/FFprobe round-trip contract in
  [§10.2](#102-mediaffprobe-round-trip-contract-requirement--phase-2-verified)
  were **contract requirements** at approval time and are now **verified
  shipping behavior**, proven by the real-FFmpeg/FFprobe integration-test suite
  (see `FFMPEG_VERIFICATION.md`).

Any proposed change to the syntax (e.g., new timestamp forms, comment syntax,
an escape language, multi-line titles, a non-ASCII delimiter) is a breaking
change to this document and MUST be versioned and signed-off as a separate
grammar revision.

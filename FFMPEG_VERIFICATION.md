# FFmpeg Chapter Round-Trip Verification

Empirical evidence from the committed integration tests, run against real
FFmpeg / FFprobe. This document records only what those tests verified.

## Scope of evidence

The round-trip evidence lives in `integration_test.go`:

- `TestChapterRoundTripMP4AndMKV` (commit `992a226`): embeds a small real
  fixture in the MP4 and Matroska (MKV) containers, builds the accompanying MKV
  fixture by transmuxing (stream copy), and asserts the round-trip via FFprobe.
- `TestChapterRoundTripM4A` (commit `1c1985a`): embeds a small real M4A/AAC
  audio fixture (no video), synthesized deterministically from a lavfi sine
  input, and asserts the round-trip via FFprobe.
- `TestRoundTripParseEqualsProbe` (commit `236078f`): the Phase 2E round-trip
  proof. For each supported container it starts from one adversarial chapter
  TXT fixture, parses it with the production parser, embeds it through the real
  FFmpeg path, probes the result with real FFprobe, converts the FFprobe
  chapters back into the project's chapter model inside the test, and compares
  them at millisecond-level semantic precision. See "Round-trip proof
  (Phase 2E)" below.

## How to reproduce

```
go test -run '^TestChapterRoundTripMP4AndMKV$' -v
```

Both subtests pass. Observed output:

```
[mp4] chapter time_base = "1/1000"; source duration = 10000 ms
[mkv] chapter time_base = "1/1000000000"; source duration = 10023 ms
```

## Verified title categories

The embedded chapter set exercised the following title content end-to-end in
both containers:

- Arabic / Unicode text (مقدمة, حروف, العربية)
- Spaces (including leading-positional interior spaces)
- `=` `;` `#`
- Double quotes
- Non-final literal backslashes (`Back\slash`)
- Literal two-character sequences `\n`, `\t`, `\r`

Each title was returned by FFprobe with exact string equality for the tested
valid UTF-8 titles, including the literal escape-lookalike sequences and the
Unicode text.

The literal two-character sequences `\n`, `\t`, and `\r` were verified in the
MP4 and MKV tests, and are covered for M4A as well by the Phase 2E round-trip
proof (`TestRoundTripParseEqualsProbe`; see "Round-trip proof (Phase 2E)"
below), which embeds the same literal sequences in all three supported
containers.

## Verified invariants

For each container (MP4 and MKV), the test asserts:

- The exact chapter count is preserved.
- Every chapter title matches exactly.
- Every chapter start is preserved after converting the raw value through the
  chapter's reported `time_base` (MP4 raw value == milliseconds; MKV raw value
  is in nanoseconds).
- Each chapter's end equals the next chapter's start, converted through the
  same `time_base` logic.
- The final chapter's end agrees with the actual probed source-media duration
  within 1 ms (mirroring the production verifier's `toleranceMs`).
- The source media file remains byte-identical after embedding (copy-out
  safety).

## Key results

| Container | Chapter time base | Observed source duration |
|-----------|-------------------|--------------------------|
| MP4       | `1/1000`          | `10000 ms`               |
| MKV       | `1/1000000000`    | `10023 ms`               |
| M4A/AAC   | `1/1000`          | `10000 ms`               |

## M4A/AAC evidence

Reproduce:

```
go test -run '^TestChapterRoundTripM4A$' -v
```

The test passes. Observed output:

```
m4a chapter time_base = "1/1000"; source duration = 10000 ms
```

Title coverage verified in M4A: Arabic/Unicode text, spaces, `=` `;` `#`,
double quotes, and non-final literal backslashes.

M4A invariants verified via FFprobe:

- The exact chapter count is preserved.
- Every chapter title matches exactly.
- Chapter starts are `0`, `2500`, `5250`, and `8000` ms after conversion through
  each returned chapter's `time_base`.
- Each chapter's end equals the next chapter's start.
- The final chapter's end agrees with the actual probed M4A source duration
  within 1 ms (`toleranceMs`).
- The source M4A remains byte-identical after embedding (copy-out safety).

## Round-trip proof (Phase 2E)

`TestRoundTripParseEqualsProbe` (commit `236078f`) proves that, for every
currently supported container — MP4, MKV, and M4A/AAC — the production pipeline
round-trips semantic chapter data:

```text
Parse(TXT) == FFprobe(FFmpeg(TXT))
```

The test uses one shared adversarial fixture and runs the full real toolchain:

1. Start from a valid chapter TXT fixture (per `GRAMMAR.md`): Arabic/Unicode,
   spaces, `=`, `;`, `#`, double quotes, non-final literal backslashes, the
   literal two-character sequences `\n`, `\t`, `\r`, a first chapter at
   `00:00:00`, and fractional timestamps through the supported duration
   boundary.
2. Parse that fixture with the production parser/model.
3. Embed the parsed chapters using the real FFmpeg path (the production addch
   pipeline, stream copy).
4. Probe the resulting media with real FFprobe (`-show_chapters`).
5. Convert the FFprobe chapter representation back into the project's chapter
   model inside the test only, normalizing each raw start/end through the
   chapter's reported `time_base` to milliseconds.
6. Compare the parsed input chapters against the FFprobe-derived chapters at
   the supported semantic precision.

Reproduce:

```
go test -run '^TestRoundTripParseEqualsProbe$' -v
```

Observed output (each subtest passes):

```
[mp4] chapter time_base = "1/1000"; source duration = 10000 ms
[mkv] chapter time_base = "1/1000000000"; source duration = 10023 ms
[m4a] chapter time_base = "1/1000"; source duration = 10000 ms
```

The comparison is semantic, not raw FFprobe JSON:

- Chapter count is preserved.
- Chapter order is preserved.
- Start timestamps match after normalization to milliseconds.
- End timestamps match where meaningful: each chapter ends where the next one
  starts, and the final chapter ends at the probed source-media duration,
  compared within the production verifier's `toleranceMs` (1 ms).
- Title strings match exactly for the tested valid UTF-8 titles, including the
  Arabic/Unicode, punctuation, quote, and literal backslash / `\n` / `\t` /
  `\r` coverage listed above.
- Timestamps are compared at millisecond-level precision; container-specific
  raw `time_base` values are never compared directly.

This proves the project's own supported round-trip behavior for media generated
through the addch path. It is not a claim about arbitrary foreign media:
chapter structures produced by external software that `addch` cannot re-import
are out of contract per `GRAMMAR.md` §10.3.

## Implementation conclusions

- Never assume raw chapter time units are milliseconds. MP4 happened to store
  raw values that equal milliseconds, but MKV stores values in nanoseconds.
  Values must always be compared after conversion through the reported
  `time_base`.
- The final chapter end must be derived from and asserted against the actual
  probed media duration, not the requested fixture duration. The MKV fixture
  was requested at 10 seconds but probed at `10023 ms` due to container/stream
  timing behavior observed in this fixture; asserting against the nominal
  10000 ms would fail. This is why the test reads the source duration with the
  same probe used by production (`getVideoDurationMs`) and compares with a
  1 ms tolerance.

## Limitations

- addch chapter embedding is empirically tested in MP4, MKV, and M4A/AAC only.
- No claim is made for MP3, FLAC, OGG, or any other audio or video container.
- No `getch` extraction round-trip validation exists yet.
- No `rmch` validation exists yet.
- These results do not expand the supported-container matrix beyond the
  empirically tested cases; untested containers remain unverified.

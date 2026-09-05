# FFmpeg Chapter Round-Trip Verification

Empirical evidence from the committed integration tests, run against real
FFmpeg / FFprobe. This document records only what those tests verified.

## Scope of evidence

Both tests live in `integration_test.go`:

- `TestChapterRoundTripMP4AndMKV` (commit `992a226`): embeds a small real
  fixture in the MP4 and Matroska (MKV) containers, builds the accompanying MKV
  fixture by transmuxing (stream copy), and asserts the round-trip via FFprobe.
- `TestChapterRoundTripM4A` (commit `1c1985a`): embeds a small real M4A/AAC
  audio fixture (no video), synthesized deterministically from a lavfi sine
  input, and asserts the round-trip via FFprobe.

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
MP4 and MKV tests only; they are not independently claimed for M4A.

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

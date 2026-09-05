# FFmpeg Chapter Round-Trip Verification

Empirical evidence recorded after running the committed Phase 2A integration
test `TestChapterRoundTripMP4AndMKV` (commit `992a226`) against real FFmpeg /
FFprobe. This document records only what that test verified.

## Scope of evidence

The sole source is the test `TestChapterRoundTripMP4AndMKV` in
`integration_test.go`. It embeds a small real fixture in the MP4 and Matroska
(MKV) containers, builds the accompanying MKV fixture by transmuxing
(stream copy), then asserts the round-trip via FFprobe.

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

- Only MP4 and MKV were tested.
- No audio-container claim is made (e.g., no test of M4A, MP3, FLAC, OGG, or
  similar).
- No `getch` extraction round-trip validation exists yet.
- No `rmch` validation exists yet.
- These results do not expand the supported-container matrix beyond the
  empirically tested cases; untested containers remain unverified.

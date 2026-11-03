# AGENTS.md — toml

## Summary

Go library that decodes TOML 1.1.0 documents into Go values, standard library
only. Replaces `github.com/BurntSushi/toml` across nlink-jp's Go tools. Strict
by default (unknown keys are an error), decode only. Module path:
`github.com/nlink-jp/toml`.

Status: v0.1.0 released. The parser passes all 681 toml-test documents for
1.1.0, and Decode/DecodeFile map the tree onto Go values. v1.0.0 follows once
a few consumers have migrated and the API has settled (RFP §4); each
consumer's migration is that consumer's own change (organization ADR-025 §6).
The decoding rules are `docs/en/decoding.md` — keep it in step with
`decode.go` and `fields.go`.

## Build & Test

```bash
make test    # go test ./... — includes the conformance suite and the data checksum
make vet     # go vet ./...
make build   # compiles the packages; there is no binary
go test -run '^$' -fuzz FuzzParse -fuzztime 60s    # fuzz the parser
go test -run '^$' -fuzz FuzzDecode -fuzztime 60s   # fuzz the decoder
```

## Structure

```
toml/
├── doc.go               # package documentation
├── decode.go            # Decode, DecodeFile, Option, MetaData, Unmarshaler; tree → Go values
├── fields.go            # struct field ↔ key matching (BurntSushi's rules), cached per type
├── datetime.go          # LocalDateTime, LocalDate, LocalTime; datetime → time.Time
├── tree.go              # node, table, tableState: the parsed tree and how each table came to exist
├── parse.go             # document, headers, keys, arrays, inline tables; the definition rules; limits
├── scalar.go            # strings, escapes, numbers, datetimes
├── errors.go            # ParseError, DecodeError, UnknownKeysError, Key; offset → line/column
├── decode_test.go       # every decoding rule
├── compat_test.go       # the consumers' patterns (pcap-analyzer, task-clock, lagent, voice-studio)
├── conformance_test.go  # toml-test harness (our own) + TestTheConformanceDataIsUnchanged
├── parse_test.go        # decisions the suite does not cover, limits, positions
├── fuzz_test.go         # FuzzParse and FuzzDecode, seeded with the suite
├── module_test.go       # TestTheModuleRequiresNothing
├── testdata/toml-test/  # vendored toml-test v2.2.0 data; SOURCE.md, SHA256SUMS
└── docs/{en,ja}/        # RFP, decoding rules
```

## Gotchas

- **`go 1.23`, no `require`.** Consumers are on 1.23 or later and adopt this
  library to drop a third-party dependency.
- **Field matching follows `BurntSushi/toml`** (tag first, then the field name
  case-insensitively; embedded structs flattened) so migration does not change
  what a config file means. This is a decision in the RFP, not an accident.
- **Decoding goes on top of what the value holds.** Structs, set pointers and
  map entries keep what the document does not set; slices and Go arrays are
  replaced. Consumers rely on it (defaults first, file second), and
  `TestMapEntriesAreDecodedOnTopOfTheirDefaults` exists because a mutation
  that dropped the old map entry passed every other test.
- **The order of checks in `decoder.value` matters:** pointers (at most 64
  deep, for `type P *P`), then `Unmarshaler`, then `any`, then the datetime
  types and `time.Duration` (an `int64` kind, so it must come before the
  integer case for strings), then `TextUnmarshaler`. `time.Time` is itself a
  `TextUnmarshaler`, so the datetime case must come first.
- **`Unmarshaler` was added in Phase 2, beyond the RFP's API,** because
  task-clock's `CommandValue` implements `UnmarshalTOML(any)`.
- **Unknown keys are collected, not failed on,** so one error lists them all,
  in document order, after every known key is decoded. An unknown table is
  listed with every key beneath it (`unknownTree`): video-studio-mcp and
  voice-studio-mcp find `workspace.workspace_dir` by name after `[workspace]`
  was removed. A `DecodeError` stops decoding at once.
- **Two keys that fold to one field are a `DecodeError`** (`setBy` in
  `structTable`); letting the later win dropped a value silently.
- **Every rule has a test that fails without it.** `rules_test.go` holds the
  ones the Phase 2 review removed unnoticed (13 mutations); when adding a
  rule, check that removing it fails a test.
- **What the migration needs is in docs/en/decoding.md, "Where this differs".**
  The Phase 2 review built every consumer against this library: about nine
  build their own unknown-key messages and need an `errors.As` rewrite, and
  seven have tests that compare BurntSushi's wording.
- **The table rules live in `tableState`.** implicit (a header's parent) →
  may still be defined by a header or by dotted keys; header → closed to
  headers and dotted keys; dotted → open only to dotted keys of the same
  section; inline → sealed. Each header and each inline table is a section.
- **Dotted keys may write into an implicit table** (`[a.b.c]` then `[a]`,
  `b.d = 1`). toml-test has no document on this; the spec lets one write to a
  key not yet directly defined. `TestDottedKeysMayDefineATableAHeaderOnlyImplied`
  pins it — a mutation forbidding it passed the whole suite.
- **The conformance harness is ours, its data is theirs.** `files-toml-1.1.0`
  picks the tests; the count is pinned at 681 so a shorter list cannot pass
  silently. Values are compared by meaning (floats numerically, NaN = NaN,
  datetimes as instants *and* by the offset as written — instants alone let a
  parser that rewrote every offset as Z pass). `TestTheComparisonTellsValuesApart`
  tests the comparison itself.
- **Invalid documents are checked for rejection, not for the reason.** The
  suite names no reason. The Phase 1 review read all 467 messages and found
  none rejected for the wrong reason; a regression there would not be caught.
- **Errors leave the descent by panic(parseFail)**, which `parse` recovers.
  Any other panic is re-raised: it is a bug, and the fuzzer must see it.
- **Positions are offsets until an error is reported.** `position` turns one
  into a line and a character column (not bytes) only then.
- **Limits:** 1 MiB per document — a node costs about 85 times the bytes that
  wrote it (16 MiB of `0,` built a 1.3 GiB tree), so the input bound is the
  memory bound. Arrays/inline tables nest at most 128 deep while parsing; a
  key has at most 128 parts; the finished tree is checked (without recursion,
  `tooDeep`) for depth 128, because headers and dotted keys add depth across
  lines. Decoding recurses, so the bound is what keeps a hostile document from
  exhausting the stack. `tooDeep` reports the earliest too-deep place, never
  the first a map yields.
- **Messages name keys from the root** (`p.path` is the current header's key);
  inside an inline table they are named from that table. Text quoted from the
  document goes through `clip`, so a message stays small whatever the input.
- **Implementation choices the spec leaves open:** CRLF inside multi-line
  strings becomes LF; a leading BOM is skipped (and is not a column); seconds
  of 60 (leap seconds) are read, and what they become as Go values is Phase 2's
  decision; fractions beyond nanoseconds are truncated; a float that overflows
  (`1e400`) is an error, one that underflows (`1e-400`) is 0.

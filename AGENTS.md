# AGENTS.md — toml

## Summary

Go library that decodes TOML 1.1.0 documents into Go values, standard library
only. Replaces `github.com/BurntSushi/toml` across nlink-jp's Go tools. Strict
by default (unknown keys are an error), decode only. Module path:
`github.com/nlink-jp/toml`.

Status: Phase 1 done — the parser reads TOML 1.1.0 into a tree with positions
and passes all 681 toml-test documents for 1.1.0. Phase 2 (mapping onto Go
values, the public API) has not started; see `docs/ja/toml-rfp.ja.md` §4.

## Build & Test

```bash
make test    # go test ./... — includes the conformance suite and the data checksum
make vet     # go vet ./...
make build   # compiles the packages; there is no binary
go test -run '^$' -fuzz FuzzParse -fuzztime 60s   # fuzz the parser
```

## Structure

```
toml/
├── doc.go               # package documentation
├── tree.go              # node, table, tableState: the parsed tree and how each table came to exist
├── parse.go             # document, headers, keys, arrays, inline tables; the definition rules; limits
├── scalar.go            # strings, escapes, numbers, datetimes
├── errors.go            # ParseError, offset → line/column
├── conformance_test.go  # toml-test harness (our own) + TestTheConformanceDataIsUnchanged
├── parse_test.go        # decisions the suite does not cover, limits, positions
├── fuzz_test.go         # FuzzParse, seeded with the suite
├── module_test.go       # TestTheModuleRequiresNothing
├── testdata/toml-test/  # vendored toml-test v2.2.0 data; SOURCE.md, SHA256SUMS
└── docs/{en,ja}/        # RFP
```

## Gotchas

- **`go 1.23`, no `require`.** Consumers are on 1.23 or later and adopt this
  library to drop a third-party dependency.
- **Field matching follows `BurntSushi/toml`** (tag first, then the field name
  case-insensitively; embedded structs flattened) so migration does not change
  what a config file means. This is a decision in the RFP, not an accident.
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
  datetimes as instants), as toml-test's README specifies.
- **Errors leave the descent by panic(parseFail)**, which `parse` recovers.
  Any other panic is re-raised: it is a bug, and the fuzzer must see it.
- **Positions are offsets until an error is reported.** `position` turns one
  into a line and a character column (not bytes) only then.
- **Limits:** 16 MiB per document; arrays/inline tables nest at most 128 deep
  while parsing; a key has at most 128 parts; the finished tree is checked
  (without recursion, `tooDeep`) for depth 128, because headers and dotted
  keys add depth across lines. Decoding recurses, so the bound is what keeps
  a hostile document from exhausting the stack.
- **Implementation choices the spec leaves open:** CRLF inside multi-line
  strings becomes LF; a leading BOM is skipped; seconds of 60 (leap seconds)
  are rejected; fractions beyond nanoseconds are truncated; a float that
  overflows (`1e400`) is an error.

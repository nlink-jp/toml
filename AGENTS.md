# AGENTS.md — toml

## Summary

Go library that decodes TOML 1.1.0 documents into Go values, standard library
only. Replaces `github.com/BurntSushi/toml` across nlink-jp's Go tools. Strict
by default (unknown keys are an error), decode only. Module path:
`github.com/nlink-jp/toml`.

Status: scaffold. Phase 1 (parser + conformance suite) has not started; see
the development plan in `docs/ja/toml-rfp.ja.md` §4.

## Build & Test

```bash
make test    # go test ./...
make vet     # go vet ./...
make build   # compiles the packages; there is no binary
```

## Structure

```
toml/
├── doc.go          # package documentation
├── module_test.go  # TestTheModuleRequiresNothing
└── docs/{en,ja}/   # RFP
```

## Gotchas

- **`go 1.23`, no `require`.** Consumers are on 1.23 or later and adopt this
  library to drop a third-party dependency.
- **Field matching follows `BurntSushi/toml`** (tag first, then the field name
  case-insensitively; embedded structs flattened) so migration does not change
  what a config file means. This is a decision in the RFP, not an accident.

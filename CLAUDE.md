# CLAUDE.md — toml

Organization rules: https://github.com/nlink-jp/.github/blob/main/CONVENTIONS.md
Read AGENTS.md first, then the RFP (`docs/ja/toml-rfp.ja.md`).

## Non-negotiable rules

- **Standard library only.** `go.mod` gains no `require` line, not even for
  tests; `TestTheModuleRequiresNothing` holds this. Never use another TOML
  library as a reference implementation.
- **Strict by default.** An unknown key is an error unless the caller passed
  `AllowUnknownKeys()`. Never drop a key silently.
- **TOML 1.1.0, decode only.** No encoder, no version switch.
- **The conformance data is data.** `testdata/toml-test/` is vendored at a
  pinned commit with a SHA256 manifest; update it only as a whole, with the
  manifest, in its own commit. The harness that reads it is ours.
- **Never panic on input.** Malformed input is an error with a position.
- **Tests with every change**; docs in sync: README.md and README.ja.md in the
  same commit.

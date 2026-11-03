# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres
to [Semantic Versioning](https://semver.org/).

## [0.1.0] - Unreleased

### Added

- Repository scaffold and RFP (`docs/en/toml-rfp.md`, `docs/ja/toml-rfp.ja.md`).
- TOML 1.1.0 parser into a tree that keeps each value's position. Passes all
  681 toml-test v2.2.0 documents for 1.1.0 (214 valid, 467 invalid), with the
  suite's data vendored and checked against its SHA256 list on every run.
- Limits: 1 MiB per document, nesting depth 128, 128 parts per key.
- Error messages name keys from the root and quote at most 40 characters of
  the document.
- Leap seconds (second 60) are read, as RFC 3339 allows.
- Fuzz test of the parser, seeded with the suite.

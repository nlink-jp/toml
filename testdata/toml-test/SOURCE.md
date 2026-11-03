# toml-test conformance data

Vendored test data. Not code: nothing here is compiled or shipped.

| | |
|---|---|
| Origin | https://github.com/toml-lang/toml-test (the TOML project's own suite) |
| Release | v2.2.0 (published 2026-04-30) |
| Commit | `ce08da1ddb075d1c7596d663c7fcba9a2ae02c5c` |
| Taken | `tests/` in full and `LICENSE` (MIT, copyright TOML authors) |
| Vendored | 2026-11-03 |

## How it was verified

The commit was fetched by its hash into an empty directory. Every one of the
commit's 1,043 files was hashed with `git hash-object --no-filters` and compared
with the blob hash GitHub's tree API returned for that commit; all matched.
Only `tests/` and `LICENSE` were then copied here.

`SHA256SUMS` lists every file in this directory except itself and this note,
paths relative to this directory, sorted bytewise. `TestTheConformanceDataIsUnchanged`
checks it on every test run.

## Which tests apply

`tests/files-toml-1.1.0` lists the files that apply to TOML 1.1.0; the harness
runs exactly those. The `*.multi` files are the suite's generator sources and are
not run directly.

## Updating

Replace the whole directory from a new pinned commit, verify it the same way,
regenerate `SHA256SUMS`, update this note, and commit it on its own.

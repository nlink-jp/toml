# RFP: toml

> Generated: 2026-11-03
> Status: Draft

## 1. Problem Statement

nlink-jp's Go tools read the hand-written `~/.config/<tool>/config.toml` through a TOML library written outside the organization (`github.com/BurntSushi/toml`). As of 2026-11-03, 29 active repositories depend on it. That contradicts the organization's supply-chain policy (no third-party dependencies; the standard library, first-party SDKs and direct REST calls only — ADR-024). One repository (mcp-bridge) chose JSON for its configuration to avoid the dependency, departing from the organization's sectioned-TOML convention. Strict loading, which rejects unknown keys, is also written separately in each repository on top of `Undecoded()`, so the implementations vary.

This library implements TOML 1.1.0 decoding with the Go standard library only and makes strict loading the default. This removes the external TOML dependency from the organization's Go tools. Its users are the developers of the organization's Go tools, and the people who ultimately benefit are the users who write the configuration files.

## 2. Functional Specification

### Commands / API Surface

```go
package toml // github.com/nlink-jp/toml

func Decode(data string, v any, opts ...Option) (MetaData, error)
func DecodeFile(path string, v any, opts ...Option) (MetaData, error)

type MetaData struct{ /* … */ }
func (MetaData) IsDefined(key ...string) bool

type Key []string // String() returns "a.b.c"

type Option func(*config)
func AllowUnknownKeys() Option // explicitly opts out of strict loading

// Every error can be extracted with errors.As
type ParseError struct{ Line, Column int; Msg string }  // syntax error, with position
type UnknownKeysError struct{ Keys []Key }               // unknown keys, all of them
type DecodeError struct{ Key Key; Line int; Msg string } // type mismatch or overflow
```

- `Decode` and `DecodeFile` take the same arguments as `BurntSushi/toml`. Migrating a repository then means changing the import and deleting its unknown-key check.
- There is no `Undecoded()`. When a document has unknown keys, the decode itself fails with `UnknownKeysError`. Callers take the list of keys from that error and build their own message or error code from it.
- Strict loading is the default. A caller gets lenient loading only by passing `AllowUnknownKeys()`.

### Input / Output

- Input: a string in memory, or a file path. The document is TOML 1.1.0 (a valid 1.0.0 document reads unchanged as 1.1.0).
- Output: Go values. Supported targets:
  - string, bool, integers (int and uint kinds; overflow is an error), floats (an integer may be decoded into a float)
  - slices, `map[string]T`, structs (nested; fields of an embedded struct are treated as fields of the outer struct), pointers, `any` (decoded into `map[string]any` and `[]any`)
  - types that implement `encoding.TextUnmarshaler`
  - the four datetime kinds (offset date-time, local date-time, local date, local time). Which Go types receive them is decided in the Phase 2 design.
- Values are decoded on top of whatever the target already holds. A key the document does not set keeps its default. A slice is replaced as a whole; a map has only the keys the document sets overwritten.
- Struct fields are matched to keys as in `BurntSushi/toml`: the tag name first, and with no tag, the field name compared case-insensitively. Tag options such as `,omitempty` are ignored.
- Input size and nesting depth are bounded. The exact limits are set in the design.
- There is no encoding (Encoder / Marshal).

### Configuration

None. The library has no configuration of its own.

### External Dependencies

None. The library uses the Go standard library only. No third-party module appears in go.mod, not even for tests.

## 3. Design Decisions

- **Go.** Every consumer is written in Go. Python tools are out of scope because the standard library's `tomllib` already serves them.
- **TOML 1.1.0.** The current `BurntSushi/toml` v1.6.0 accepts TOML 1.1.0 by default. A 1.0-only library would fail, after migration, on existing configuration files that use 1.1 syntax. TOML 1.1 adds four things: optional seconds in times, the `\xHH` and `\e` escapes, newlines inside inline tables, and trailing commas in inline tables.
- **Two-stage decoding.** The document is first parsed into a tree that carries source positions, and that tree is then mapped onto Go values. Unknown-key detection, `IsDefined` and line-numbered errors all come from the same tree.
- **Strict by default.** This prevents typos in hand-written configuration from being silently ignored, in line with the organization's strict-decode practice.
- **Conformance data from toml-lang/toml-test.** The suite is a collection of about 1,000 valid and invalid documents; it is data, not code. It is vendored into `testdata/` at a pinned commit, together with a SHA256 manifest of every file and its license notice. It is read only at test time and never enters a shipped artifact or go.mod, so it is not "shipping code written outside the organization", which ADR-024 forbids. The test harness that compares against it is written in-house.
- **Fuzzing.** Go's built-in fuzzing confirms that malformed input never causes a panic.
- **Place in the organization.** The library lives in lib-series and, like pathguard, serves as a foundation for the organization's Go tools. Once migration is done, the reason mcp-bridge chose JSON no longer holds. Whether mcp-bridge moves back to TOML is a separate decision.

### Out of Scope

- Encoding (Encoder / Marshal)
- Comment-preserving editing or rewriting
- Streaming decode from an `io.Reader`
- Switching between TOML versions other than 1.1.0
- A validation CLI
- Python support, including removing `tomli`
- Using `BurntSushi/toml` as a reference implementation during development
- Migrating the existing 29 repositories (planned separately as follow-up work)

## 4. Development Plan

### Phase 1: Core

Build the parser (document → tree with source positions).

- Accept the full TOML 1.1.0 syntax.
- Vendor the toml-test data at a pinned commit, with its SHA256 manifest and license notice.
  - Valid documents: compare the parse result with the expected value (typed JSON), read with the standard `encoding/json`.
  - Invalid documents: confirm that every one is rejected with a positioned error.
- Add fuzzing, and add the input-size and nesting-depth limits.
- **Review checkpoint:** the whole conformance suite passes.

### Phase 2: Features

Map the tree onto Go values and build the public API.

- `Decode`, `DecodeFile`, `MetaData.IsDefined`, `AllowUnknownKeys()` and the three error types.
- Type mapping, decoding on top of defaults, pointers, `any`, `TextUnmarshaler`, overflow detection.
- Decide which Go types receive the four datetime kinds, and record the decision in a design document.
- Compatibility tests that reproduce how the existing repositories use the current library:
  - decoding into a struct pre-filled with defaults
  - `*int` and `*bool` fields
  - `Duration` (a `TextUnmarshaler`)
  - showing where a setting came from through `IsDefined`
  - building a tool-specific error from the list of unknown keys
- **Review checkpoint:** an independent verification pass by a subagent. Findings are triaged before they are acted on, never applied automatically.

### Phase 3: Release

- Write README.md and README.ja.md, AGENTS.md and CHANGELOG.md.
- Record in organization ADR-026 that TOML is read with `nlink-jp/toml`.
- Release v0.1.0. Move to v1.0.0 after a few repositories have migrated and the API has settled.
- Register the library as a lib-series submodule, update the org profile, pass check-org, and feed what was learned back into the knowledge repository.

### Follow-up (outside this RFP): migrating the existing repositories

- Plan the change across the 29 repositories as one sweep and split the work. Following ADR-025, change each repository from inside it.
- Some repositories currently skip unknown keys silently: `DecodeFile` appears 31 times, but the unknown-key check only 19 times. Decide per repository whether to load strictly or to pass `AllowUnknownKeys()`. Going strict is a breaking change and is stated in that repository's release notes.
- Release each repository separately.

## 5. Required API Scopes / Permissions

None. The library uses no external service and no credentials.

## 6. Series Placement

Series: lib-series
Reason: It is a foundation library used across series, which is the same kind of library as pathguard. Work starts in `_wip/toml/`, and the library is registered as `lib-series/toml` at release.

## 7. External Platform Constraints

None. The minimum Go version is 1.23, matching pathguard.

---

## Discussion Log

- **Origin:** TOML loaders keep coming up in supply-chain reviews. The proposal is to implement one in-house as a proper library.
- **Survey (2026-11-03):**
  - 29 active Go repositories depend on an external TOML library (`BurntSushi/toml` v1.4.0–v1.6.0). In addition, `markdown-viewer` pulls in `pelletier/go-toml/v2` as an indirect dependency.
  - The Python tools use the standard `tomllib`; one place loads `tomli` as a fallback.
  - Nothing encodes TOML. The calls in use are `DecodeFile` 31, `Undecoded()` 19, `Decode` 4 and `IsDefined` 2.
- **Remaining dependencies:** Even without the TOML library, many small tools still depend on `spf13/cobra` and the packages it brings in. That was set aside as a separate topic.
- **Name:** A package named `toml` turns migration into changing an import. The repository is `nlink-jp/toml`.
- **API:** The arguments match the current library. `Undecoded()` is dropped; strict loading becomes the default, and `UnknownKeysError` returns the list of keys.
- **Repositories that load leniently today:** Breaking changes are confirmed before they ship, so the library defaults to strict loading, and during migration each repository decides whether to pass `AllowUnknownKeys()`.
- **Target version corrected:** The plan first assumed TOML 1.0. Inspecting v1.6.0 showed that it accepts 1.1.0 by default, so the target became 1.1.0 to avoid a regression.
- **Conformance data:** Option A (vendor the conformance data) was compared with option B (write every test in-house). A was chosen for coverage and for fewer gaps: only the data is vendored, pinned and with a hash manifest.
- **Release:** The library stays at v0.x until migration has settled the API.

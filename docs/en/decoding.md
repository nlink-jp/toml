# Decoding rules

How `Decode` and `DecodeFile` map a TOML document onto Go values. The RFP
(`toml-rfp.md`) left the datetime types to this document; the rest records
what the code does and why.

## Where values go

| TOML | Go target | Notes |
|---|---|---|
| string | `string` kinds | |
| string | `time.Duration` | Read by `time.ParseDuration`: `"1m30s"`. |
| string | `time.Time` | When RFC 3339. The offset is kept, unnamed, as for a datetime. |
| integer | `int`/`uint` kinds | Out of range is a `DecodeError`; a negative value into a `uint` too. Into `time.Duration` it counts nanoseconds. |
| integer | `float32`/`float64` | Only when held exactly: `2^53 + 1` into `float64` is an error. |
| float | `float32`/`float64` | Overflow into `float32` is an error; `inf` and `nan` pass. Never into an integer. |
| bool | `bool` kinds | |
| offset date-time | `time.Time` | The offset is kept as written, unnamed; a zero offset is `time.UTC`. |
| local date-time | `toml.LocalDateTime` | Not `time.Time`: see below. |
| local date | `toml.LocalDate` | |
| local time | `toml.LocalTime` | `Second` may be 60. |
| array | slice | Replaced whole. |
| array | Go array | Up to its length; the rest is zeroed. Longer is an error. |
| table | struct | Field matching below. |
| table | `map[string]T` | Key type of any string kind. Existing entries are kept, and an entry the document writes is decoded on top of the one there. |
| any | `any` | `map[string]any`, `[]any`, `string`, `int64`, `float64`, `bool`, `time.Time`, `LocalDateTime`, `LocalDate`, `LocalTime`. |

A target whose pointer implements `toml.Unmarshaler` (`UnmarshalTOML(any)
error`) receives the value as an `any` target would, for a key whose value
may take more than one shape. An array of tables arrives as `[]any` of
`map[string]any`. task-clock's `command` (an argv array or a shell string) is
the case it exists for. Keys under it count as decoded.

A target whose pointer implements `encoding.TextUnmarshaler` receives a
string's text, or a datetime's text as TOML writes it. Other values are a
`DecodeError`.

## Datetimes

The three local kinds have no offset, so they do not name an instant. Putting
one into `time.Time` would mean choosing a zone, UTC or `time.Local`, without
the document saying so, and a wrong choice moves the value by hours without
an error. Each local kind therefore decodes into its own type. Decoding one
into `time.Time` is an error that names the right type. `LocalDateTime.In(loc)`
turns a local date-time into an instant once the caller has decided the zone.

No tool in the organization decoded a datetime when this was decided
(2026-11-03), so nothing depends on another choice.

An offset is kept as an unnamed fixed zone, never under `time.Local`'s name:
the document says `+09:00`, not `JST`. This holds for a string decoded into
`time.Time` too, although `time.Parse` alone would name the zone.

A leap second (second 60) is kept as 60 in `LocalTime`. In `time.Time`, and
through `In`, it becomes the first second of the next minute, as `time.Date`
normalises it.

## Defaults

The value given to `Decode` is decoded into, not replaced. A key the document
does not set keeps whatever the value held. This holds through set pointers,
nested structs and map entries. A nil pointer gets a new value only when the
document writes it, so a pointer field tells "set" from "absent". (If decoding
into that new value then fails, the pointer stays set; the error is returned.)
Slices and Go arrays are the exception: the document's array replaces them.

## Struct fields

This follows `BurntSushi/toml`, so that a tool moved to this library reads its
config file the same way:

1. The `toml:"name"` tag names the field. `toml:"-"` takes no key. Options
   after a comma (`,omitempty`) are ignored.
2. An exported field without a tag is named by its Go name. Unexported fields
   take no key.
3. A key that matches no name exactly may match one case-insensitively; the
   first such field in declaration order wins.
4. An untagged embedded struct, or pointer to struct, lends its fields to the
   outer struct. The shallowest field of a name wins. At equal depth a tagged
   field wins. A name still shared is ambiguous and takes no key. A nil
   embedded pointer is created when one of its fields is written.

TOML keys are case-sensitive, so `name` and `NAME` are two keys. When rule 3
leads two keys of one table to the same field, that is a `DecodeError`: one of
the two values would otherwise be dropped without a word.

## Unknown keys

A key the struct has no field for is collected, not decoded. A table the
struct has no field for is collected together with every key beneath it, so a
tool can still find `workspace.workspace_dir` after removing `[workspace]`.
When decoding finishes without any other error, every collected key is
returned in an `UnknownKeysError`, in document order. A key inside an array
of tables is named without an index and listed once. Every known key has been
decoded by then. `AllowUnknownKeys()` skips them instead.

Maps, `any` and `Unmarshaler` targets take every key. A struct inside a map
does not: `[servers.main] colour = "red"` into `map[string]Server` reports
`servers.main.colour` if `Server` has no such field.

## Errors

| Error | When | Carries |
|---|---|---|
| `*ParseError` | The document is not TOML 1.1.0 | Line, Column (characters), Msg |
| `*DecodeError` | A value does not fit its target | Key, Line, Msg; unwraps to the error of `UnmarshalText`, `UnmarshalTOML`, `time.Parse` or `time.ParseDuration` |
| `*UnknownKeysError` | Keys the value has no place for | Keys, in document order |

Decoding stops at the first `DecodeError`, so the value may hold part of the
document. Messages name keys from the root and stay small whatever the
document holds: a key part or token is quoted up to 40 characters, the error
of a target's own method up to 200 (the whole error stays reachable through
`errors.Unwrap`), and `UnknownKeysError` names at most ten keys. A pointer
type that points to itself (`type P *P`) is a `DecodeError`, not an endless
loop.

## Where this differs from BurntSushi/toml

Each difference was checked against the organization's tools on 2026-11-03.
Only the first two change what a migrating tool has to do; the rest touch
nothing any tool uses.

- **Unknown keys are an error by default**, where BurntSushi lists them in
  `Undecoded()` and carries on. There is no `Undecoded()`: a tool that built
  its own message from that list reads `UnknownKeysError.Keys` through
  `errors.As` instead. Pass `AllowUnknownKeys()` to carry on.
- **The messages are this library's own.** Tests that compare BurntSushi's
  wording change with the migration.
- **Two keys that fold to one field are an error.** BurntSushi lets the later
  one win.
- **A local datetime does not decode into `time.Time`.** BurntSushi gives
  it a placeholder zone; see Datetimes.
- **Only strings and datetimes reach a `TextUnmarshaler`.** BurntSushi
  also passes integers, floats and booleans to it as text. The only
  `TextUnmarshaler` types in the organization are durations, which reject
  a bare number under either library.
- **A slice is replaced whole.** BurntSushi decodes into the elements
  already there, so a default inside an element survives.
- **A map entry the document writes is decoded on top of the one there.**
  BurntSushi decodes each entry fresh.
- **A Go array takes up to its length.** BurntSushi requires the exact length.
- **An integer goes into a float only when held exactly.** BurntSushi accepts
  up to ±2^53 and refuses beyond; here 2^60 passes and 2^53 + 1 does not.
- **`map[any]T` is not a target.** Map keys must be of a string kind.
- **An `Unmarshaler` receives `[]any` for an array of tables** (BurntSushi:
  `[]map[string]any`), and the local types for local datetimes (BurntSushi:
  `time.Time`).
- **`DecodeFile` reads at most 1 MiB.**
- **No `Primitive`, `PrimitiveDecode`, `Keys()`, `Type()`, `NewDecoder`,
  `DecodeFS`, and no encoder.** No tool uses them.

## Limits

1 MiB per document (`DecodeFile` reads no further than that); tables and
arrays nest at most 128 deep; a key has at most 128 parts. The input bound is
also the memory bound: the parsed tree costs about 85 times the document's
size in the worst case.

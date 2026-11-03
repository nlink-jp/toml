# toml Reference Manual

> Version: 0.1.1

## Overview

`github.com/nlink-jp/toml` decodes TOML 1.1.0 documents into Go values. It uses
the Go standard library only and has no encoder.

```
go get github.com/nlink-jp/toml
```

Three things set its behaviour apart from a plain unmarshaller:

- **It decodes into the value you give it.** Fill a config struct with
  defaults, then decode the file on top: every key the file does not set keeps
  its default.
- **It is strict by default.** A key the value has no field for is an error
  (`UnknownKeysError`), never silently dropped.
- **It matches struct fields as `BurntSushi/toml` does**, so a tool can move to
  it without its config files changing meaning.

This manual describes every exported name. The full rules — which TOML value
fits which Go type, how fields match keys, and every difference from
`BurntSushi/toml` — are in [decoding.md](decoding.md).

```go
import (
	"errors"

	"github.com/nlink-jp/toml"
)

cfg := defaultConfig()
md, err := toml.DecodeFile(path, &cfg)
var unknown *toml.UnknownKeysError
switch {
case errors.As(err, &unknown):
	// unknown.Keys: every key cfg has no field for; everything else is decoded
case err != nil:
	// a *toml.ParseError, a *toml.DecodeError, or the error of opening the file
}
if md.IsDefined("model", "name") {
	// the file set model.name
}
```

---

## Functions

### `Decode(data string, v any, opts ...Option) (MetaData, error)`

Decodes the TOML document `data` into `v`, which must be a non-nil pointer.

- Values already in `v` that the document does not set are kept. This holds
  through nested structs, pointers that are already set, and map entries.
  Slices and Go arrays are replaced by the document's array.
- A nil pointer gets a new value only when the document writes it, so a
  pointer field tells "set" from "absent".
- The error is one of:
  - `*ParseError` — `data` is not TOML 1.1.0.
  - `*DecodeError` — a value does not fit its target. Decoding stops there, so
    `v` may hold part of the document.
  - `*UnknownKeysError` — the document has keys `v` has no place for. Returned
    only when nothing else went wrong; every known key has been decoded.
  - A plain error when `v` is not a non-nil pointer.
- `MetaData` is returned whenever the document parsed, including together with
  a `*DecodeError` or `*UnknownKeysError`.

```go
var cfg Config
md, err := toml.Decode(`name = "x"`, &cfg)
```

### `DecodeFile(path string, v any, opts ...Option) (MetaData, error)`

Reads the file at `path` and decodes it as `Decode` does. It reads no more than
1 MiB and a byte: a larger file is a `*ParseError` without being read whole. An
error opening or reading the file is returned as the `os` package returns it, so
`errors.Is(err, fs.ErrNotExist)` works.

```go
md, err := toml.DecodeFile(filepath.Join(home, ".config", "tool", "config.toml"), &cfg)
if errors.Is(err, fs.ErrNotExist) {
	// no config file: keep the defaults
}
```

---

## Options

### `type Option`

```go
type Option func(*config)
```

Changes how a document is decoded. Pass any number to `Decode` or `DecodeFile`.

### `AllowUnknownKeys() Option`

Skips keys the value has no place for instead of returning `UnknownKeysError`.
Use it where a document may carry settings for other readers, or to keep an
existing tool lenient while it migrates.

```go
_, err := toml.Decode(doc, &cfg, toml.AllowUnknownKeys())
```

---

## MetaData

### `type MetaData`

```go
type MetaData struct { /* unexported */ }
```

Describes the document that was decoded. The zero value describes no document.

### `(MetaData) IsDefined(key ...string) bool`

Reports whether the document wrote the key, one argument per key part. True for
a value, and for a table whether a header, dotted keys or a deeper header wrote
it. A key inside an array of tables is not reached. No arguments at all is
false; the single empty part `""` is a key and is true when the document wrote
`"" = …`.

```go
md.IsDefined("model", "name")                            // [model] name = …
md.IsDefined(strings.Split("sandbox.enabled", ".")...)  // a dotted name you hold
```

---

## Custom decoding

### `type Unmarshaler`

```go
type Unmarshaler interface {
	UnmarshalTOML(any) error
}
```

Implemented, on its pointer, by a type that decodes a TOML value itself — for a
key whose value may take more than one shape, such as a command given either as
an argv array or as one shell string. `UnmarshalTOML` receives the value as an
`any` target would:

| TOML | Go value received |
|------|-------------------|
| table | `map[string]any` |
| array (also an array of tables) | `[]any` |
| string | `string` |
| integer | `int64` |
| float | `float64` |
| boolean | `bool` |
| offset date-time | `time.Time` |
| local date-time / date / time | `LocalDateTime` / `LocalDate` / `LocalTime` |

Every key under such a value counts as decoded. An error it returns becomes a
`*DecodeError` that unwraps to it.

### `encoding.TextUnmarshaler`

Not part of this package, but honoured: a target whose pointer implements
`UnmarshalText` receives a string's text, or a datetime's text as TOML writes
it. Other values are a `*DecodeError`. Its error is wrapped as for
`Unmarshaler`. `time.Time` is a `TextUnmarshaler`, so an RFC 3339 string also
decodes into it.

### `time.Duration`

A string such as `"1m30s"` decodes into a `time.Duration` through
`time.ParseDuration`; an integer counts nanoseconds.

---

## Datetime types

An offset date-time decodes into `time.Time`, keeping the offset as written (an
unnamed fixed zone; a zero offset is `time.UTC`). The three local kinds name no
instant, so each has its own type. Decoding one into `time.Time` is a
`*DecodeError` that names the right type.

### `type LocalDate`

```go
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}
```

A day, with no time and no offset: `1979-05-27`.

#### `(LocalDate) String() string`

Writes the date as TOML does: `1979-05-27`.

### `type LocalTime`

```go
type LocalTime struct {
	Hour, Minute, Second int
	Nanosecond           int
}
```

A time of day, with no date and no offset: `07:32:00.5`. `Second` may be 60 (a
leap second). Seconds omitted in the document (`07:32`, as TOML 1.1 allows) are
0. A fraction beyond nanoseconds is truncated.

#### `(LocalTime) String() string`

Writes the time as TOML does: `07:32:00`, with a fraction only when there is
one.

### `type LocalDateTime`

```go
type LocalDateTime struct {
	Date LocalDate
	Time LocalTime
}
```

A date and a time, with no offset: `1979-05-27T07:32:00`.

#### `(LocalDateTime) String() string`

Writes the date-time as TOML does: `1979-05-27T07:32:00`.

#### `(LocalDateTime) In(loc *time.Location) time.Time`

The instant this date-time names in `loc`, once the caller has chosen the zone.
A leap second becomes the first second of the next minute, as `time.Date`
normalises it.

```go
start := cfg.Start.In(time.Local)
```

---

## Errors

Every error is reachable with `errors.As`, also through `fmt.Errorf("…: %w", err)`.
Messages stay small whatever the document holds: text quoted from the document
is cut at 40 characters, and an error from a target's own method at 200.

### `type ParseError`

```go
type ParseError struct {
	Line, Column int
	Msg          string
}
```

The document is not TOML 1.1.0: a syntax error, a key or table defined twice, a
value out of range, invalid UTF-8, or a document over the limits. `Line` and
`Column` are 1-based; `Column` counts characters, not bytes, and a leading byte
order mark is not one.

#### `(*ParseError) Error() string`

`toml: line L, column C: msg`

### `type DecodeError`

```go
type DecodeError struct {
	Key  Key
	Line int
	Msg  string
	// unexported: the wrapped error
}
```

A valid document whose value at `Key` does not fit its Go target: the wrong
type, a number out of range or not held exactly, two keys that differ only in
case and reach one field, or an error from the target's own `UnmarshalTOML` or
`UnmarshalText`, `time.Parse` or `time.ParseDuration`. `Line` is where the value
is written. `Key` is empty when the document as a whole does not fit (a table
decoded into an `int`).

#### `(*DecodeError) Error() string`

`toml: line L: key K: msg`, or `toml: line L: msg` when `Key` is empty.

#### `(*DecodeError) Unwrap() error`

The error of the target's own method, `time.Parse` or `time.ParseDuration`; nil
otherwise.

### `type UnknownKeysError`

```go
type UnknownKeysError struct {
	Keys []Key
}
```

Every key, in document order, that the value has no place for. A table with no
place is listed together with every key beneath it, so a key under a removed
table can still be found by its full name (`workspace` and
`workspace.workspace_dir`). A key inside an array of tables is named without an
index and listed once. Maps, `any` and `Unmarshaler` targets take every key; a
struct inside a map does not.

#### `(*UnknownKeysError) Error() string`

`toml: unknown key: K` for one key; `toml: unknown keys: K1, K2, …` for more,
naming at most ten and counting the rest (`and 3 more`).

### `type Key`

```go
type Key []string
```

The path to a value, one element per key part: `[model] name = …` is
`Key{"model", "name"}`. Arrays of tables add no index.

#### `(Key) String() string`

Writes the key as TOML would: parts joined by dots, each quoted only when it is
not a bare key — `model.name`, `characters."少女".pitch`. Compare it with the
dotted names your tool uses.

---

## Limits

| Limit | Value |
|-------|-------|
| Document size | 1 MiB |
| Nesting of tables and arrays | 128 levels |
| Parts in one key | 128 |

A document over a limit is a `*ParseError`. A pointer type that points to itself
(`type P *P`) is a `*DecodeError`.

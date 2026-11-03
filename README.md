# toml

A TOML 1.1.0 decoder for [nlink-jp](https://github.com/nlink-jp) tools.

A Go library, standard library only. It reads TOML documents into Go values
and is **strict by default**: a key the target has no place for is an error,
not silently dropped. It decodes only; there is no encoder.

> **Status: in development.** The API below is the planned one and is not
> implemented yet. See the [RFP](docs/en/toml-rfp.md).

## Install

```bash
go get github.com/nlink-jp/toml
```

## Usage (planned)

```go
import "github.com/nlink-jp/toml"

cfg := defaultConfig() // keys absent from the file keep these values
md, err := toml.DecodeFile(path, &cfg)
var unknown *toml.UnknownKeysError
if errors.As(err, &unknown) {
	// unknown.Keys lists every key the struct has no field for
}
if md.IsDefined("model", "name") {
	// the file set model.name
}
```

Pass `toml.AllowUnknownKeys()` to accept keys the target does not know.

## What it reads

- TOML 1.1.0 in full (1.0.0 documents read the same). It passes every
  document of the [toml-test](https://github.com/toml-lang/toml-test) suite
  for 1.1.0.
- Documents up to 1 MiB, with tables and arrays nested at most 128 deep.
- Errors give the line and the column (counted in characters).
- Dotted keys may write into a table that a header only implied: after
  `[a.b.c]`, a later `[a]` may say `b.d = 1`. The spec allows this, but some
  other TOML implementations read it more strictly, so such a file may not
  read elsewhere.

## Why

nlink-jp ships no code written outside the organization. Its Go tools read
`config.toml` through a third-party library; this one replaces it.

## License

MIT — see [LICENSE](LICENSE).

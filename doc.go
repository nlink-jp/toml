// Package toml decodes TOML 1.1.0 documents into Go values, using the
// standard library only.
//
// Decode and DecodeFile decode into the value they are given rather than
// replacing it, so a config struct filled with defaults keeps every default
// the document does not set:
//
//	cfg := defaultConfig()
//	md, err := toml.DecodeFile(path, &cfg)
//
// Decoding is strict by default: a key the value has no place for is an
// error (UnknownKeysError), never silently dropped. AllowUnknownKeys skips
// such keys instead. Every error is one of three types — ParseError,
// DecodeError, UnknownKeysError — reachable with errors.As.
//
// Struct fields match keys by the `toml:"name"` tag, then by the field name,
// then by the field name case-insensitively, as github.com/BurntSushi/toml
// matches them. There is no encoder.
//
// The full rules — which TOML value fits which Go type, how unknown keys are
// reported, and where this package differs from BurntSushi/toml — are in
// docs/en/decoding.md; every exported name is described in
// docs/en/reference.md.
package toml

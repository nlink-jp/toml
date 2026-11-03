// Package toml decodes TOML 1.1.0 documents into Go values, using the
// standard library only.
//
// Decoding is strict by default: a key the target has no place for is an
// error (UnknownKeysError), never silently dropped. There is no encoder.
// See docs/en/toml-rfp.md.
package toml

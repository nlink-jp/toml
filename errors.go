package toml

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ParseError is a document that is not valid TOML 1.1.0. Line and Column are
// 1-based; Column counts characters, not bytes.
type ParseError struct {
	Line, Column int
	Msg          string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("toml: line %d, column %d: %s", e.Line, e.Column, e.Msg)
}

// Key is the path to a value, one element per key part: [model] name = …
// is Key{"model", "name"}. Arrays of tables add no index.
type Key []string

// String writes the key as TOML would: parts joined by dots, each quoted
// only when it is not a bare key.
func (k Key) String() string {
	parts := make([]string, len(k))
	for i, p := range k {
		parts[i] = quoteKey(p)
	}
	return strings.Join(parts, ".")
}

func (k Key) child(name string) Key {
	return append(k[:len(k):len(k)], name)
}

// DecodeError is a valid document whose value at Key does not fit the Go
// value it is decoded into: the wrong type, a number out of range, or an
// error from an UnmarshalText method (which errors.Unwrap returns).
type DecodeError struct {
	Key  Key
	Line int
	Msg  string
	err  error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("toml: line %d: key %s: %s", e.Line, e.Key, e.Msg)
}

func (e *DecodeError) Unwrap() error { return e.err }

// UnknownKeysError lists every key, in document order, that the Go value
// has no place for. It is returned only when nothing else went wrong, and
// only unless AllowUnknownKeys was given.
type UnknownKeysError struct {
	Keys []Key
}

func (e *UnknownKeysError) Error() string {
	const shown = 10
	names := make([]string, 0, shown)
	for i, k := range e.Keys {
		if i == shown {
			break
		}
		names = append(names, clip(k.String()))
	}
	s := "toml: unknown keys: " + strings.Join(names, ", ")
	if len(e.Keys) > shown {
		s += fmt.Sprintf(" and %d more", len(e.Keys)-shown)
	}
	if len(e.Keys) == 1 {
		s = "toml: unknown key: " + names[0]
	}
	return s
}

// position turns a byte offset into a line and a character column. It runs
// only when an error is reported, so the parser itself tracks offsets alone.
// A leading byte order mark is not a character of the first line.
func position(src string, off int) (line, col int) {
	if off > len(src) {
		off = len(src)
	}
	line, start := 1, 0
	if strings.HasPrefix(src, bom) && off >= len(bom) {
		start = len(bom)
	}
	for i := start; i < off; i++ {
		if src[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return line, utf8.RuneCountInString(src[start:off]) + 1
}

const bom = "\xEF\xBB\xBF"

// clip shortens text quoted from the document so that an error message stays
// readable and small whatever the document holds.
func clip(s string) string {
	const max = 40
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	i, n := 0, 0
	for n < max {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return s[:i] + "…"
}

func newParseError(src string, off int, format string, args ...any) *ParseError {
	line, col := position(src, off)
	return &ParseError{Line: line, Column: col, Msg: fmt.Sprintf(format, args...)}
}

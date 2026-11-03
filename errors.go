package toml

import (
	"fmt"
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

// position turns a byte offset into a line and a character column. It runs
// only when an error is reported, so the parser itself tracks offsets alone.
func position(src string, off int) (line, col int) {
	if off > len(src) {
		off = len(src)
	}
	line, start := 1, 0
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return line, utf8.RuneCountInString(src[start:off]) + 1
}

func newParseError(src string, off int, format string, args ...any) *ParseError {
	line, col := position(src, off)
	return &ParseError{Line: line, Column: col, Msg: fmt.Sprintf(format, args...)}
}

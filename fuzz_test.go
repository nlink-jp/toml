package toml

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParse holds the parser to two properties on any input: it never
// panics, and every failure is a *ParseError with a real position. The seeds
// are every document in the conformance suite. Inputs the fuzzer finds land
// in testdata/fuzz/FuzzParse and are committed as regression cases.
//
//	go test -run '^$' -fuzz FuzzParse -fuzztime 60s
func FuzzParse(f *testing.F) {
	list, err := os.ReadFile(filepath.Join(suiteDir, "tests", "files-toml-1.1.0"))
	if err != nil {
		f.Fatal(err)
	}
	for _, name := range strings.Split(string(list), "\n") {
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(suiteDir, "tests", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(b))
	}
	f.Fuzz(func(t *testing.T, src string) {
		root, err := parse(src)
		if err == nil {
			if root == nil {
				t.Fatal("no error and no tree")
			}
			return
		}
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("error is %T, want *ParseError: %v", err, err)
		}
		if pe.Line < 1 || pe.Column < 1 || pe.Msg == "" {
			t.Fatalf("error without a position or message: %+v", pe)
		}
		// The position is inside the document, or just past its end.
		lines := strings.Split(src, "\n")
		if pe.Line > len(lines) {
			t.Fatalf("line %d of a %d-line document", pe.Line, len(lines))
		}
		if max := utf8.RuneCountInString(lines[pe.Line-1]) + 1; pe.Column > max {
			t.Fatalf("column %d of a %d-character line %d", pe.Column, max-1, pe.Line)
		}
	})
}

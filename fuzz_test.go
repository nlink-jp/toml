package toml

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// addSuiteSeeds seeds a fuzz target with every document of the conformance
// suite for TOML 1.1.0.
func addSuiteSeeds(f *testing.F) {
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
}

// FuzzDecode decodes any input into an `any` and into a struct of every kind
// of field. It holds the decoder to never panicking, and to failing only with
// the three error types it documents.
//
//	go test -run '^$' -fuzz FuzzDecode -fuzztime 60s
func FuzzDecode(f *testing.F) {
	addSuiteSeeds(f)
	f.Add("a = 1\n[b]\nc = 'x'\n[[d]]\ne = 1979-05-27T07:32:00Z\n")
	f.Add("S = 's'\nI = -1\nU = 1\nF = 1.5\nB = true\nT = 07:32\nL = [1, 2]\nM = {k = 'v'}\nP = 1\n")
	type fuzzTarget struct {
		S  string
		I  int8
		U  uint16
		F  float32
		B  bool
		T  time.Time
		LT LocalTime
		L  []int
		A  [2]string
		M  map[string]any
		P  *int
		N  struct{ X, Y int }
		Q  []struct{ Z string }
	}
	f.Fuzz(func(t *testing.T, src string) {
		var anyV any
		var st fuzzTarget
		for _, v := range []any{&anyV, &st} {
			_, err := Decode(src, v)
			if err == nil {
				continue
			}
			var pe *ParseError
			var de *DecodeError
			var uk *UnknownKeysError
			if !errors.As(err, &pe) && !errors.As(err, &de) && !errors.As(err, &uk) {
				t.Fatalf("error is %T: %v", err, err)
			}
		}
	})
}

// FuzzParse holds the parser to two properties on any input: it never
// panics, and every failure is a *ParseError with a real position. The seeds
// are every document in the conformance suite. Inputs the fuzzer finds land
// in testdata/fuzz/FuzzParse and are committed as regression cases.
//
//	go test -run '^$' -fuzz FuzzParse -fuzztime 60s
func FuzzParse(f *testing.F) {
	addSuiteSeeds(f)
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

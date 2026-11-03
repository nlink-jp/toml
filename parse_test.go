package toml

import (
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *table {
	t.Helper()
	root, err := parse(src)
	if err != nil {
		t.Fatalf("parse(%q): %v", src, err)
	}
	return root
}

func parseErr(t *testing.T, src string) *ParseError {
	t.Helper()
	_, err := parse(src)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("parse(%q) = %v, want a *ParseError", src, err)
	}
	return pe
}

func lookup(t *testing.T, root *table, path ...string) *node {
	t.Helper()
	n := &node{kind: kTable, tbl: root}
	for _, k := range path {
		if n.kind != kTable {
			t.Fatalf("%v: %s is a %s", path, k, n.kind)
		}
		next, ok := n.tbl.entries[k]
		if !ok {
			t.Fatalf("%v: no key %s", path, k)
		}
		n = next
	}
	return n
}

// toml-test has no document on this, so the decision is pinned here. A table
// a header created only as a parent ([a.b.c] creates a.b) has not been
// defined, and the spec lets one write to a key "as long as [it] hasn't been
// directly defined". Writing into it with dotted keys defines it; after that
// no header may define it again.
func TestDottedKeysMayDefineATableAHeaderOnlyImplied(t *testing.T) {
	root := mustParse(t, "[a.b.c]\nz = 1\n[a]\nb.d = 2\n")
	if got := lookup(t, root, "a", "b", "d").i; got != 2 {
		t.Errorf("a.b.d = %d, want 2", got)
	}
	if got := lookup(t, root, "a", "b", "c", "z").i; got != 1 {
		t.Errorf("a.b.c.z = %d, want 1", got)
	}
	pe := parseErr(t, "[a.b.c]\n[a]\nb.d = 2\n[a.b]\n")
	if !strings.Contains(pe.Msg, "defined twice") {
		t.Errorf("redefining a.b by header: %v", pe)
	}
}

// What the suite does cover, restated so that a change to the rule that
// allows the case above cannot quietly allow these too.
func TestTablesDefinedOnceStayDefined(t *testing.T) {
	for _, src := range []string{
		"[a.b.c]\nz = 9\n[a]\nb.c.t = 1\n", // dotted keys into a header's table
		"[a]\nb.c = 1\n[a.b]\n",            // a header over a dotted table
		"[a]\nb.c = 1\n[x]\n[a]\n",         // a header twice
		"a = {b = 1}\na.c = 2\n",           // into an inline table
		"a = {b = 1}\n[a.c]\n",             // a header through an inline table
		"[a]\nb.c = 1\n[x]\n[a.b]\n",       // a dotted table from another section
		"[[a]]\n[a]\n",                     // a table over an array of tables
		"a = []\n[[a]]\n",                  // extending a static array
	} {
		if _, err := parse(src); err == nil {
			t.Errorf("parse(%q) accepted it", src)
		}
	}
}

func TestHeadersMayDefineSubTablesOfDottedTables(t *testing.T) {
	root := mustParse(t, "[fruit]\napple.color = 'red'\n[fruit.apple.texture]\nsmooth = true\n")
	if !lookup(t, root, "fruit", "apple", "texture", "smooth").b {
		t.Error("fruit.apple.texture.smooth is not true")
	}
}

func TestErrorsNameTheLineAndTheCharacterColumn(t *testing.T) {
	pe := parseErr(t, "a = 1\n\"ключ\" = 2\n\"ключ\" = 3\n")
	if pe.Line != 3 || pe.Column != 1 {
		t.Errorf("duplicate key at %d:%d, want 3:1 (%v)", pe.Line, pe.Column, pe)
	}
	// Columns count characters: after four Cyrillic letters (two bytes each)
	// the stray x is at column 12; counting bytes would say 16.
	pe = parseErr(t, "\"ключ\" = 2 x\n")
	if pe.Line != 1 || pe.Column != 12 {
		t.Errorf("trailing text at %d:%d, want 1:12 (%v)", pe.Line, pe.Column, pe)
	}
	if !strings.HasPrefix(pe.Error(), "toml: line 1, column 12: ") {
		t.Errorf("Error() = %q", pe.Error())
	}
}

func TestInvalidUTF8IsReportedWhereItIs(t *testing.T) {
	pe := parseErr(t, "a = 1\nb = \"\xff\"\n")
	if pe.Line != 2 || pe.Column != 6 {
		t.Errorf("invalid UTF-8 at %d:%d, want 2:6", pe.Line, pe.Column)
	}
}

func TestALeadingByteOrderMarkIsSkipped(t *testing.T) {
	root := mustParse(t, "\xEF\xBB\xBFa = 1\n")
	if lookup(t, root, "a").i != 1 {
		t.Error("a != 1")
	}
	// Nor is it a character of the first line when an error is placed.
	with, without := parseErr(t, "\xEF\xBB\xBFa = 1 x\n"), parseErr(t, "a = 1 x\n")
	if with.Column != without.Column {
		t.Errorf("column with a BOM %d, without %d", with.Column, without.Column)
	}
}

func TestMultilineStringsTurnCRLFIntoLF(t *testing.T) {
	root := mustParse(t, "a = \"\"\"\r\none\r\ntwo\"\"\"\r\nb = '''\r\nthree\r\nfour'''\r\n")
	if got := lookup(t, root, "a").str; got != "one\ntwo" {
		t.Errorf("a = %q", got)
	}
	if got := lookup(t, root, "b").str; got != "three\nfour" {
		t.Errorf("b = %q", got)
	}
	parseErr(t, "a = \"\"\"one\rtwo\"\"\"\n") // a lone CR is not a newline
}

func TestFractionsBeyondNanosecondsAreTruncated(t *testing.T) {
	root := mustParse(t, "t = 07:32:00.9999999999\n")
	if got := lookup(t, root, "t").dt.nanos; got != 999999999 {
		t.Errorf("nanos = %d, want 999999999 (truncated, not rounded)", got)
	}
}

func TestOmittedSecondsAreZero(t *testing.T) {
	root := mustParse(t, "a = 1979-05-27 07:32-07:00\nb = 07:32\n")
	a := lookup(t, root, "a")
	if a.kind != kDatetime || a.dt.minute != 32 || a.dt.second != 0 || a.dt.offset != -7*3600 {
		t.Errorf("a = %+v", a)
	}
	if b := lookup(t, root, "b"); b.kind != kTimeLocal || b.dt.second != 0 {
		t.Errorf("b = %+v", b)
	}
}

func TestValuesOutOfRangeAreErrors(t *testing.T) {
	for _, src := range []string{
		"a = 9223372036854775808\n",
		"a = -9223372036854775809\n",
		"a = 0x8000000000000000\n",
		"a = 1e400\n",
		"a = 1979-02-29\n",          // not a leap year
		"a = 1979-05-27T07:32:60\n", // leap seconds are not read
		"a = \"\\uD800\"\n",         // a surrogate is not a scalar value
	} {
		parseErr(t, src)
	}
	root := mustParse(t, "a = 9223372036854775807\nb = -9223372036854775808\nc = 2000-02-29\n")
	if lookup(t, root, "b").i != -9223372036854775808 {
		t.Error("b is not the smallest int64")
	}
}

func TestDepthIsBounded(t *testing.T) {
	ok := strings.Repeat("[", maxDepth) + strings.Repeat("]", maxDepth)
	mustParse(t, "a = "+ok+"\n")
	parseErr(t, "a = "+strings.Repeat("[", maxDepth+1)+strings.Repeat("]", maxDepth+1)+"\n")
	parseErr(t, "a = "+strings.Repeat("{b = ", maxDepth+1)+"1"+strings.Repeat("}", maxDepth+1)+"\n")

	parts := strings.Repeat("a.", maxDepth-1) + "a"
	mustParse(t, parts+" = 1\n")
	parseErr(t, "a."+parts+" = 1\n")
	parseErr(t, "["+parts+".a]\n")

	// Each header stays within the limit; together they nest too deep.
	pe := parseErr(t, "["+parts+"]\n"+parts+" = 1\n")
	if !strings.Contains(pe.Msg, "nest deeper") {
		t.Errorf("deep tree: %v", pe)
	}
}

// The tree is walked through maps; the reported place must not depend on
// the order a map happens to yield. Three lines nest too deep, and the
// first of them is the one named, every time.
func TestTheDepthErrorNamesTheFirstPlaceEveryTime(t *testing.T) {
	half := strings.Repeat("a.", 69) + "a"
	src := "[" + strings.Repeat("h.", 69) + "h]\nx." + half + " = 1\ny." + half + " = 1\nz." + half + " = 1\n"
	for i := 0; i < 50; i++ {
		if pe := parseErr(t, src); pe.Line != 2 {
			t.Fatalf("run %d: line %d, want 2 (%v)", i, pe.Line, pe)
		}
	}
}

func TestTheDocumentSizeIsBounded(t *testing.T) {
	mustParse(t, strings.Repeat("#", maxInputBytes))
	pe := parseErr(t, strings.Repeat("#", maxInputBytes+1))
	if !strings.Contains(pe.Msg, "1 MiB") {
		t.Errorf("%v", pe)
	}
}

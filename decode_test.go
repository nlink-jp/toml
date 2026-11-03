package toml

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func decodeErr[T any](t *testing.T, src string, v *T, opts ...Option) error {
	t.Helper()
	_, err := Decode(src, v, opts...)
	if err == nil {
		t.Fatalf("Decode(%q) succeeded, want an error", src)
	}
	return err
}

func mustDecode[T any](t *testing.T, src string, v *T, opts ...Option) MetaData {
	t.Helper()
	md, err := Decode(src, v, opts...)
	if err != nil {
		t.Fatalf("Decode(%q): %v", src, err)
	}
	return md
}

func TestDecodeScalarsAndNesting(t *testing.T) {
	type model struct {
		Name        string  `toml:"name"`
		Temperature float64 `toml:"temperature"`
		MaxTokens   int     `toml:"max_tokens"`
		Stream      bool    `toml:"stream"`
	}
	var cfg struct {
		Model model             `toml:"model"`
		Tags  []string          `toml:"tags"`
		Env   map[string]string `toml:"env"`
		Port  uint16            `toml:"port"`
	}
	mustDecode(t, `
tags = ["a", "b"]
port = 8080
[model]
name = "m"
temperature = 1   # an integer may fill a float
max_tokens = 4096
stream = true
[env]
HOME = "/h"
`, &cfg)
	if cfg.Model != (model{"m", 1, 4096, true}) || cfg.Port != 8080 ||
		!reflect.DeepEqual(cfg.Tags, []string{"a", "b"}) || cfg.Env["HOME"] != "/h" {
		t.Errorf("decoded %+v", cfg)
	}
}

// The organization's tools fill a config with defaults and decode the file
// on top: what the file does not say must keep its default.
func TestDecodeKeepsWhatTheDocumentDoesNotSet(t *testing.T) {
	type section struct {
		A, B string
		List []int
	}
	cfg := struct {
		S   section
		M   map[string]int
		Ptr *section
	}{
		S:   section{A: "default-a", B: "default-b", List: []int{1, 2, 3}},
		M:   map[string]int{"kept": 1, "over": 1},
		Ptr: &section{A: "ptr-default"},
	}
	mustDecode(t, "[S]\nB = 'file-b'\nList = [9]\n[M]\nover = 2\nnew = 3\n[Ptr]\nB = 'ptr-file'\n", &cfg)
	if cfg.S.A != "default-a" || cfg.S.B != "file-b" {
		t.Errorf("S = %+v", cfg.S)
	}
	if !reflect.DeepEqual(cfg.S.List, []int{9}) {
		t.Errorf("a slice is replaced whole: %v", cfg.S.List)
	}
	if !reflect.DeepEqual(cfg.M, map[string]int{"kept": 1, "over": 2, "new": 3}) {
		t.Errorf("a map keeps entries the document does not write: %v", cfg.M)
	}
	if cfg.Ptr.A != "ptr-default" || cfg.Ptr.B != "ptr-file" {
		t.Errorf("a set pointer is decoded through: %+v", cfg.Ptr)
	}
}

// A map entry the document writes is decoded on top of the entry already
// there, as a struct is: its fields the document does not set keep their
// defaults.
func TestMapEntriesAreDecodedOnTopOfTheirDefaults(t *testing.T) {
	type server struct{ Host, Port string }
	cfg := map[string]server{"main": {Host: "localhost", Port: "80"}}
	mustDecode(t, "[main]\nPort = '8080'\n", &cfg)
	if cfg["main"] != (server{"localhost", "8080"}) {
		t.Errorf("main = %+v", cfg["main"])
	}
}

func TestPointersTellSetFromAbsent(t *testing.T) {
	var cfg struct {
		MaxTokens *int  `toml:"max_tokens"`
		Enabled   *bool `toml:"enabled"`
	}
	mustDecode(t, "enabled = false\n", &cfg)
	if cfg.MaxTokens != nil {
		t.Error("max_tokens was not written but is set")
	}
	if cfg.Enabled == nil || *cfg.Enabled {
		t.Error("enabled = false was not decoded into a set *bool")
	}
}

func TestFieldsAreMatchedLikeBurntSushi(t *testing.T) {
	type Inner struct {
		Shared string
		Deep   string `toml:"deep"`
	}
	var cfg struct {
		Inner                       // untagged and embedded: its fields are promoted
		Shared      string          // shallower than Inner.Shared, so it wins
		MixedCase   string          // matched case-insensitively
		Exact       string          `toml:"exact"`
		Other       string          `toml:"EXACT"`
		Skipped     string          `toml:"-"`
		WithOptions string          `toml:"with_options,omitempty"`
		unexported  string          //nolint:unused
		Tagged      struct{ X int } `toml:"tagged"`
	}
	mustDecode(t, `
Shared = "outer"
deep = "promoted"
mixedcase = "folded"
exact = "lower"
EXACT = "upper"
with_options = "opts"
tagged.X = 1
`, &cfg)
	if cfg.Shared != "outer" || cfg.Inner.Shared != "" {
		t.Errorf("Shared: outer %q, inner %q", cfg.Shared, cfg.Inner.Shared)
	}
	if cfg.Deep != "promoted" || cfg.MixedCase != "folded" || cfg.WithOptions != "opts" || cfg.Tagged.X != 1 {
		t.Errorf("decoded %+v", cfg)
	}
	if cfg.Exact != "lower" || cfg.Other != "upper" {
		t.Errorf("an exact match must win over a case-insensitive one: %q %q", cfg.Exact, cfg.Other)
	}

	var skip struct {
		Skipped string `toml:"-"`
	}
	err := decodeErr(t, "Skipped = 'x'\n", &skip)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) {
		t.Errorf("a field tagged \"-\" takes no key: %v", err)
	}
}

// `name` and `NAME` are different TOML keys that the case-insensitive match
// leads to one field. Neither may silently win.
func TestTwoKeysForOneFieldAreAnError(t *testing.T) {
	var cfg struct {
		Name string `toml:"name"`
	}
	for _, src := range []string{"name = 'exact'\nNAME = 'folded'\n", "NAME = 'folded'\nname = 'exact'\n", "Name = 'a'\nnAmE = 'b'\n"} {
		err := decodeErr(t, src, &cfg)
		var de *DecodeError
		if !errors.As(err, &de) || de.Line != 2 || !strings.Contains(de.Msg, "both set the field Name") {
			t.Errorf("Decode(%q): %v", src, err)
		}
	}
	// The same key in two tables of an array is two values, not a clash.
	var list struct{ Items []struct{ Name string } }
	mustDecode(t, "[[Items]]\nname = 'a'\n[[Items]]\nNAME = 'b'\n", &list)
}

func TestEmbeddedPointersAreCreatedWhenWritten(t *testing.T) {
	type Base struct{ Level string }
	var cfg struct {
		*Base
		Name string
	}
	mustDecode(t, "Level = 'debug'\nName = 'n'\n", &cfg)
	if cfg.Base == nil || cfg.Level != "debug" {
		t.Errorf("decoded %+v", cfg)
	}
}

func TestAmbiguousPromotedFieldsTakeNoKey(t *testing.T) {
	type A struct{ Name string }
	type B struct{ Name string }
	var cfg struct {
		A
		B
	}
	err := decodeErr(t, "Name = 'x'\n", &cfg)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) {
		t.Errorf("two promoted fields of one name at one depth: %v", err)
	}
}

func TestUnknownKeysAreListedInDocumentOrder(t *testing.T) {
	type entry struct {
		Name string `toml:"name"`
	}
	var cfg struct {
		Model struct {
			Name string `toml:"name"`
		} `toml:"model"`
		Items []entry `toml:"items"`
	}
	src := `
typo = 1
[model]
name = "kept"
nmae = "typo"
[[items]]
name = "a"
color = "red"
[[items]]
name = "b"
color = "blue"
["odd key"]
`
	_, err := Decode(src, &cfg)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) {
		t.Fatalf("err = %v", err)
	}
	var got []string
	for _, k := range uk.Keys {
		got = append(got, k.String())
	}
	want := []string{"typo", "model.nmae", "items.color", `"odd key"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknown keys %q, want %q", got, want)
	}
	if cfg.Model.Name != "kept" || len(cfg.Items) != 2 || cfg.Items[1].Name != "b" {
		t.Errorf("known keys must still be decoded: %+v", cfg)
	}
	if !strings.Contains(err.Error(), "typo, model.nmae, items.color") {
		t.Errorf("Error() = %q", err.Error())
	}

	cfg2 := cfg
	if _, err := Decode(src, &cfg2, AllowUnknownKeys()); err != nil {
		t.Errorf("AllowUnknownKeys: %v", err)
	}
}

func TestUnknownKeysErrorShowsTenAndCountsTheRest(t *testing.T) {
	var cfg struct{}
	var b strings.Builder
	for i := 0; i < 13; i++ {
		b.WriteString("k" + string(rune('a'+i)) + " = 1\n")
	}
	err := decodeErr(t, b.String(), &cfg)
	if !strings.HasSuffix(err.Error(), "kj and 3 more") {
		t.Errorf("Error() = %q", err.Error())
	}
	err = decodeErr(t, "only = 1\n", &cfg)
	if err.Error() != "toml: unknown key: only" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestIsDefined(t *testing.T) {
	var cfg map[string]any
	md := mustDecode(t, "[a.b.c]\nx = 1\n[[list]]\ny = 2\n", &cfg)
	for _, k := range [][]string{{"a"}, {"a", "b"}, {"a", "b", "c", "x"}, {"list"}} {
		if !md.IsDefined(k...) {
			t.Errorf("IsDefined(%q) = false", k)
		}
	}
	for _, k := range [][]string{{}, {"z"}, {"a", "z"}, {"a", "b", "c", "x", "deeper"}, {"list", "y"}} {
		if md.IsDefined(k...) {
			t.Errorf("IsDefined(%q) = true", k)
		}
	}
	if (MetaData{}).IsDefined("a") {
		t.Error("the zero MetaData defines nothing")
	}
}

func TestNumbersMustFit(t *testing.T) {
	var i8 struct{ V int8 }
	var u struct{ V uint }
	var f32 struct{ V float32 }
	var f64 struct{ V float64 }
	var i struct{ V int }
	for _, c := range []struct {
		src string
		v   any
	}{
		{"V = 128", &i8},
		{"V = -1", &u},
		{"V = 1e300", &f32},
		{"V = 9007199254740993", &f64}, // 2^53 + 1 has no float64
		{"V = 16777217", &f32},         // 2^24 + 1 has no float32
		{"V = 1.0", &i},                // a float is never an integer
		{"V = 'x'", &i},
	} {
		_, err := Decode(c.src, c.v)
		var de *DecodeError
		if !errors.As(err, &de) {
			t.Errorf("Decode(%q) into %T: %v, want a *DecodeError", c.src, c.v, err)
		}
	}
	mustDecode(t, "V = 9007199254740992\n", &f64)
	mustDecode(t, "V = inf\n", &f32)
	if !math.IsInf(float64(f32.V), 1) {
		t.Errorf("inf into float32 = %v", f32.V)
	}
}

func TestDecodeErrorsNameTheKeyAndLine(t *testing.T) {
	var cfg struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
	}
	err := decodeErr(t, "\n[server]\nport = 'eighty'\n", &cfg)
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v", err)
	}
	if de.Line != 3 || de.Key.String() != "server.port" {
		t.Errorf("DecodeError at line %d key %s", de.Line, de.Key)
	}
	if err.Error() != "toml: line 3: key server.port: cannot decode a string into int" {
		t.Errorf("Error() = %q", err.Error())
	}
}

type level string

func (l *level) UnmarshalText(b []byte) error {
	switch s := string(b); s {
	case "debug", "info":
		*l = level(s)
		return nil
	}
	return errBadLevel
}

var errBadLevel = errors.New("not a level")

type stamp struct{ text string }

func (s *stamp) UnmarshalText(b []byte) error { s.text = string(b); return nil }

func TestTextUnmarshalers(t *testing.T) {
	var cfg struct {
		Level level
		Stamp stamp
	}
	mustDecode(t, "Level = 'debug'\nStamp = 1979-05-27T07:32:00-07:00\n", &cfg)
	if cfg.Level != "debug" || cfg.Stamp.text != "1979-05-27T07:32:00-07:00" {
		t.Errorf("decoded %+v", cfg)
	}
	err := decodeErr(t, "Level = 'loud'\n", &cfg)
	if !errors.Is(err, errBadLevel) {
		t.Errorf("the method's error must be reachable: %v", err)
	}
	err = decodeErr(t, "Level = 1\n", &cfg)
	if !strings.Contains(err.Error(), "cannot decode an integer into toml.level") {
		t.Errorf("err = %v", err)
	}
}

func TestDatetimes(t *testing.T) {
	var cfg struct {
		Offset   time.Time
		UTC      time.Time
		FromText time.Time
		DT       LocalDateTime
		D        LocalDate
		T        LocalTime
		Leap     LocalTime
	}
	mustDecode(t, `
Offset = 1979-05-27T00:32:00.5-07:00
UTC = 1979-05-27 07:32Z
FromText = "1979-05-27T07:32:00+09:00"
DT = 1979-05-27T07:32:00.999999999
D = 1979-05-27
T = 07:32
Leap = 23:59:60
`, &cfg)
	if _, off := cfg.Offset.Zone(); off != -7*3600 || cfg.Offset.Nanosecond() != 5e8 {
		t.Errorf("Offset = %v: the offset as written must be kept", cfg.Offset)
	}
	// 00:32:00.5-07:00 is the instant 07:32:00.5Z, half a second after UTC.
	if cfg.UTC.Location() != time.UTC || !cfg.UTC.Equal(cfg.Offset.Add(-500*time.Millisecond)) {
		t.Errorf("UTC = %v", cfg.UTC)
	}
	if _, off := cfg.FromText.Zone(); off != 9*3600 {
		t.Errorf("FromText = %v", cfg.FromText)
	}
	want := LocalDateTime{LocalDate{1979, time.May, 27}, LocalTime{7, 32, 0, 999999999}}
	if cfg.DT != want || cfg.D != want.Date || cfg.T != (LocalTime{7, 32, 0, 0}) || cfg.Leap.Second != 60 {
		t.Errorf("decoded %+v", cfg)
	}
	if cfg.DT.String() != "1979-05-27T07:32:00.999999999" || cfg.T.String() != "07:32:00" {
		t.Errorf("String: %s %s", cfg.DT, cfg.T)
	}
	if got := want.In(time.UTC); !got.Equal(time.Date(1979, 5, 27, 7, 32, 0, 999999999, time.UTC)) {
		t.Errorf("In = %v", got)
	}

	// A local value has no offset; it does not quietly become an instant.
	var inst struct{ V time.Time }
	err := decodeErr(t, "V = 1979-05-27T07:32:00\n", &inst)
	if !strings.Contains(err.Error(), "decode it into toml.LocalDateTime") {
		t.Errorf("err = %v", err)
	}
	var d struct{ V LocalDate }
	decodeErr(t, "V = 07:32:00\n", &d)
}

func TestDurationsReadStrings(t *testing.T) {
	var cfg struct {
		Timeout time.Duration
		Nanos   time.Duration
		Ptr     *time.Duration
	}
	mustDecode(t, "Timeout = '1m30s'\nNanos = 1500\nPtr = '2h'\n", &cfg)
	if cfg.Timeout != 90*time.Second || cfg.Nanos != 1500 || cfg.Ptr == nil || *cfg.Ptr != 2*time.Hour {
		t.Errorf("decoded %+v", cfg)
	}
	err := decodeErr(t, "Timeout = 'soon'\n", &cfg)
	var de *DecodeError
	if !errors.As(err, &de) || de.Key.String() != "Timeout" || errors.Unwrap(err) == nil {
		t.Errorf("err = %v", err)
	}
	decodeErr(t, "Timeout = 1.5\n", &cfg)
}

func TestAnyReceivesPlainValues(t *testing.T) {
	var v any
	mustDecode(t, "s = 'x'\ni = 1\nf = 1.5\nb = true\nt = 1979-05-27T07:32:00Z\nd = 1979-05-27\na = [1, 'two']\n[tbl]\nk = 'v'\n", &v)
	m := v.(map[string]any)
	if m["s"] != "x" || m["i"] != int64(1) || m["f"] != 1.5 || m["b"] != true {
		t.Errorf("scalars: %#v", m)
	}
	if _, ok := m["t"].(time.Time); !ok {
		t.Errorf("t is %T", m["t"])
	}
	if m["d"] != (LocalDate{1979, time.May, 27}) {
		t.Errorf("d = %#v", m["d"])
	}
	if !reflect.DeepEqual(m["a"], []any{int64(1), "two"}) || !reflect.DeepEqual(m["tbl"], map[string]any{"k": "v"}) {
		t.Errorf("a = %#v, tbl = %#v", m["a"], m["tbl"])
	}
}

func TestTargetsThatCannotHoldATable(t *testing.T) {
	var s fmtStringer
	err := decodeErr(t, "a = 1\n", &s)
	if !strings.Contains(err.Error(), "interface") {
		t.Errorf("err = %v", err)
	}
	var m map[int]string
	decodeErr(t, "a = 1\n", &m)
	var n int
	decodeErr(t, "a = 1\n", &n)
	if _, err := Decode("a = 1", nil); err == nil {
		t.Error("nil target accepted")
	}
	var notPtr struct{}
	if _, err := Decode("a = 1", notPtr); err == nil {
		t.Error("non-pointer target accepted")
	}
}

type fmtStringer interface{ String() string }

func TestGoArrays(t *testing.T) {
	cfg := struct{ A [3]int }{A: [3]int{7, 7, 7}}
	mustDecode(t, "A = [1, 2]\n", &cfg)
	if cfg.A != [3]int{1, 2, 0} {
		t.Errorf("A = %v", cfg.A)
	}
	decodeErr(t, "A = [1, 2, 3, 4]\n", &cfg)
}

// Messages quote the document and the errors of a target's own methods, and
// stay small whatever those hold.
func TestDecodeMessagesStaySmall(t *testing.T) {
	long := strings.Repeat("k", 100000)
	var m map[string]int
	var tt struct{ T time.Time }
	var lv struct{ Level level }
	for _, c := range []struct {
		src string
		v   any
	}{
		{`"` + long + `" = 'x'`, &m},
		{`T = "` + long + `"`, &tt},
		{`Level = "` + long + `"`, &lv},
	} {
		_, err := Decode(c.src, c.v)
		if err == nil || len(err.Error()) > 300 {
			t.Errorf("%T: %d-byte message", c.v, len(fmt.Sprint(err)))
		}
	}
	var s struct{}
	_, err := Decode(`"`+long+`" = 1`, &s)
	if err == nil || len(err.Error()) > 100 {
		t.Errorf("unknown key: %d-byte message", len(fmt.Sprint(err)))
	}
}

// A string decoded into time.Time keeps the offset as written, unnamed, as a
// TOML datetime does — not the name of the machine's zone.
func TestTimeFromAStringKeepsAnUnnamedOffset(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("JST", 9*3600)
	defer func() { time.Local = saved }()
	var cfg struct{ A, B time.Time }
	mustDecode(t, "A = '1979-05-27T07:32:00+09:00'\nB = 1979-05-27T07:32:00+09:00\n", &cfg)
	an, ao := cfg.A.Zone()
	bn, bo := cfg.B.Zone()
	if an != bn || ao != bo || an != "" {
		t.Errorf("from a string %q %d, from a datetime %q %d", an, ao, bn, bo)
	}
}

type selfPointer *selfPointer

func TestSelfReferentialPointersEnd(t *testing.T) {
	var cfg struct{ P selfPointer }
	done := make(chan error, 1)
	go func() { _, err := Decode("P = 1\n", &cfg); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "pointers do not end") {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decoding into type P *P did not end")
	}
}

func TestTheRootIsNamedWithoutAKey(t *testing.T) {
	var n int
	err := decodeErr(t, "a = 1\n", &n)
	if err.Error() != "toml: line 1: cannot decode a table into int" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestDecodeFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(good, []byte("name = 'x'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Name string }
	if _, err := DecodeFile(good, &cfg); err != nil || cfg.Name != "x" {
		t.Errorf("DecodeFile: %v, %+v", err, cfg)
	}
	if _, err := DecodeFile(filepath.Join(dir, "missing.toml"), &cfg); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
	big := filepath.Join(dir, "big.toml")
	if err := os.WriteFile(big, []byte(strings.Repeat("#", maxInputBytes+10)), 0o600); err != nil {
		t.Fatal(err)
	}
	var pe *ParseError
	if _, err := DecodeFile(big, &cfg); !errors.As(err, &pe) {
		t.Errorf("oversized file: %v", err)
	}
}

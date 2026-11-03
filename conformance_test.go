package toml

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const suiteDir = "testdata/toml-test"

// The conformance data is vendored, so a test run is the place to notice that
// it changed: every file must be listed in SHA256SUMS with its hash, and
// nothing may be there that is not listed.
func TestTheConformanceDataIsUnchanged(t *testing.T) {
	f, err := os.Open(filepath.Join(suiteDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	listed := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			t.Fatalf("malformed SHA256SUMS line %q", sc.Text())
		}
		listed[name] = sum
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	seen := 0
	err = filepath.WalkDir(suiteDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name, _ := filepath.Rel(suiteDir, path)
		name = filepath.ToSlash(name)
		if name == "SHA256SUMS" || name == "SOURCE.md" {
			return nil
		}
		want, ok := listed[name]
		if !ok {
			t.Errorf("%s is not in SHA256SUMS", name)
			return nil
		}
		seen++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		if got := hex.EncodeToString(h[:]); got != want {
			t.Errorf("%s: sha256 %s, SHA256SUMS says %s", name, got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(listed) {
		t.Errorf("SHA256SUMS lists %d files, %d are present", len(listed), seen)
	}
}

// suiteFiles is the list of tests that apply to TOML 1.1.0.
func suiteFiles(t *testing.T) []string {
	b, err := os.ReadFile(filepath.Join(suiteDir, "tests", "files-toml-1.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasSuffix(line, ".toml") {
			files = append(files, line)
		}
	}
	// toml-test v2.2.0 lists 214 valid and 467 invalid documents for 1.1.0.
	// A different count means the list is not the one vendored, and a
	// silently shorter list would pass more easily.
	if len(files) != 681 {
		t.Fatalf("%d tests listed for TOML 1.1.0, want 681", len(files))
	}
	return files
}

// parseNoPanic turns a panic into a test failure that names the document.
func parseNoPanic(src string) (root *table, err error, panicked any) {
	defer func() { panicked = recover() }()
	root, err = parse(src)
	return root, err, nil
}

func TestConformanceValid(t *testing.T) {
	for _, name := range suiteFiles(t) {
		if !strings.HasPrefix(name, "valid/") {
			continue
		}
		t.Run(strings.TrimSuffix(name, ".toml"), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(suiteDir, "tests", name))
			if err != nil {
				t.Fatal(err)
			}
			root, err, panicked := parseNoPanic(string(src))
			if panicked != nil {
				t.Fatalf("panic: %v", panicked)
			}
			if err != nil {
				t.Fatalf("rejected a valid document: %v", err)
			}
			wantJSON, err := os.ReadFile(filepath.Join(suiteDir, "tests", strings.TrimSuffix(name, ".toml")+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(wantJSON, &want); err != nil {
				t.Fatalf("expected value: %v", err)
			}
			if msg := sameTagged(want, tagged(&node{kind: kTable, tbl: root}), ""); msg != "" {
				t.Error(msg)
			}
		})
	}
}

func TestConformanceInvalid(t *testing.T) {
	for _, name := range suiteFiles(t) {
		if !strings.HasPrefix(name, "invalid/") {
			continue
		}
		t.Run(strings.TrimSuffix(name, ".toml"), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(suiteDir, "tests", name))
			if err != nil {
				t.Fatal(err)
			}
			_, err, panicked := parseNoPanic(string(src))
			if panicked != nil {
				t.Fatalf("panic: %v", panicked)
			}
			if err == nil {
				t.Fatalf("accepted an invalid document:\n%s", src)
			}
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("error is %T, want *ParseError: %v", err, err)
			}
		})
	}
}

// tagged writes a tree in toml-test's JSON shape: tables are objects, arrays
// are arrays, and every other value is {"type": …, "value": "…"}.
func tagged(n *node) any {
	switch n.kind {
	case kTable:
		m := map[string]any{}
		for k, v := range n.tbl.entries {
			m[k] = tagged(v)
		}
		return m
	case kArray:
		a := make([]any, len(n.arr))
		for i, v := range n.arr {
			a[i] = tagged(v)
		}
		return a
	}
	var v string
	switch n.kind {
	case kString:
		v = n.str
	case kInteger:
		v = strconv.FormatInt(n.i, 10)
	case kFloat:
		switch {
		case math.IsNaN(n.f):
			v = "nan"
		case math.IsInf(n.f, 1):
			v = "inf"
		case math.IsInf(n.f, -1):
			v = "-inf"
		default:
			v = strconv.FormatFloat(n.f, 'g', -1, 64)
		}
	case kBool:
		v = strconv.FormatBool(n.b)
	default:
		v = formatDatetime(n.kind, n.dt)
	}
	return map[string]any{"type": n.kind.String(), "value": v}
}

func formatDatetime(k kind, d datetime) string {
	date := fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day)
	tm := fmt.Sprintf("%02d:%02d:%02d", d.hour, d.minute, d.second)
	if d.nanos != 0 {
		tm += strings.TrimRight(fmt.Sprintf(".%09d", d.nanos), "0")
	}
	switch k {
	case kDateLocal:
		return date
	case kTimeLocal:
		return tm
	case kDatetimeLocal:
		return date + "T" + tm
	}
	off := d.offset
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	return fmt.Sprintf("%sT%s%c%02d:%02d", date, tm, sign, off/3600, off/60%60)
}

// sameTagged compares a result with the expected JSON the way the values mean
// the same thing: floats as numbers (any NaN equals any NaN), datetimes as the
// instants or wall times they name, everything else as written.
func sameTagged(want, have any, path string) string {
	switch w := want.(type) {
	case []any:
		h, ok := have.([]any)
		if !ok {
			return fmt.Sprintf("%s: want an array, have %v", path, have)
		}
		if len(w) != len(h) {
			return fmt.Sprintf("%s: want %d elements, have %d", path, len(w), len(h))
		}
		for i := range w {
			if msg := sameTagged(w[i], h[i], fmt.Sprintf("%s[%d]", path, i)); msg != "" {
				return msg
			}
		}
		return ""
	case map[string]any:
		h, ok := have.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: want a table, have %v", path, have)
		}
		if isTaggedValue(w) {
			if !isTaggedValue(h) {
				return fmt.Sprintf("%s: want %v, have %v", path, w, h)
			}
			return sameValue(w["type"].(string), w["value"].(string), h["type"].(string), h["value"].(string), path)
		}
		if len(w) != len(h) {
			return fmt.Sprintf("%s: want keys %v, have %v", path, keysOf(w), keysOf(h))
		}
		for k := range w {
			hv, ok := h[k]
			if !ok {
				return fmt.Sprintf("%s: missing key %q", path, k)
			}
			if msg := sameTagged(w[k], hv, path+"."+k); msg != "" {
				return msg
			}
		}
		return ""
	}
	return fmt.Sprintf("%s: unexpected expected value %v", path, want)
}

// The comparison is itself a test subject: one that equated too much would
// let wrong values through the whole suite.
func TestTheComparisonTellsValuesApart(t *testing.T) {
	differ := [][4]string{
		{"datetime", "1979-05-27T07:32:00Z", "datetime", "1979-05-27T00:32:00-07:00"},
		{"datetime", "1979-05-27T07:32:00+09:00", "datetime", "1979-05-26T22:32:00Z"},
		{"datetime-local", "1979-05-27T07:32:00", "datetime-local", "1979-05-27T07:32:01"},
		{"float", "0.0", "float", "-0.0"},
		{"float", "nan", "float", "inf"},
		{"integer", "1", "integer", "2"},
		{"integer", "1", "float", "1"},
		{"string", "a", "string", "a "},
	}
	for _, c := range differ {
		if sameValue(c[0], c[1], c[2], c[3], "k") == "" {
			t.Errorf("%s %q and %s %q compare equal", c[0], c[1], c[2], c[3])
		}
	}
	same := [][4]string{
		{"datetime", "1979-05-27T07:32:00Z", "datetime", "1979-05-27 07:32:00z"},
		{"datetime", "1979-05-27T07:32:00Z", "datetime", "1979-05-27T07:32:00+00:00"},
		{"float", "1e2", "float", "100"},
		{"float", "-nan", "float", "nan"},
		{"time-local", "07:32:00.5", "time-local", "07:32:00.500"},
	}
	for _, c := range same {
		if msg := sameValue(c[0], c[1], c[2], c[3], "k"); msg != "" {
			t.Errorf("%s", msg)
		}
	}
}

func isTaggedValue(m map[string]any) bool {
	if len(m) != 2 {
		return false
	}
	_, t := m["type"].(string)
	_, v := m["value"].(string)
	return t && v
}

func keysOf(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

var datetimeLayouts = map[string]string{
	"datetime":       time.RFC3339Nano,
	"datetime-local": "2006-01-02T15:04:05.999999999",
	"date-local":     "2006-01-02",
	"time-local":     "15:04:05.999999999",
}

func sameValue(wantType, want, haveType, have, path string) string {
	mismatch := fmt.Sprintf("%s: want %s %q, have %s %q", path, wantType, want, haveType, have)
	if wantType != haveType {
		return mismatch
	}
	switch wantType {
	case "float":
		if strings.HasSuffix(want, "nan") || strings.HasSuffix(have, "nan") {
			if strings.TrimLeft(want, "+-") != strings.TrimLeft(have, "+-") {
				return mismatch
			}
			return ""
		}
		wf, err1 := strconv.ParseFloat(want, 64)
		hf, err2 := strconv.ParseFloat(have, 64)
		if err1 != nil || err2 != nil || wf != hf || math.Signbit(wf) != math.Signbit(hf) {
			return mismatch
		}
	case "datetime", "datetime-local", "date-local", "time-local":
		norm := strings.NewReplacer(" ", "T", "t", "T", "z", "Z")
		layout := datetimeLayouts[wantType]
		wt, err1 := time.Parse(layout, norm.Replace(want))
		ht, err2 := time.Parse(layout, norm.Replace(have))
		if err1 != nil || err2 != nil || !wt.Equal(ht) {
			return mismatch
		}
		// The same instant is not enough: the offset as written is part
		// of the value, and a parser that turned every offset into Z
		// would otherwise pass.
		_, wo := wt.Zone()
		_, ho := ht.Zone()
		if wo != ho {
			return mismatch
		}
	default:
		if want != have {
			return mismatch
		}
	}
	return ""
}

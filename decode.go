package toml

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"time"
)

// Option changes how a document is decoded.
type Option func(*config)

type config struct {
	allowUnknown bool
}

// AllowUnknownKeys accepts keys the Go value has no place for, instead of
// failing with UnknownKeysError. They are skipped.
func AllowUnknownKeys() Option {
	return func(c *config) { c.allowUnknown = true }
}

// Unmarshaler is implemented by a type that decodes a TOML value itself, for
// a key whose value may take more than one shape (a string or an array, say).
// UnmarshalTOML receives the value as an `any` target would: map[string]any,
// []any, string, int64, float64, bool, time.Time, LocalDateTime, LocalDate or
// LocalTime. Every key under it counts as decoded.
type Unmarshaler interface {
	UnmarshalTOML(any) error
}

// MetaData describes the document that was decoded.
type MetaData struct {
	root *table
}

// IsDefined reports whether the document wrote the key: a value, or a table
// whether written by a header, by dotted keys or implied by a deeper header.
// A key inside an array of tables is not reached. No key parts at all is
// false; one empty part is the key "" and is true when the document wrote it.
func (md MetaData) IsDefined(key ...string) bool {
	if md.root == nil || len(key) == 0 {
		return false
	}
	t := md.root
	for i, k := range key {
		n, ok := t.entries[k]
		if !ok {
			return false
		}
		if i == len(key)-1 {
			return true
		}
		if n.kind != kTable {
			return false
		}
		t = n.tbl
	}
	return false
}

// DecodeFile reads the file at path and decodes it into v. A file larger than
// 1 MiB is refused without being read whole.
func DecodeFile(path string, v any, opts ...Option) (MetaData, error) {
	f, err := os.Open(path)
	if err != nil {
		return MetaData{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return MetaData{}, err
	}
	return Decode(string(b), v, opts...)
}

// Decode decodes the TOML document data into v, which must be a non-nil
// pointer. Values already in v that the document does not set are kept, so
// v may be filled with defaults first.
//
// The error is a *ParseError when data is not TOML 1.1.0, a *DecodeError
// when a value does not fit, or an *UnknownKeysError when the document has
// keys v has no place for. After a *DecodeError, v may hold part of the
// document; after an *UnknownKeysError, every known key has been decoded.
func Decode(data string, v any, opts ...Option) (MetaData, error) {
	var c config
	for _, o := range opts {
		o(&c)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return MetaData{}, errors.New("toml: Decode needs a non-nil pointer")
	}
	root, err := parse(data)
	if err != nil {
		return MetaData{}, err
	}
	md := MetaData{root: root}
	d := &decoder{src: data, allowUnknown: c.allowUnknown, seen: map[string]bool{}}
	if err := d.value(&node{kind: kTable, tbl: root}, rv.Elem(), nil); err != nil {
		return md, err
	}
	if len(d.unknown) > 0 {
		sort.SliceStable(d.unknown, func(i, j int) bool { return d.unknown[i].off < d.unknown[j].off })
		keys := make([]Key, len(d.unknown))
		for i, u := range d.unknown {
			keys[i] = u.key
		}
		return md, &UnknownKeysError{Keys: keys}
	}
	return md, nil
}

type unknownKey struct {
	key Key
	off int
}

type decoder struct {
	src          string
	allowUnknown bool
	unknown      []unknownKey
	seen         map[string]bool // unknown keys already listed (arrays of tables repeat them)
}

var (
	timeType          = reflect.TypeOf(time.Time{})
	durationType      = reflect.TypeOf(time.Duration(0))
	localDateType     = reflect.TypeOf(LocalDate{})
	localTimeType     = reflect.TypeOf(LocalTime{})
	localDateTimeType = reflect.TypeOf(LocalDateTime{})
	textUnmarshaler   = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	unmarshaler       = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

// wrap returns err from a target's own method as a DecodeError that
// errors.Is and errors.As see through. Such errors often quote their input
// (time.ParseDuration does), so the message keeps only its start; the whole
// error stays reachable through Unwrap.
func (d *decoder) wrap(n *node, key Key, err error) error {
	e := d.fail(n, key, "%s", clipTo(err.Error(), 200)).(*DecodeError)
	e.err = err
	return e
}

func (d *decoder) fail(n *node, key Key, format string, args ...any) error {
	line, _ := position(d.src, n.off)
	return &DecodeError{Key: key, Line: line, Msg: fmt.Sprintf(format, args...)}
}

func (d *decoder) mismatch(n *node, key Key, v reflect.Value) error {
	return d.fail(n, key, "cannot decode %s into %s", n.describe(), v.Type())
}

// value decodes n into v, which is settable.
func (d *decoder) value(n *node, v reflect.Value, key Key) error {
	// A nil pointer gets a new value; a pointer already set is decoded
	// through, so what it points to keeps the defaults it holds. A pointer
	// type can point to itself (`type P *P`), which would never end.
	for depth := 0; v.Kind() == reflect.Pointer; depth++ {
		if depth == 64 {
			return d.fail(n, key, "cannot decode into %s: its pointers do not end", v.Type())
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	if v.CanAddr() && v.Addr().Type().Implements(unmarshaler) {
		if err := v.Addr().Interface().(Unmarshaler).UnmarshalTOML(plain(n)); err != nil {
			return d.wrap(n, key, err)
		}
		return nil
	}

	if v.Kind() == reflect.Interface {
		if v.NumMethod() != 0 {
			return d.fail(n, key, "cannot decode into the interface %s", v.Type())
		}
		v.Set(reflect.ValueOf(plain(n)))
		return nil
	}

	switch v.Type() {
	case timeType, localDateType, localTimeType, localDateTimeType:
		return d.datetimeValue(n, v, key)
	case durationType:
		// "1m30s", as time.ParseDuration reads it. An integer falls through
		// to the integer case and counts nanoseconds, as BurntSushi did.
		if n.kind == kString {
			dur, err := time.ParseDuration(n.str)
			if err != nil {
				return d.wrap(n, key, err)
			}
			v.SetInt(int64(dur))
			return nil
		}
	}

	if v.CanAddr() && v.Addr().Type().Implements(textUnmarshaler) {
		var text string
		switch n.kind {
		case kString:
			text = n.str
		case kDatetime, kDatetimeLocal, kDateLocal, kTimeLocal:
			text = n.datetimeText()
		default:
			return d.mismatch(n, key, v)
		}
		if err := v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(text)); err != nil {
			return d.wrap(n, key, err)
		}
		return nil
	}

	switch n.kind {
	case kString:
		if v.Kind() != reflect.String {
			return d.mismatch(n, key, v)
		}
		v.SetString(n.str)
	case kBool:
		if v.Kind() != reflect.Bool {
			return d.mismatch(n, key, v)
		}
		v.SetBool(n.b)
	case kInteger:
		return d.integer(n, v, key)
	case kFloat:
		switch v.Kind() {
		case reflect.Float32, reflect.Float64:
			if v.OverflowFloat(n.f) {
				return d.fail(n, key, "%g does not fit in %s", n.f, v.Type())
			}
			v.SetFloat(n.f)
		default:
			return d.mismatch(n, key, v)
		}
	case kArray:
		return d.array(n, v, key)
	case kTable:
		switch v.Kind() {
		case reflect.Struct:
			return d.structTable(n.tbl, v, key)
		case reflect.Map:
			return d.mapTable(n, v, key)
		default:
			return d.mismatch(n, key, v)
		}
	default:
		return d.mismatch(n, key, v)
	}
	return nil
}

func (d *decoder) integer(n *node, v reflect.Value, key Key) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.OverflowInt(n.i) {
			return d.fail(n, key, "%d does not fit in %s", n.i, v.Type())
		}
		v.SetInt(n.i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if n.i < 0 || v.OverflowUint(uint64(n.i)) {
			return d.fail(n, key, "%d does not fit in %s", n.i, v.Type())
		}
		v.SetUint(uint64(n.i))
	case reflect.Float32, reflect.Float64:
		f := float64(n.i)
		// TOML asks for integers to be kept losslessly; a float64 holds every
		// integer up to 2^53 exactly, and this one may be larger.
		if f >= math.MaxInt64 || int64(f) != n.i || v.OverflowFloat(f) || float64(float32(f)) != f && v.Kind() == reflect.Float32 {
			return d.fail(n, key, "%d cannot be held exactly in %s", n.i, v.Type())
		}
		v.SetFloat(f)
	default:
		return d.mismatch(n, key, v)
	}
	return nil
}

// array replaces a slice whole: the document's elements, decoded onto zero
// values. A Go array takes up to its length; the rest is zeroed.
func (d *decoder) array(n *node, v reflect.Value, key Key) error {
	switch v.Kind() {
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), len(n.arr), len(n.arr))
		for i, e := range n.arr {
			if err := d.value(e, s.Index(i), key); err != nil {
				return err
			}
		}
		v.Set(s)
	case reflect.Array:
		if len(n.arr) > v.Len() {
			return d.fail(n, key, "%d elements do not fit in %s", len(n.arr), v.Type())
		}
		z := reflect.New(v.Type()).Elem()
		for i, e := range n.arr {
			if err := d.value(e, z.Index(i), key); err != nil {
				return err
			}
		}
		v.Set(z)
	default:
		return d.mismatch(n, key, v)
	}
	return nil
}

func (d *decoder) structTable(t *table, v reflect.Value, key Key) error {
	fields := fieldsOf(v.Type())
	// TOML keys are case-sensitive, so `name` and `NAME` are two keys, but
	// the case-insensitive match can lead both to one field. Letting the
	// later one win would drop the other silently.
	setBy := map[*field]string{}
	for _, k := range t.keys {
		n := t.entries[k]
		f, ok := fields.lookup(k)
		if !ok {
			d.unknownTree(n, key.child(k))
			continue
		}
		if other, ok := setBy[f]; ok {
			return d.fail(n, key.child(k), "keys %s and %s both set the field %s",
				quoteKey(clip(other)), quoteKey(clip(k)), f.goName)
		}
		setBy[f] = k
		fv, err := d.fieldByIndex(v, f.index, n, key.child(k))
		if err != nil {
			return err
		}
		if err := d.value(n, fv, key.child(k)); err != nil {
			return err
		}
	}
	return nil
}

// fieldByIndex walks to a promoted field, creating the embedded structs on
// the way that are nil pointers.
func (d *decoder) fieldByIndex(v reflect.Value, index []int, n *node, key Key) (reflect.Value, error) {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, d.fail(n, key, "cannot set the field through the unexported embedded pointer %s", v.Type())
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, nil
}

// mapTable writes the document's keys into a map, keeping the entries it
// already holds. An entry the document writes is decoded on top of the one
// already there.
func (d *decoder) mapTable(n *node, v reflect.Value, key Key) error {
	t := v.Type()
	if t.Key().Kind() != reflect.String {
		return d.fail(n, key, "cannot decode a table into %s: its keys are not strings", t)
	}
	if v.IsNil() {
		v.Set(reflect.MakeMap(t))
	}
	for _, k := range n.tbl.keys {
		mk := reflect.ValueOf(k).Convert(t.Key())
		elem := reflect.New(t.Elem()).Elem()
		if old := v.MapIndex(mk); old.IsValid() {
			elem.Set(old)
		}
		if err := d.value(n.tbl.entries[k], elem, key.child(k)); err != nil {
			return err
		}
		v.SetMapIndex(mk, elem)
	}
	return nil
}

// unknownTree lists an unknown key and, when it holds a table or tables,
// every key beneath it, as BurntSushi's Undecoded() did. Tools name keys they
// have removed — `workspace.workspace_dir` under a `[workspace]` that is
// gone — and can find them only if the key itself is listed, not just the
// table it was in.
func (d *decoder) unknownTree(n *node, key Key) {
	if d.allowUnknown {
		return
	}
	d.unknownKey(key, n.off)
	switch n.kind {
	case kTable:
		for _, k := range n.tbl.keys {
			d.unknownTree(n.tbl.entries[k], key.child(k))
		}
	case kArray:
		for _, e := range n.arr {
			if e.kind == kTable {
				d.unknownTree(e, key)
			}
		}
	}
}

func (d *decoder) unknownKey(key Key, off int) {
	s := key.String()
	if d.seen[s] {
		return
	}
	d.seen[s] = true
	d.unknown = append(d.unknown, unknownKey{key, off})
}

func (d *decoder) datetimeValue(n *node, v reflect.Value, key Key) error {
	switch {
	case v.Type() == timeType && n.kind == kDatetime:
		v.Set(reflect.ValueOf(n.dt.instant()))
	case v.Type() == timeType && n.kind == kString:
		t, err := time.Parse(time.RFC3339Nano, n.str)
		if err != nil {
			return d.wrap(n, key, err)
		}
		// time.Parse names the zone after time.Local when the offsets
		// agree; the value keeps the offset as written, as a datetime does.
		_, off := t.Zone()
		v.Set(reflect.ValueOf(t.In(zoneAt(off))))
	case v.Type() == localDateTimeType && n.kind == kDatetimeLocal:
		v.Set(reflect.ValueOf(LocalDateTime{n.dt.localDate(), n.dt.localTime()}))
	case v.Type() == localDateType && n.kind == kDateLocal:
		v.Set(reflect.ValueOf(n.dt.localDate()))
	case v.Type() == localTimeType && n.kind == kTimeLocal:
		v.Set(reflect.ValueOf(n.dt.localTime()))
	case v.Type() == timeType && (n.kind == kDatetimeLocal || n.kind == kDateLocal || n.kind == kTimeLocal):
		return d.fail(n, key, "%s has no offset, so it is not an instant; decode it into toml.%s",
			n.describe(), localTypeFor(n.kind))
	default:
		return d.mismatch(n, key, v)
	}
	return nil
}

func localTypeFor(k kind) string {
	switch k {
	case kDateLocal:
		return "LocalDate"
	case kTimeLocal:
		return "LocalTime"
	}
	return "LocalDateTime"
}

// plain is a node as the value an `any` receives: map[string]any, []any,
// string, int64, float64, bool, time.Time, LocalDateTime, LocalDate or
// LocalTime.
func plain(n *node) any {
	switch n.kind {
	case kString:
		return n.str
	case kInteger:
		return n.i
	case kFloat:
		return n.f
	case kBool:
		return n.b
	case kDatetime:
		return n.dt.instant()
	case kDatetimeLocal:
		return LocalDateTime{n.dt.localDate(), n.dt.localTime()}
	case kDateLocal:
		return n.dt.localDate()
	case kTimeLocal:
		return n.dt.localTime()
	case kArray:
		a := make([]any, len(n.arr))
		for i, e := range n.arr {
			a[i] = plain(e)
		}
		return a
	default:
		m := make(map[string]any, len(n.tbl.keys))
		for _, k := range n.tbl.keys {
			m[k] = plain(n.tbl.entries[k])
		}
		return m
	}
}

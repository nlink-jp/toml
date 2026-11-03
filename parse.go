package toml

import (
	"strings"
	"unicode/utf8"
)

const (
	// maxInputBytes bounds the document, and with it the tree: a node costs
	// far more than the bytes that wrote it, so a document of tiny values
	// builds a tree about 85 times its size (measured: 16 MiB of `0,` in one
	// array built 1.3 GiB). Configuration files are a few kilobytes; 1 MiB
	// keeps the worst case under 100 MiB.
	maxInputBytes = 1 << 20
	// maxDepth bounds how deep tables and arrays nest. Decoding walks the
	// tree recursively, so an unbounded depth (`a.a.a.…` repeated a million
	// times) would exhaust the stack — a crash, not an error. toml-test
	// suggests 128 or 256 as more than any real document needs.
	maxDepth = 128
)

// parseFail carries a ParseError out of the recursive descent. parse recovers
// it and nothing else: any other panic is a bug and is left to surface.
type parseFail struct{ err *ParseError }

type parser struct {
	src         string
	pos         int
	root        *table
	cur         *table    // the table key/value pairs are written into
	path        []keyPart // cur's key from the root, for error messages
	section     int       // the current header's section
	nextSection int
	depth       int // nesting of arrays and inline tables being read
}

// parse reads a whole document into a tree.
func parse(src string) (root *table, err error) {
	if len(src) > maxInputBytes {
		return nil, &ParseError{Line: 1, Column: 1, Msg: "document is larger than 1 MiB"}
	}
	if !utf8.ValidString(src) {
		return nil, newParseError(src, firstInvalidUTF8(src), "document is not valid UTF-8")
	}
	p := &parser{src: src, root: newTable(headerTable, 0), nextSection: 1}
	p.cur = p.root
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(parseFail)
			if !ok {
				panic(r)
			}
			root, err = nil, f.err
		}
	}()
	p.document()
	if off, ok := tooDeep(p.root); ok {
		p.failAt(off, "tables and arrays nest deeper than %d levels", maxDepth)
	}
	return p.root, nil
}

func firstInvalidUTF8(s string) int {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return len(s)
}

func (p *parser) fail(format string, args ...any) {
	p.failAt(p.pos, format, args...)
}

func (p *parser) failAt(off int, format string, args ...any) {
	panic(parseFail{newParseError(p.src, off, format, args...)})
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) at(s string) bool { return strings.HasPrefix(p.src[p.pos:], s) }

// describe names the character at the current position for an error message.
func (p *parser) describe() string {
	if p.eof() {
		return "end of document"
	}
	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	switch {
	case r == '\n' || p.at("\r\n"):
		return "end of line"
	case r < 0x20 || r == 0x7f:
		return "control character U+" + hex4(r)
	default:
		return "'" + string(r) + "'"
	}
}

func hex4(r rune) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[r>>12&0xf], digits[r>>8&0xf], digits[r>>4&0xf], digits[r&0xf]})
}

func (p *parser) document() {
	if p.at(bom) {
		p.pos += len(bom)
	}
	for {
		p.skipWS()
		if p.eof() {
			return
		}
		switch c := p.peek(); {
		case c == '\n' || c == '\r':
			p.newline()
		case c == '#':
			p.comment()
		case c == '[':
			p.header()
			p.endOfLine()
		default:
			p.keyval(p.cur, p.section, p.path)
			p.endOfLine()
		}
	}
}

func (p *parser) skipWS() {
	for !p.eof() && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

// newline consumes LF or CRLF. A carriage return on its own is not a newline.
func (p *parser) newline() {
	switch {
	case p.peek() == '\n':
		p.pos++
	case p.at("\r\n"):
		p.pos += 2
	default:
		p.fail("expected a newline, found %s", p.describe())
	}
}

// comment consumes a comment up to, not including, the newline that ends it.
func (p *parser) comment() {
	p.pos++ // '#'
	for !p.eof() {
		c := p.src[p.pos]
		if c == '\n' || p.at("\r\n") {
			return
		}
		if (c < 0x20 && c != '\t') || c == 0x7f {
			p.fail("%s is not allowed in a comment", p.describe())
		}
		p.pos++
	}
}

// endOfLine requires that nothing but whitespace and a comment follows a
// key/value pair or a header on its line.
func (p *parser) endOfLine() {
	p.skipWS()
	if p.peek() == '#' {
		p.comment()
	}
	if p.eof() {
		return
	}
	if p.peek() != '\n' && !p.at("\r\n") {
		p.fail("expected the end of the line, found %s", p.describe())
	}
	p.newline()
}

// skipBlank skips whitespace, comments and newlines: what may sit between
// the elements of an array or an inline table.
func (p *parser) skipBlank() {
	for {
		p.skipWS()
		switch {
		case p.eof():
			return
		case p.peek() == '#':
			p.comment()
		case p.peek() == '\n' || p.peek() == '\r':
			p.newline()
		default:
			return
		}
	}
}

// keyPart is one part of a dotted key and where it was written.
type keyPart struct {
	name string
	off  int
}

func isBare(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// key reads a bare, quoted or dotted key. It stops at the first character
// after the key, skipping whitespace around the dots only.
func (p *parser) key() []keyPart {
	var parts []keyPart
	for {
		off := p.pos
		switch c := p.peek(); {
		case c == '"':
			if p.at(`"""`) {
				p.fail("a multi-line string cannot be a key")
			}
			parts = append(parts, keyPart{p.basicString(), off})
		case c == '\'':
			if p.at("'''") {
				p.fail("a multi-line string cannot be a key")
			}
			parts = append(parts, keyPart{p.literalString(), off})
		case isBare(c):
			for !p.eof() && isBare(p.src[p.pos]) {
				p.pos++
			}
			parts = append(parts, keyPart{p.src[off:p.pos], off})
		default:
			p.fail("expected a key, found %s", p.describe())
		}
		p.skipWS()
		if p.peek() != '.' {
			return parts
		}
		if len(parts) == maxDepth {
			p.fail("a key has more than %d parts", maxDepth)
		}
		p.pos++
		p.skipWS()
	}
}

// keyval reads `key = value` and writes it into t. section is the header (or
// inline table) the pair belongs to, which decides what dotted keys may reach.
// prefix is t's key, so that messages name keys in full; inside an inline
// table it is empty and keys are named from that table.
func (p *parser) keyval(t *table, section int, prefix []keyPart) {
	parts := p.key()
	if p.peek() != '=' {
		p.fail("expected '=' after the key, found %s", p.describe())
	}
	p.pos++
	p.skipWS()
	v := p.value()
	p.assign(t, section, prefix, parts, v)
}

// assign writes v under a dotted key, creating the tables the key's leading
// parts name.
func (p *parser) assign(t *table, section int, prefix, parts []keyPart, v *node) {
	name := func(n int) string { return dotted(prefix, parts[:n]) }
	for i, part := range parts[:len(parts)-1] {
		n, ok := t.entries[part.name]
		if !ok {
			child := newTable(dottedTable, section)
			t.set(part.name, &node{kind: kTable, off: part.off, tbl: child})
			t = child
			continue
		}
		if n.kind != kTable {
			p.failAt(part.off, "key %s is already %s, not a table", name(i+1), n.describe())
		}
		switch n.tbl.state {
		case implicitTable:
			n.tbl.state, n.tbl.section = dottedTable, section
		case dottedTable:
			if n.tbl.section != section {
				p.failAt(part.off, "table %s was defined by dotted keys under another header; it cannot be added to here", name(i+1))
			}
		case headerTable:
			p.failAt(part.off, "table %s is defined by a header; dotted keys cannot add to it", name(i+1))
		case inlineTable:
			p.failAt(part.off, "table %s is an inline table; nothing can be added to it", name(i+1))
		}
		t = n.tbl
	}
	last := parts[len(parts)-1]
	if _, ok := t.entries[last.name]; ok {
		p.failAt(last.off, "key %s is defined twice", name(len(parts)))
	}
	v.off = last.off
	t.set(last.name, v)
}

// describe names what a node is, with its article, for error messages.
func (n *node) describe() string {
	switch {
	case n.kind == kArray && n.aot:
		return "an array of tables"
	case n.kind == kTable && n.tbl.state == inlineTable:
		return "an inline table"
	case n.kind == kInteger || n.kind == kArray:
		return "an " + n.kind.String()
	default:
		return "a " + n.kind.String()
	}
}

// dotted writes a key from its parts the way TOML would spell it.
func dotted(groups ...[]keyPart) string {
	var names []string
	for _, parts := range groups {
		for _, part := range parts {
			names = append(names, quoteKey(clip(part.name)))
		}
	}
	return strings.Join(names, ".")
}

// quoteKey writes a key the way it would have to be written in TOML.
func quoteKey(k string) string {
	bare := k != ""
	for i := 0; i < len(k); i++ {
		if !isBare(k[i]) {
			bare = false
			break
		}
	}
	if bare {
		return k
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(k) + `"`
}

// header reads [table] or [[array of tables]] and makes the table it names
// the current one.
func (p *parser) header() {
	array := p.at("[[")
	if array {
		p.pos += 2
	} else {
		p.pos++
	}
	p.skipWS()
	parts := p.key()
	if array {
		if !p.at("]]") {
			p.fail("expected ']]' to close the array of tables header, found %s", p.describe())
		}
		p.pos += 2
	} else {
		if p.peek() != ']' {
			p.fail("expected ']' to close the table header, found %s", p.describe())
		}
		p.pos++
	}

	t := p.root
	for i, part := range parts[:len(parts)-1] {
		n, ok := t.entries[part.name]
		if !ok {
			child := newTable(implicitTable, 0)
			t.set(part.name, &node{kind: kTable, off: part.off, tbl: child})
			t = child
			continue
		}
		switch {
		case n.kind == kTable && n.tbl.state != inlineTable:
			t = n.tbl
		case n.kind == kArray && n.aot:
			t = n.arr[len(n.arr)-1].tbl
		default:
			p.failAt(part.off, "key %s is already %s, not a table", dotted(parts[:i+1]), n.describe())
		}
	}

	last := parts[len(parts)-1]
	n, exists := t.entries[last.name]
	if array {
		elem := newTable(headerTable, 0)
		switch {
		case !exists:
			t.set(last.name, &node{kind: kArray, off: last.off, aot: true,
				arr: []*node{{kind: kTable, off: last.off, tbl: elem}}})
		case n.kind == kArray && n.aot:
			n.arr = append(n.arr, &node{kind: kTable, off: last.off, tbl: elem})
		default:
			p.failAt(last.off, "key %s is already %s, not an array of tables", dotted(parts), n.describe())
		}
		p.cur = elem
	} else {
		switch {
		case !exists:
			child := newTable(headerTable, 0)
			t.set(last.name, &node{kind: kTable, off: last.off, tbl: child})
			p.cur = child
		case n.kind == kTable && n.tbl.state == implicitTable:
			n.tbl.state = headerTable
			p.cur = n.tbl
		case n.kind == kTable:
			p.failAt(last.off, "table %s is defined twice", dotted(parts))
		default:
			p.failAt(last.off, "key %s is already %s, not a table", dotted(parts), n.describe())
		}
	}
	p.path = parts
	p.section = p.nextSection
	p.nextSection++
}

func (p *parser) enter() {
	p.depth++
	if p.depth > maxDepth {
		p.fail("arrays and inline tables nest deeper than %d levels", maxDepth)
	}
}

func (p *parser) leave() { p.depth-- }

// value reads any value.
func (p *parser) value() *node {
	off := p.pos
	switch c := p.peek(); {
	case c == '"':
		if p.at(`"""`) {
			return &node{kind: kString, off: off, str: p.multilineBasicString()}
		}
		return &node{kind: kString, off: off, str: p.basicString()}
	case c == '\'':
		if p.at("'''") {
			return &node{kind: kString, off: off, str: p.multilineLiteralString()}
		}
		return &node{kind: kString, off: off, str: p.literalString()}
	case c == '[':
		return p.array()
	case c == '{':
		return p.inlineTable()
	case p.at("true") && !p.bareAfter(4):
		p.pos += 4
		return &node{kind: kBool, off: off, b: true}
	case p.at("false") && !p.bareAfter(5):
		p.pos += 5
		return &node{kind: kBool, off: off, b: false}
	case c == '+' || c == '-' || c >= '0' && c <= '9' || c == 'i' || c == 'n':
		return p.numberOrDatetime()
	default:
		p.fail("expected a value, found %s", p.describe())
		return nil
	}
}

// bareAfter reports whether a word continues n bytes ahead, so that `truex`
// is not read as `true` followed by garbage that endOfLine would name less
// helpfully.
func (p *parser) bareAfter(n int) bool {
	return p.pos+n < len(p.src) && isBare(p.src[p.pos+n])
}

func (p *parser) array() *node {
	n := &node{kind: kArray, off: p.pos}
	p.enter()
	p.pos++ // '['
	for {
		p.skipBlank()
		if p.peek() == ']' {
			p.pos++
			p.leave()
			return n
		}
		n.arr = append(n.arr, p.value())
		p.skipBlank()
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
			p.pos++
			p.leave()
			return n
		default:
			p.fail("expected ',' or ']' in an array, found %s", p.describe())
		}
	}
}

// inlineTable reads { … }. TOML 1.1 lets it span lines and end with a comma.
// Its pairs form their own section, and once the brace closes, the table and
// every table its dotted keys created are sealed.
func (p *parser) inlineTable() *node {
	section := p.nextSection
	p.nextSection++
	t := newTable(dottedTable, section)
	n := &node{kind: kTable, off: p.pos, tbl: t}
	p.enter()
	p.pos++ // '{'
	for {
		p.skipBlank()
		if p.peek() == '}' {
			p.pos++
			break
		}
		p.keyval(t, section, nil)
		p.skipBlank()
		if p.peek() == ',' {
			p.pos++
			continue
		}
		if p.peek() == '}' {
			p.pos++
			break
		}
		p.fail("expected ',' or '}' in an inline table, found %s", p.describe())
	}
	p.leave()
	seal(t, section)
	return n
}

func seal(t *table, section int) {
	t.state = inlineTable
	for _, n := range t.entries {
		if n.kind == kTable && n.tbl.state == dottedTable && n.tbl.section == section {
			seal(n.tbl, section)
		}
	}
}

// tooDeep walks the finished tree without recursion and reports the earliest
// position, in the document, of a node that nests deeper than maxDepth.
// Header keys and dotted keys add depth one part at a time, so only the
// finished tree knows the real depth. Every too-deep node is looked at, not
// the first one met, so the same document always names the same place.
func tooDeep(root *table) (int, bool) {
	type item struct {
		n     *node
		depth int
	}
	first, found := 0, false
	stack := []item{{&node{kind: kTable, tbl: root}, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if it.depth > maxDepth {
			if !found || it.n.off < first {
				first, found = it.n.off, true
			}
			continue
		}
		switch it.n.kind {
		case kTable:
			for _, k := range it.n.tbl.keys {
				stack = append(stack, item{it.n.tbl.entries[k], it.depth + 1})
			}
		case kArray:
			for _, child := range it.n.arr {
				stack = append(stack, item{child, it.depth + 1})
			}
		}
	}
	return first, found
}

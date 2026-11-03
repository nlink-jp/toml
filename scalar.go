package toml

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Strings.

// controlIn reports a character no string may hold literally: the control
// characters other than tab, and DEL. Multi-line strings also allow newlines;
// their callers check those first.
func controlIn(c byte) bool { return (c < 0x20 && c != '\t') || c == 0x7f }

func (p *parser) basicString() string {
	p.pos++ // '"'
	var b strings.Builder
	for {
		if p.eof() {
			p.fail("unterminated string")
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			p.pos++
			return b.String()
		case c == '\\':
			p.escape(&b)
		case c == '\n' || p.at("\r\n"):
			p.fail("a newline is not allowed in a single-line string")
		case controlIn(c):
			p.fail("%s must be escaped in a string", p.describe())
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

func (p *parser) multilineBasicString() string {
	p.pos += 3 // `"""`
	p.trimFirstNewline()
	var b strings.Builder
	for {
		if p.eof() {
			p.fail("unterminated multi-line string")
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			if p.closingQuotes('"', &b) {
				return b.String()
			}
		case c == '\\':
			if p.lineEndingBackslash() {
				continue
			}
			p.escape(&b)
		case c == '\n':
			b.WriteByte('\n')
			p.pos++
		case c == '\r':
			p.newline() // only as part of CRLF, which becomes LF
			b.WriteByte('\n')
		case controlIn(c):
			p.fail("%s must be escaped in a string", p.describe())
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

func (p *parser) literalString() string {
	p.pos++ // '\''
	start := p.pos
	for {
		if p.eof() {
			p.fail("unterminated literal string")
		}
		c := p.src[p.pos]
		switch {
		case c == '\'':
			p.pos++
			return p.src[start : p.pos-1]
		case c == '\n' || p.at("\r\n"):
			p.fail("a newline is not allowed in a single-line string")
		case controlIn(c):
			p.fail("%s is not allowed in a literal string", p.describe())
		default:
			p.pos++
		}
	}
}

func (p *parser) multilineLiteralString() string {
	p.pos += 3 // "'''"
	p.trimFirstNewline()
	var b strings.Builder
	for {
		if p.eof() {
			p.fail("unterminated multi-line literal string")
		}
		c := p.src[p.pos]
		switch {
		case c == '\'':
			if p.closingQuotes('\'', &b) {
				return b.String()
			}
		case c == '\n':
			b.WriteByte('\n')
			p.pos++
		case c == '\r':
			p.newline()
			b.WriteByte('\n')
		case controlIn(c):
			p.fail("%s is not allowed in a literal string", p.describe())
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

// trimFirstNewline drops the newline written right after an opening `"""`.
func (p *parser) trimFirstNewline() {
	if p.peek() == '\n' {
		p.pos++
	} else if p.at("\r\n") {
		p.pos += 2
	}
}

// closingQuotes reads a run of quote characters inside a multi-line string.
// Three of them close the string; up to two more just before those three
// belong to the content (`""""` ends a string with one quote). Six or more
// in a row cannot be read either way.
func (p *parser) closingQuotes(q byte, b *strings.Builder) (closed bool) {
	n := 0
	for p.pos+n < len(p.src) && p.src[p.pos+n] == q {
		n++
	}
	if n < 3 {
		for i := 0; i < n; i++ {
			b.WriteByte(q)
		}
		p.pos += n
		return false
	}
	if n > 5 {
		p.pos += 5
		p.fail("too many quotes in a row at the end of a multi-line string")
	}
	for i := 0; i < n-3; i++ {
		b.WriteByte(q)
	}
	p.pos += n
	return true
}

// lineEndingBackslash handles a backslash that is the last thing on its line
// apart from whitespace: it and every whitespace and newline after it are
// dropped. It reports false, consuming nothing, for any other backslash.
func (p *parser) lineEndingBackslash() bool {
	i := p.pos + 1
	for i < len(p.src) && (p.src[i] == ' ' || p.src[i] == '\t') {
		i++
	}
	if i >= len(p.src) || (p.src[i] != '\n' && !strings.HasPrefix(p.src[i:], "\r\n")) {
		return false
	}
	p.pos = i
	for !p.eof() {
		switch c := p.src[p.pos]; {
		case c == ' ' || c == '\t' || c == '\n':
			p.pos++
		case c == '\r':
			p.newline()
		default:
			return true
		}
	}
	return true
}

// escape reads one escape sequence in a basic string.
func (p *parser) escape(b *strings.Builder) {
	start := p.pos
	p.pos++ // '\\'
	if p.eof() {
		p.fail("unterminated escape sequence")
	}
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'b':
		b.WriteByte('\b')
	case 't':
		b.WriteByte('\t')
	case 'n':
		b.WriteByte('\n')
	case 'f':
		b.WriteByte('\f')
	case 'r':
		b.WriteByte('\r')
	case 'e':
		b.WriteByte(0x1b)
	case '"':
		b.WriteByte('"')
	case '\\':
		b.WriteByte('\\')
	case 'x':
		b.WriteRune(p.hexEscape(start, 2))
	case 'u':
		b.WriteRune(p.hexEscape(start, 4))
	case 'U':
		b.WriteRune(p.hexEscape(start, 8))
	default:
		p.pos--
		r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
		if controlIn(c) || c == ' ' || c == '\n' {
			p.failAt(start, "a backslash must start an escape sequence, found %s after it", p.describe())
		}
		p.failAt(start, "invalid escape sequence \"\\%c\"", r)
	}
}

func (p *parser) hexEscape(start, digits int) rune {
	if p.pos+digits > len(p.src) {
		p.failAt(start, "escape sequence needs %d hexadecimal digits", digits)
	}
	v, err := strconv.ParseUint(p.src[p.pos:p.pos+digits], 16, 32)
	if err != nil || strings.ContainsAny(p.src[p.pos:p.pos+digits], "+-_") {
		p.failAt(start, "escape sequence needs %d hexadecimal digits", digits)
	}
	p.pos += digits
	r := rune(v)
	if !utf8.ValidRune(r) {
		p.failAt(start, "escape sequence U+%X is not a Unicode scalar value", v)
	}
	return r
}

// Numbers and datetimes.

func isTokenChar(c byte) bool {
	return isBare(c) || c == '+' || c == '.' || c == ':'
}

// numberOrDatetime reads an integer, a float or one of the datetime kinds.
// They share a first character, so the whole token is read first and then
// classified.
func (p *parser) numberOrDatetime() *node {
	start := p.pos
	for !p.eof() && isTokenChar(p.src[p.pos]) {
		p.pos++
	}
	tok := p.src[start:p.pos]
	// A space may separate a date from its time: "1979-05-27 07:32:00Z".
	if isDate(tok) && p.pos+3 < len(p.src) && p.src[p.pos] == ' ' &&
		isDigit(p.src[p.pos+1]) && isDigit(p.src[p.pos+2]) && p.src[p.pos+3] == ':' {
		p.pos++
		for !p.eof() && isTokenChar(p.src[p.pos]) {
			p.pos++
		}
		tok = p.src[start:p.pos]
	}
	if len(tok) >= 5 && (isDate(tok[:min(len(tok), 10)]) || tok[2] == ':') {
		return p.datetime(start, tok)
	}
	return p.number(start, tok)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isDate(s string) bool {
	return len(s) == 10 && isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) &&
		s[4] == '-' && isDigit(s[5]) && isDigit(s[6]) && s[7] == '-' && isDigit(s[8]) && isDigit(s[9])
}

func (p *parser) number(start int, tok string) *node {
	n := &node{off: start}
	bad := func() { p.failAt(start, "invalid number %q", clip(tok)) }

	sign, rest := "", tok
	if rest != "" && (rest[0] == '+' || rest[0] == '-') {
		sign, rest = rest[:1], rest[1:]
	}
	switch rest {
	case "inf":
		n.kind, n.f = kFloat, math.Inf(1)
		if sign == "-" {
			n.f = math.Inf(-1)
		}
		return n
	case "nan":
		n.kind, n.f = kFloat, math.NaN()
		return n
	}

	if len(rest) > 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'o' || rest[1] == 'b') {
		if sign != "" {
			p.failAt(start, "a sign is not allowed on a hexadecimal, octal or binary integer")
		}
		base := map[byte]int{'x': 16, 'o': 8, 'b': 2}[rest[1]]
		digits := rest[2:]
		if !underscoresBetweenDigits(digits, func(c byte) bool { return digitOf(c) < base }) {
			bad()
		}
		v, err := strconv.ParseUint(strings.ReplaceAll(digits, "_", ""), base, 64)
		if err != nil || v > math.MaxInt64 {
			p.failAt(start, "integer %s does not fit in 64 bits", clip(tok))
		}
		n.kind, n.i = kInteger, int64(v)
		return n
	}

	if !strings.ContainsAny(rest, ".eE") {
		if !decimalInt(rest) {
			bad()
		}
		v, err := strconv.ParseInt(sign+strings.ReplaceAll(rest, "_", ""), 10, 64)
		if err != nil {
			p.failAt(start, "integer %s does not fit in 64 bits", clip(tok))
		}
		n.kind, n.i = kInteger, v
		return n
	}

	// A float: an integer part, then a fraction and/or an exponent.
	intPart, frac, exp := rest, "", ""
	hasFrac, hasExp := false, false
	if i := strings.IndexAny(intPart, "eE"); i >= 0 {
		intPart, exp, hasExp = intPart[:i], intPart[i+1:], true
	}
	if i := strings.IndexByte(intPart, '.'); i >= 0 {
		intPart, frac, hasFrac = intPart[:i], intPart[i+1:], true
	}
	if !decimalInt(intPart) {
		bad()
	}
	if hasFrac && !underscoresBetweenDigits(frac, isDigit) {
		bad()
	}
	if hasExp {
		if exp != "" && (exp[0] == '+' || exp[0] == '-') {
			exp = exp[1:]
		}
		if !underscoresBetweenDigits(exp, isDigit) {
			bad()
		}
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(sign+rest, "_", ""), 64)
	if err != nil && math.IsInf(v, 0) {
		p.failAt(start, "float %s is out of range", clip(tok))
	} else if err != nil {
		bad()
	}
	n.kind, n.f = kFloat, v
	return n
}

func digitOf(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 99
}

// underscoresBetweenDigits accepts one or more digits where every underscore
// has a digit on each side.
func underscoresBetweenDigits(s string, digit func(byte) bool) bool {
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '_':
			if s[i-1] == '_' {
				return false
			}
		case !digit(s[i]):
			return false
		}
	}
	return true
}

// decimalInt is the unsigned part of a decimal integer: no leading zero
// unless the number is zero itself.
func decimalInt(s string) bool {
	if !underscoresBetweenDigits(s, isDigit) {
		return false
	}
	return s[0] != '0' || len(s) == 1
}

// datetime reads the four datetime kinds. TOML 1.1 lets the seconds be
// omitted; fractions beyond nanoseconds are truncated, not rounded.
func (p *parser) datetime(start int, tok string) *node {
	bad := func(why string) { p.failAt(start, "invalid datetime %q: %s", clip(tok), why) }
	n := &node{off: start}
	s := tok

	hasDate := len(s) >= 10 && isDate(s[:10])
	if hasDate {
		y, m, d := atoi(s[0:4]), atoi(s[5:7]), atoi(s[8:10])
		if m < 1 || m > 12 {
			bad("month out of range")
		}
		if d < 1 || d > daysIn(y, m) {
			bad("day out of range")
		}
		n.dt.year, n.dt.month, n.dt.day = y, m, d
		s = s[10:]
		if s == "" {
			n.kind = kDateLocal
			return n
		}
		if s[0] != 'T' && s[0] != 't' && s[0] != ' ' {
			bad("expected 'T' or a space between the date and the time")
		}
		s = s[1:]
	}

	// The time: HH:MM, then optionally :SS and a fraction.
	if len(s) < 5 || !isDigit(s[0]) || !isDigit(s[1]) || s[2] != ':' || !isDigit(s[3]) || !isDigit(s[4]) {
		bad("expected a time as HH:MM")
	}
	n.dt.hour, n.dt.minute = atoi(s[0:2]), atoi(s[3:5])
	s = s[5:]
	if s != "" && s[0] == ':' {
		if len(s) < 3 || !isDigit(s[1]) || !isDigit(s[2]) {
			bad("expected seconds as two digits")
		}
		n.dt.second = atoi(s[1:3])
		s = s[3:]
		if s != "" && s[0] == '.' {
			i := 1
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			if i == 1 {
				bad("expected digits after the decimal point")
			}
			digits := s[1:i]
			if len(digits) > 9 {
				digits = digits[:9]
			}
			n.dt.nanos = atoi(digits + strings.Repeat("0", 9-len(digits)))
			s = s[i:]
		}
	}
	if n.dt.hour > 23 {
		bad("hour out of range")
	}
	if n.dt.minute > 59 {
		bad("minute out of range")
	}
	if n.dt.second > 59 {
		bad("second out of range")
	}

	if !hasDate {
		if s != "" {
			bad("unexpected text after the time")
		}
		n.kind = kTimeLocal
		return n
	}
	switch {
	case s == "":
		n.kind = kDatetimeLocal
	case s == "Z" || s == "z":
		n.kind = kDatetime
	case len(s) == 6 && (s[0] == '+' || s[0] == '-') && isDigit(s[1]) && isDigit(s[2]) &&
		s[3] == ':' && isDigit(s[4]) && isDigit(s[5]):
		h, m := atoi(s[1:3]), atoi(s[4:6])
		if h > 23 || m > 59 {
			bad("offset out of range")
		}
		n.kind, n.dt.offset = kDatetime, (h*60+m)*60
		if s[0] == '-' {
			n.dt.offset = -n.dt.offset
		}
	default:
		bad("expected 'Z' or an offset as +HH:MM")
	}
	return n
}

// atoi reads digits already known to be digits.
func atoi(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
	}
	return v
}

func daysIn(year, month int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

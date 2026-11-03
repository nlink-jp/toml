package toml

// The parser's output: a tree of nodes that remembers where each value was
// written. Decoding maps this tree onto Go values; unknown-key detection,
// IsDefined and line numbers in errors all read it.

type kind uint8

const (
	kString kind = iota
	kInteger
	kFloat
	kBool
	kDatetime      // offset date-time
	kDatetimeLocal // local date-time
	kDateLocal
	kTimeLocal
	kArray
	kTable
)

func (k kind) String() string {
	switch k {
	case kString:
		return "string"
	case kInteger:
		return "integer"
	case kFloat:
		return "float"
	case kBool:
		return "bool"
	case kDatetime:
		return "datetime"
	case kDatetimeLocal:
		return "datetime-local"
	case kDateLocal:
		return "date-local"
	case kTimeLocal:
		return "time-local"
	case kArray:
		return "array"
	default:
		return "table"
	}
}

type node struct {
	kind kind
	off  int // byte offset in the document where the value (or its key) starts
	str  string
	i    int64
	f    float64
	b    bool
	dt   datetime
	arr  []*node
	tbl  *table
	aot  bool // an array made by [[header]], which later headers may extend
}

// datetime holds the fields as written. Which fields mean anything depends on
// the node's kind: a local date has no time, a local time no date, and only an
// offset date-time has an offset.
type datetime struct {
	year, month, day            int
	hour, minute, second, nanos int
	offset                      int // seconds east of UTC
}

// How a table came to exist decides what may still be written into it.
type tableState uint8

const (
	// Created as a parent by a header ([a.b] creates a). A later [a] may still
	// define it, and dotted keys may still write into it.
	implicitTable tableState = iota
	// Defined by a [header], or an element of an array of tables. No other
	// header may define it again, and no dotted key may write into it.
	headerTable
	// Defined by dotted keys. Only further dotted keys in the same section
	// (the same header, or the same inline table) may write into it; a header
	// may define a sub-table of it but not the table itself.
	dottedTable
	// An inline table: complete when its closing brace is read.
	inlineTable
)

type table struct {
	keys    []string // in the order written
	entries map[string]*node
	state   tableState
	section int // the section that defined a dottedTable
}

func newTable(state tableState, section int) *table {
	return &table{entries: map[string]*node{}, state: state, section: section}
}

func (t *table) set(key string, n *node) {
	t.keys = append(t.keys, key)
	t.entries[key] = n
}

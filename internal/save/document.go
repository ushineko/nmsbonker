package save

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

/*
A JSON tree that gives back the bytes it was given (R3).

encoding/json into maps reorders keys and re-encodes every string and number,
and a save payload has things a re-encoder would change: technology ids that
carry raw non-UTF-8 bytes between `^` and `#`, forward slashes the game writes
as `\/`, and numbers whose spelling the game chose. Any of those changed is a
byte the game did not write, in a file the game will read.

So every scalar keeps the bytes it was parsed from and writes them back
untouched, and only a node that was assigned a new value is formatted. The
golden test for this package (AC3) is that parsing the user's own saves and
serialising them again reproduces the payload byte for byte.

Whitespace between tokens is not kept: the game writes none, and a
pretty-printed export re-imported through here comes back compact, which is
what the game writes anyway.
*/

// Type is a JSON value's type.
type Type int

// The JSON types.
const (
	TypeNull Type = iota
	TypeBool
	TypeNumber
	TypeString
	TypeArray
	TypeObject
)

// Node is one JSON value.
type Node struct {
	kind    Type
	raw     []byte // scalar source or formatted replacement, including quotes
	members []*Member
	elems   []*Node
}

// Member is one key of an object. The key keeps its quoted source bytes and
// its decoded name; the game's keys carry no escapes, so the two agree, but a
// renamed key (R5.3 export) is formatted from the name.
type Member struct {
	key   []byte
	Name  string
	Value *Node
}

// ErrSyntax reports a payload that is not JSON.
var ErrSyntax = errors.New("not valid JSON")

// Parse reads a payload into a tree (R3.1).
func Parse(b []byte) (*Node, error) {
	p := parser{b: b}
	p.ws()
	n, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.b) {
		return nil, p.errf("trailing data")
	}
	return n, nil
}

// Bytes serialises the tree compactly, the way the game writes it.
func (n *Node) Bytes() []byte {
	var w bytes.Buffer
	n.write(&w, -1)
	return w.Bytes()
}

// Pretty serialises the tree indented, for a person to read.
func (n *Node) Pretty() []byte {
	var w bytes.Buffer
	n.write(&w, 0)
	w.WriteByte('\n')
	return w.Bytes()
}

// Type is the node's type.
func (n *Node) Type() Type { return n.kind }

// Member finds an object member by key.
func (n *Node) Member(key string) *Node {
	if n == nil || n.kind != TypeObject {
		return nil
	}
	for _, m := range n.members {
		if m.Name == key {
			return m.Value
		}
	}
	return nil
}

// Path walks object members by key; nil when any step is missing.
func (n *Node) Path(keys ...string) *Node {
	cur := n
	for _, k := range keys {
		cur = cur.Member(k)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// Members lists an object's members in order.
func (n *Node) Members() []*Member {
	if n == nil {
		return nil
	}
	return n.members
}

// Len is the element or member count.
func (n *Node) Len() int {
	if n == nil {
		return 0
	}
	if n.kind == TypeObject {
		return len(n.members)
	}
	return len(n.elems)
}

// Index is the i-th array element, nil when out of range.
func (n *Node) Index(i int) *Node {
	if n == nil || n.kind != TypeArray || i < 0 || i >= len(n.elems) {
		return nil
	}
	return n.elems[i]
}

// Append adds an element to an array.
func (n *Node) Append(child *Node) {
	if n != nil && n.kind == TypeArray {
		n.elems = append(n.elems, child)
	}
}

// SetIndex replaces an array element.
func (n *Node) SetIndex(i int, child *Node) {
	if n != nil && n.kind == TypeArray && i >= 0 && i < len(n.elems) {
		n.elems[i] = child
	}
}

// Size is the compact serialisation's length in bytes.
func (n *Node) Size() int { return len(n.Bytes()) }

// TypeName is the JSON type, for display.
func (n *Node) TypeName() string {
	if n == nil {
		return "missing"
	}
	switch n.kind {
	case TypeObject:
		return "object"
	case TypeArray:
		return "array"
	case TypeString:
		return "string"
	case TypeNumber:
		return "number"
	case TypeBool:
		return "boolean"
	default:
		return "null"
	}
}

// Truncate keeps the first k elements of an array.
func (n *Node) Truncate(k int) {
	if n != nil && n.kind == TypeArray && k >= 0 && k < len(n.elems) {
		n.elems = n.elems[:k]
	}
}

// Set assigns an object member, replacing one of the same key or appending.
func (n *Node) Set(key string, v *Node) {
	if n == nil || n.kind != TypeObject {
		return
	}
	for _, m := range n.members {
		if m.Name == key {
			m.Value = v
			return
		}
	}
	n.members = append(n.members, &Member{key: quote(key), Name: key, Value: v})
}

// Rename changes a member's key, for the export that names keys (R5.3).
func (m *Member) Rename(name string) {
	m.Name = name
	m.key = quote(name)
}

// Int reads an integer number.
func (n *Node) Int() (int64, bool) {
	if n == nil || n.kind != TypeNumber {
		return 0, false
	}
	v, err := strconv.ParseInt(string(n.raw), 10, 64)
	return v, err == nil
}

// Float reads any number.
func (n *Node) Float() (float64, bool) {
	if n == nil || n.kind != TypeNumber {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(n.raw), 64)
	return v, err == nil
}

// String reads a string, escapes decoded.
func (n *Node) String() (string, bool) {
	if n == nil || n.kind != TypeString {
		return "", false
	}
	return unquote(n.raw), true
}

// Bool reads a boolean.
func (n *Node) Bool() (bool, bool) {
	if n == nil || n.kind != TypeBool {
		return false, false
	}
	return string(n.raw) == "true", true
}

// Raw is the scalar's source bytes, for display.
func (n *Node) Raw() string {
	if n == nil {
		return ""
	}
	if n.kind == TypeObject || n.kind == TypeArray {
		return string(n.Bytes())
	}
	return string(n.raw)
}

// SetInt replaces the value with an integer.
func (n *Node) SetInt(v int64) { n.kind, n.raw = TypeNumber, []byte(strconv.FormatInt(v, 10)) }

// SetUint replaces the value with an unsigned integer.
func (n *Node) SetUint(v uint64) { n.kind, n.raw = TypeNumber, []byte(strconv.FormatUint(v, 10)) }

// SetString replaces the value with a string.
func (n *Node) SetString(s string) { n.kind, n.raw = TypeString, quote(s) }

// SetBool replaces the value with a boolean.
func (n *Node) SetBool(b bool) { n.kind, n.raw = TypeBool, []byte(strconv.FormatBool(b)) }

// NewObject is an empty object.
func NewObject() *Node { return &Node{kind: TypeObject} }

// NewArray is an empty array.
func NewArray() *Node { return &Node{kind: TypeArray} }

// NewInt is an integer node.
func NewInt(v int64) *Node { n := &Node{}; n.SetInt(v); return n }

// NewString is a string node.
func NewString(s string) *Node { n := &Node{}; n.SetString(s); return n }

// Walk visits every node depth-first, members before elements, with the
// member key that led to it ("" for the root and array elements).
func (n *Node) Walk(visit func(key string, node *Node)) {
	n.walk("", visit)
}

func (n *Node) walk(key string, visit func(string, *Node)) {
	if n == nil {
		return
	}
	visit(key, n)
	for _, m := range n.members {
		m.Value.walk(m.Name, visit)
	}
	for _, e := range n.elems {
		e.walk("", visit)
	}
}

// --- serialisation --------------------------------------------------------

func (n *Node) write(w *bytes.Buffer, indent int) {
	switch n.kind {
	case TypeObject:
		w.WriteByte('{')
		for i, m := range n.members {
			if i > 0 {
				w.WriteByte(',')
			}
			newline(w, child(indent))
			w.Write(m.key)
			w.WriteByte(':')
			if indent >= 0 {
				w.WriteByte(' ')
			}
			m.Value.write(w, child(indent))
		}
		if len(n.members) > 0 {
			newline(w, indent)
		}
		w.WriteByte('}')
	case TypeArray:
		w.WriteByte('[')
		for i, e := range n.elems {
			if i > 0 {
				w.WriteByte(',')
			}
			newline(w, child(indent))
			e.write(w, child(indent))
		}
		if len(n.elems) > 0 {
			newline(w, indent)
		}
		w.WriteByte(']')
	default:
		w.Write(n.raw)
	}
}

func child(indent int) int {
	if indent < 0 {
		return -1
	}
	return indent + 1
}

func newline(w *bytes.Buffer, indent int) {
	if indent < 0 {
		return
	}
	w.WriteByte('\n')
	for range indent {
		w.WriteString("  ")
	}
}

// quote formats a string with JSON's escapes. Slashes are left as they are:
// the game's own saves carry model paths as "MODELS/COMMON/…" unescaped, and
// an existing string's escapes are preserved by the tree regardless.
func quote(s string) []byte {
	var w bytes.Buffer
	w.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			w.WriteString(`\"`)
		case '\\':
			w.WriteString(`\\`)
		case '\n':
			w.WriteString(`\n`)
		case '\r':
			w.WriteString(`\r`)
		case '\t':
			w.WriteString(`\t`)
		case '\b':
			w.WriteString(`\b`)
		case '\f':
			w.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&w, `\u%04x`, r)
			} else {
				w.WriteRune(r)
			}
		}
	}
	w.WriteByte('"')
	return w.Bytes()
}

// unquote decodes a quoted string's escapes. Bytes that are not valid UTF-8
// are passed through unchanged rather than replaced, so a decoded value can be
// compared with what the game wrote.
func unquote(raw []byte) string {
	body := raw[1 : len(raw)-1]
	if bytes.IndexByte(body, '\\') < 0 {
		return string(body)
	}
	var w strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' || i+1 >= len(body) {
			w.WriteByte(c)
			continue
		}
		i++
		switch body[i] {
		case '"', '\\', '/':
			w.WriteByte(body[i])
		case 'n':
			w.WriteByte('\n')
		case 'r':
			w.WriteByte('\r')
		case 't':
			w.WriteByte('\t')
		case 'b':
			w.WriteByte('\b')
		case 'f':
			w.WriteByte('\f')
		case 'u':
			if i+4 < len(body) {
				if v, err := strconv.ParseUint(string(body[i+1:i+5]), 16, 32); err == nil {
					r := rune(v) //nolint:gosec // four hex digits
					i += 4
					// A surrogate pair is two escapes; join them.
					if utf16IsHigh(r) && i+6 < len(body) && body[i+1] == '\\' && body[i+2] == 'u' {
						lo, err := strconv.ParseUint(string(body[i+3:i+7]), 16, 32)
						if err == nil && utf16IsLow(rune(lo)) { //nolint:gosec // four hex digits
							r = (r-0xD800)<<10 + (rune(lo) - 0xDC00) + 0x10000 //nolint:gosec // four hex digits
							i += 6
						}
					}
					w.WriteRune(r)
					continue
				}
			}
			w.WriteString(`\u`)
		default:
			w.WriteByte('\\')
			w.WriteByte(body[i])
		}
	}
	return w.String()
}

func utf16IsHigh(r rune) bool { return r >= 0xD800 && r < 0xDC00 }
func utf16IsLow(r rune) bool  { return r >= 0xDC00 && r < 0xE000 }

// --- parsing --------------------------------------------------------------

type parser struct {
	b []byte
	i int
}

// maxDepth bounds nesting so a hostile file cannot exhaust the stack. A save
// nests about fifteen deep.
const maxDepth = 256

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("%w: %s at offset %d", ErrSyntax, fmt.Sprintf(format, args...), p.i)
}

func (p *parser) ws() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (*Node, error) {
	if depth > maxDepth {
		return nil, p.errf("nesting deeper than %d", maxDepth)
	}
	if p.i >= len(p.b) {
		return nil, p.errf("unexpected end")
	}
	switch c := p.b[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		raw, err := p.str()
		if err != nil {
			return nil, err
		}
		return &Node{kind: TypeString, raw: raw}, nil
	case c == 't' || c == 'f' || c == 'n':
		return p.literal()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, p.errf("unexpected byte %q", p.b[p.i])
}

func (p *parser) object(depth int) (*Node, error) {
	n := &Node{kind: TypeObject}
	p.i++ // {
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		return n, nil
	}
	for {
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			return nil, p.errf("expected a key")
		}
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return nil, p.errf("expected ':'")
		}
		p.i++
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		n.members = append(n.members, &Member{key: key, Name: unquote(key), Value: v})
		p.ws()
		if p.i >= len(p.b) {
			return nil, p.errf("unterminated object")
		}
		switch p.b[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return n, nil
		default:
			return nil, p.errf("expected ',' or '}'")
		}
	}
}

func (p *parser) array(depth int) (*Node, error) {
	n := &Node{kind: TypeArray}
	p.i++ // [
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		return n, nil
	}
	for {
		p.ws()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		n.elems = append(n.elems, v)
		p.ws()
		if p.i >= len(p.b) {
			return nil, p.errf("unterminated array")
		}
		switch p.b[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return n, nil
		default:
			return nil, p.errf("expected ',' or ']'")
		}
	}
}

/*
str scans a quoted string and returns it with its quotes.

Every unescaped byte between the quotes is accepted, including the raw
non-UTF-8 bytes the game writes inside hashed technology ids, and nothing is
decoded: the raw bytes are the value this tree promises to give back. Escapes,
though, must be JSON escapes -- `\q` or a short `\u` is a typo in a hand-edited
export, and a typo written into a save is a save the game may not load.
*/
func (p *parser) str() ([]byte, error) {
	start := p.i
	p.i++ // opening quote
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case '\\':
			if p.i+1 >= len(p.b) {
				return nil, p.errf("unterminated escape")
			}
			switch p.b[p.i+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				p.i += 2
			case 'u':
				if p.i+6 > len(p.b) || !isHex(p.b[p.i+2:p.i+6]) {
					return nil, p.errf("malformed \\u escape")
				}
				p.i += 6
			default:
				return nil, p.errf("invalid escape \\%c", p.b[p.i+1])
			}
		case '"':
			p.i++
			return p.b[start:p.i], nil
		default:
			p.i++
		}
	}
	p.i = start
	return nil, p.errf("unterminated string")
}

func isHex(b []byte) bool {
	for _, c := range b {
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return true
}

func (p *parser) literal() (*Node, error) {
	for _, lit := range []string{"true", "false", "null"} {
		if bytes.HasPrefix(p.b[p.i:], []byte(lit)) {
			raw := p.b[p.i : p.i+len(lit)]
			p.i += len(lit)
			kind := TypeBool
			if lit == "null" {
				kind = TypeNull
			}
			return &Node{kind: kind, raw: raw}, nil
		}
	}
	return nil, p.errf("unexpected literal")
}

// number scans a number with JSON's grammar: an optional minus, an integer
// part with no leading zero, an optional fraction with at least one digit, an
// optional exponent with at least one digit. strconv would take `01` and `1.`;
// the game never writes them and a hand-edited file should not either.
func (p *parser) number() (*Node, error) {
	start := p.i
	digits := func() int {
		n := 0
		for p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	if p.i < len(p.b) && p.b[p.i] == '-' {
		p.i++
	}
	if p.i < len(p.b) && p.b[p.i] == '0' {
		p.i++
		if p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '9' {
			p.i = start
			return nil, p.errf("malformed number: leading zero")
		}
	} else if digits() == 0 {
		p.i = start
		return nil, p.errf("malformed number")
	}
	if p.i < len(p.b) && p.b[p.i] == '.' {
		p.i++
		if digits() == 0 {
			p.i = start
			return nil, p.errf("malformed number: no digits after the point")
		}
	}
	if p.i < len(p.b) && (p.b[p.i] == 'e' || p.b[p.i] == 'E') {
		p.i++
		if p.i < len(p.b) && (p.b[p.i] == '+' || p.b[p.i] == '-') {
			p.i++
		}
		if digits() == 0 {
			p.i = start
			return nil, p.errf("malformed number: no exponent digits")
		}
	}
	return &Node{kind: TypeNumber, raw: p.b[start:p.i]}, nil
}

// ValidUTF8 reports whether a decoded string is clean text, for the display
// paths that would otherwise print raw technology-id bytes.
func ValidUTF8(s string) bool { return utf8.ValidString(s) }

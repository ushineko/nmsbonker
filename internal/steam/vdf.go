/*
Package steam locates the No Man's Sky installation by reading Steam's on-disk
manifests (spec 001 R3). It never talks to a running Steam client: the files are
the source of truth, they are readable with the client closed, and a client
integration would be a second way for the tool to be wrong about where the game
is.
*/
package steam

import (
	"strings"
)

// AppID is No Man's Sky on Steam.
const AppID = "275850"

/*
Steam's manifests are Valve KeyValues ("VDF"): quoted keys, quoted values, and
brace-delimited blocks. The parser below is line-based and tolerant rather than
a full VDF implementation, which is what R3.1 asks for. The reason is not
laziness about the grammar but about the failure mode: a strict parser that
rejects a file Steam itself accepts turns "your game is at X" into "nmsbonker
cannot read Steam's library list", and the only two documents read here have a
shape that fits on a page.

What it deliberately does handle: escaped backslashes in Windows-style paths
("D:\\SteamLibrary"), keys with no value that open a block on the next line,
and unexpected nesting depth.
*/

// vdfNode is one KeyValues block.
type vdfNode struct {
	// values holds scalar key/value pairs in this block. Later duplicates win,
	// matching Steam's own last-wins behaviour.
	values map[string]string
	// children preserves block order, which libraryfolders.vdf relies on to
	// present libraries in the order Steam registered them.
	children []vdfChild
}

type vdfChild struct {
	key  string
	node *vdfNode
}

func newVDFNode() *vdfNode { return &vdfNode{values: map[string]string{}} }

// child returns the first sub-block with the given key, or nil.
func (n *vdfNode) child(key string) *vdfNode {
	for _, c := range n.children {
		if strings.EqualFold(c.key, key) {
			return c.node
		}
	}
	return nil
}

// parseVDF reads a KeyValues document into a tree.
//
// It never returns an error: a malformed document yields whatever was
// recognisable, and the caller reports "the game is not in this library"
// rather than "Steam's file is broken". Callers that need a value check for it.
func parseVDF(text string) *vdfNode {
	root := newVDFNode()
	stack := []*vdfNode{root}
	var pendingKey string

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if line == "{" {
			node := newVDFNode()
			top := stack[len(stack)-1]
			top.children = append(top.children, vdfChild{key: pendingKey, node: node})
			stack = append(stack, node)
			pendingKey = ""
			continue
		}
		if line == "}" {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			pendingKey = ""
			continue
		}
		key, value, hasValue := splitVDFLine(line)
		if key == "" {
			continue
		}
		if hasValue {
			stack[len(stack)-1].values[key] = value
			continue
		}
		// A bare key opens a block on the following line.
		pendingKey = key
	}
	return root
}

// splitVDFLine pulls the one or two quoted tokens out of a KeyValues line.
func splitVDFLine(line string) (key, value string, hasValue bool) {
	first, rest, ok := nextQuoted(line)
	if !ok {
		return "", "", false
	}
	second, _, ok := nextQuoted(rest)
	if !ok {
		return first, "", false
	}
	return first, second, true
}

// nextQuoted returns the first quoted token in s and whatever follows it.
//
// Backslash escapes are honoured because Windows library paths arrive as
// "D:\\SteamLibrary": reading that literally would look for a directory whose
// name contains a backslash and report the library as missing.
func nextQuoted(s string) (token, rest string, ok bool) {
	start := strings.IndexByte(s, '"')
	if start < 0 {
		return "", "", false
	}
	var b strings.Builder
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) {
				return "", "", false
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				// Covers \\ and \" and leaves anything else as the literal
				// character, which is what Steam's own reader does.
				b.WriteByte(s[i])
			}
		case '"':
			return b.String(), s[i+1:], true
		default:
			b.WriteByte(s[i])
		}
	}
	return "", "", false
}

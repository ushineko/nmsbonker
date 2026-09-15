package save

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

/*
The key mapping (R3.2, R3.3, R4).

The game obfuscates every JSON key to three characters ("F2P" is Version).
MBINCompiler publishes the table as `mapping.json` beside each release, and
that file is the only source this project uses: it is generated from the same
class metadata the compiler decompiles with, so it tracks the game exactly as
far as the compiler does.

The mapping is not a bijection. A few keys carry two names (`V86`, `NE3`),
depending on which struct they appear in, and this code cannot know which. The
first entry in the file wins in both directions, and the tree on disk is never
rewritten to names, so the ambiguity affects display and path lookup only.
*/

// MappingFile is the asset's name on a release and beside the installed
// compiler.
const MappingFile = "mapping.json"

// Mapping names obfuscated keys. A nil *Mapping is usable and names nothing.
type Mapping struct {
	// LibMBINVersion is the libMBIN the mapping was generated from.
	LibMBINVersion string
	// Entries is how many pairs the file held.
	Entries int
	toName  map[string]string
	toKey   map[string]string
	// ambiguous are names that appear with more than one key. Such a key is
	// never renamed on export, because the import could not know which key
	// to turn it back into. The current mapping has none; the guard is for
	// the release that does.
	ambiguous map[string]bool
}

// ParseMapping reads the release asset.
func ParseMapping(b []byte) (*Mapping, error) {
	var doc struct {
		LibMBINVersion string `json:"libMBIN_version"`
		Mapping        []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"Mapping"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", MappingFile, err)
	}
	if len(doc.Mapping) == 0 {
		return nil, fmt.Errorf("parse %s: no entries", MappingFile)
	}
	m := &Mapping{
		LibMBINVersion: doc.LibMBINVersion,
		Entries:        len(doc.Mapping),
		toName:         make(map[string]string, len(doc.Mapping)),
		toKey:          make(map[string]string, len(doc.Mapping)),
		ambiguous:      map[string]bool{},
	}
	for _, e := range doc.Mapping {
		if e.Key == "" || e.Value == "" {
			continue
		}
		if _, dup := m.toName[e.Key]; !dup {
			m.toName[e.Key] = e.Value
		}
		if prev, dup := m.toKey[e.Value]; !dup {
			m.toKey[e.Value] = e.Key
		} else if prev != e.Key {
			m.ambiguous[e.Value] = true
		}
	}
	return m, nil
}

// LoadMapping reads the asset from disk.
func LoadMapping(path string) (*Mapping, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseMapping(b)
}

// Name is the readable name of a key, or the key itself when unknown.
func (m *Mapping) Name(key string) string {
	if m == nil {
		return key
	}
	if name, ok := m.toName[key]; ok {
		return name
	}
	return key
}

// Key is the obfuscated key for a name.
func (m *Mapping) Key(name string) (string, bool) {
	if m == nil {
		return "", false
	}
	k, ok := m.toKey[name]
	return k, ok
}

// Known says whether a key has a name.
func (m *Mapping) Known(key string) bool {
	if m == nil {
		return false
	}
	_, ok := m.toName[key]
	return ok
}

// Resolve turns a path segment into the key the tree holds: a readable name
// becomes its key, and anything else -- an obfuscated key typed directly, or a
// name the mapping lacks -- is used as written (R3.2).
func (m *Mapping) Resolve(segment string) string {
	if k, ok := m.Key(segment); ok {
		return k
	}
	return segment
}

// Lookup walks a slash-separated path of names or keys, with a number for an
// array element ("BaseContext/PlayerStateData/ShipOwnership/0").
func (m *Mapping) Lookup(root *Node, path string) *Node {
	cur := root
	for _, seg := range SplitPath(path) {
		cur = m.step(cur, seg)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// SplitPath breaks a slash-separated path into its segments, dropping empties.
func SplitPath(path string) []string {
	var out []string
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// step is one segment of a Lookup.
func (m *Mapping) step(cur *Node, seg string) *Node {
	if cur != nil && cur.Type() == TypeArray {
		i, err := strconv.Atoi(seg)
		if err != nil {
			return nil
		}
		return cur.Index(i)
	}
	return cur.Member(m.Resolve(seg))
}

/*
Replace puts a value at a path, in place of what is there (spec 009).

The path has to lead to an existing member or element: this edits a save, it
does not grow one, and a typo in a path must not quietly add a key the game
has never heard of. Returns false when the path leads nowhere.
*/
func (m *Mapping) Replace(root *Node, path string, value *Node) bool {
	segs := SplitPath(path)
	if len(segs) == 0 {
		return false
	}
	parent := root
	for _, seg := range segs[:len(segs)-1] {
		parent = m.step(parent, seg)
		if parent == nil {
			return false
		}
	}
	last := segs[len(segs)-1]
	if parent.Type() == TypeArray {
		i, err := strconv.Atoi(last)
		if err != nil || parent.Index(i) == nil {
			return false
		}
		parent.SetIndex(i, value)
		return true
	}
	key := m.Resolve(last)
	if parent.Member(key) == nil {
		return false
	}
	parent.Set(key, value)
	return true
}

// Unmapped lists the distinct keys in a tree that the mapping does not name,
// sorted (R3.3). With a nil mapping every key is unmapped.
func (m *Mapping) Unmapped(root *Node) []string {
	seen := map[string]bool{}
	root.Walk(func(key string, _ *Node) {
		if key != "" && !m.Known(key) {
			seen[key] = true
		}
	})
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Deobfuscate renames every key the mapping knows to its readable name, for an
// export a person will read (R5.3). It returns how many keys it could not name.
func (m *Mapping) Deobfuscate(root *Node) int {
	_, missed := rename(root, func(key string) (string, bool) {
		if m == nil {
			return "", false
		}
		name, ok := m.toName[key]
		if ok && m.ambiguous[name] {
			return "", false
		}
		return name, ok
	})
	return missed
}

// Obfuscate is the inverse, for importing such an export. Keys that are
// already obfuscated -- or that the mapping never named -- are left as they
// are, so a file exported without names imports unchanged. It returns how many
// keys it turned back.
func (m *Mapping) Obfuscate(root *Node) int {
	renamed, _ := rename(root, func(name string) (string, bool) {
		if m == nil {
			return "", false
		}
		if _, isKey := m.toName[name]; isKey {
			return name, true
		}
		key, ok := m.toKey[name]
		return key, ok
	})
	return renamed
}

func rename(root *Node, fn func(string) (string, bool)) (renamed, missed int) {
	root.Walk(func(_ string, n *Node) {
		for _, mem := range n.Members() {
			if to, ok := fn(mem.Name); ok {
				if to != mem.Name {
					mem.Rename(to)
					renamed++
				}
			} else {
				missed++
			}
		}
	})
	return renamed, missed
}

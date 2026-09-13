/*
Package tweaks holds the built-in mod scripts this project ships (spec 004 R1).

They are ordinary AMUMSS scripts, written for this project and licensed with it,
embedded in the binary rather than copied into the user's library. Two reasons
for embedding. A built-in that lived in the library would be a file the user can
edit and then wonder why "reset to defaults" does not restore it; and the whole
point of a fresh install being able to build something is that there is
something to build without downloading anything first.

Nothing here is third-party. Scripts from Nexus and elsewhere belong to their
authors and stay in the user's library directory, where `mods import` puts them.

The parameter contract is `-- @param` headers plus a top-level `NAME = <number>`
assignment; internal/modscript reads both and rewrites the assignment in memory
when the user has an override. The headers are comments, so every script here is
still a working AMUMSS script.
*/
package tweaks

import (
	"embed"
	"path"
	"sort"
	"strings"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

//go:embed scripts/*.lua
var scripts embed.FS

// Source is how a mod's name maps onto its file. The name is the basename
// without the extension, which is also how a library script is named, so the
// build order can hold both kinds in one list.
const dir = "scripts"

/*
order is the build order a fresh configuration starts with (R1.4).

It is the reference pipeline's order and it is not arbitrary: the two currency
tweaks multiply the same reward blocks, and NaniteRewardBuff has to run after
MoneyAndNanites5x for its multiplier to compound onto the units-and-nanites pass
rather than being overwritten by it. A user can reorder them like any mod; this
is only where they start.
*/
//
//nolint:gochecknoglobals // a fixed list, read-only after initialisation
var order = []string{
	"MaterialYield10x",
	"ChestAndLootMaterials10x",
	"MoneyAndNanites5x",
	"BigStacks",
	"ScanValue50x",
	"SpaceMiningBoost",
	"ItemValueBoost",
	"LearnMoreWords",
	"NaniteRewardBuff",
	"MissionStandingBuff",
	"NexusRewards",
	"MissionBoardRewards",
}

// Groups are the headings the Tweaks section lays the cards out under, in
// display order. A tweak whose header names something else is shown under
// "Other" rather than being hidden.
//
//nolint:gochecknoglobals // a fixed list, read-only after initialisation
var Groups = []string{
	"Mining", "Loot", "Currency", "Standing", "Inventory", "Economy",
	"Exploration", "Language", "Missions",
}

// GroupOther is where a tweak with an unrecognised group lands.
const GroupOther = "Other"

// Tweak is one built-in: its name, what its header says, and the parameters it
// declares with the values the script itself carries.
type Tweak struct {
	Name   string            `json:"name"`
	Header modscript.Header  `json:"header"`
	Params []modscript.Param `json:"params"`
}

// Names lists the built-ins in their default build order (R1.4).
func Names() []string { return append([]string(nil), order...) }

// Has reports whether a name is a built-in. Used to tell a built-in from a
// library script, and to spot a library script shadowing one.
func Has(name string) bool {
	for _, n := range order {
		if n == name {
			return true
		}
	}
	return false
}

// Source returns a built-in's script text.
//
// A copy per call: the caller rewrites parameter assignments in it, and handing
// out the embedded bytes would let one build's overrides leak into the next.
func Source(name string) ([]byte, bool) {
	if !Has(name) {
		return nil, false
	}
	b, err := scripts.ReadFile(path.Join(dir, name+".lua"))
	if err != nil {
		return nil, false
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, true
}

// Get describes one built-in.
func Get(name string) (Tweak, bool) {
	src, ok := Source(name)
	if !ok {
		return Tweak{}, false
	}
	t := Tweak{Name: name, Header: modscript.ParseHeader(src), Params: modscript.Parameters(src)}
	if t.Header.Name == "" {
		t.Header.Name = name
	}
	t.Header.Group = normaliseGroup(t.Header.Group)
	return t, true
}

// All describes every built-in, in default build order.
func All() []Tweak {
	out := make([]Tweak, 0, len(order))
	for _, name := range order {
		if t, ok := Get(name); ok {
			out = append(out, t)
		}
	}
	return out
}

// Files lists the embedded script filenames, for the test that asserts the
// order list and the directory hold the same set. A script that is embedded but
// not listed would ship in the binary and never appear in the interface.
func Files() []string {
	entries, err := scripts.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".lua") {
			out = append(out, strings.TrimSuffix(e.Name(), ".lua"))
		}
	}
	sort.Strings(out)
	return out
}

func normaliseGroup(g string) string {
	for _, known := range Groups {
		if strings.EqualFold(known, g) {
			return known
		}
	}
	return GroupOther
}

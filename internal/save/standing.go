package save

import (
	"fmt"
)

/*
Faction standing (spec 008).

Reputation with the three races and the three guilds is kept in the player
state's Stats: the group whose GroupId is ^GLOBAL_STATS holds one entry per
stat id, each with a Value union whose IntValue is the standing. The game
writes an empty union for a standing of zero, and this code does the same so a
zeroed standing looks the way the game would have written it.

Per-system groups (^SYSTEM_STATS) carry the same ids for the local standing
the game shows in a system; they are left alone. The story gates that a high
standing can lock a player out of read the global value.

The game shows a level, not the value. metadata/gamestate/stats/
leveledstatstable.mbin gives every one of the six stats the same eleven
StatLevels, at values -5, -2, 0, 3, 8, 14, 21, 30, 40, 60 and 100; the two
negative ones are the hostile ranks and the nine from 0 up are the levels 1 to
9 the reputation screen shows. The editor works in those levels: setting level
N writes the lowest value that is level N, which is also the value that sits
just under the gate of level N+1.
*/

// Faction is one reputation the editor can change.
type Faction struct {
	// Key is how the command line names it.
	Key string `json:"key"`
	// Label is how it is shown.
	Label string `json:"label"`
	// Stat is the id under ^GLOBAL_STATS.
	Stat string `json:"stat"`
}

// Factions lists the six, in display order.
//
//nolint:gochecknoglobals // a fixed table
var Factions = []Faction{
	{Key: "gek", Label: "Gek", Stat: "^TRA_STANDING"},
	{Key: "korvax", Label: "Korvax", Stat: "^EXP_STANDING"},
	{Key: "vykeen", Label: "Vy'keen", Stat: "^WAR_STANDING"},
	{Key: "mercenaries", Label: "Mercenaries Guild", Stat: "^WGUILD_STAND"},
	{Key: "explorers", Label: "Explorers Guild", Stat: "^EGUILD_STAND"},
	{Key: "merchants", Label: "Merchants Guild", Stat: "^TGUILD_STAND"},
}

// FactionByKey finds a faction by its command-line key.
func FactionByKey(key string) (Faction, bool) {
	for _, f := range Factions {
		if f.Key == key {
			return f, true
		}
	}
	return Faction{}, false
}

// StandingLevels are the lowest values of levels 1 to 9, from the game's
// leveled stats table (see above). The same for all six factions.
//
//nolint:gochecknoglobals // a fixed table from game data
var StandingLevels = []int64{0, 3, 8, 14, 21, 30, 40, 60, 100}

// MaxLevel is the highest level the game shows.
const MaxLevel = 9

// StandingLevel is the level the game shows for a value: 1 to 9, or 0 for a
// hostile (negative) standing.
func StandingLevel(value int64) int {
	level := 0
	for _, min := range StandingLevels {
		if value >= min {
			level++
		}
	}
	return level
}

// LevelValue is the lowest value that shows as a level.
func LevelValue(level int) (int64, bool) {
	if level < 1 || level > MaxLevel {
		return 0, false
	}
	return StandingLevels[level-1], true
}

// StandingValue is one faction's standing as the save holds it.
type StandingValue struct {
	Faction
	Value int64 `json:"value"`
	// Level is what the game shows for Value.
	Level int `json:"level"`
	// Present is false when the stat entry is missing from the save, which a
	// brand-new save can be.
	Present bool `json:"present"`
}

const (
	nameStats    = "Stats"
	nameGroupID  = "GroupId"
	nameValue    = "Value"
	nameIntValue = "IntValue"
	globalStats  = "^GLOBAL_STATS"
)

// globalStat finds the ^GLOBAL_STATS entry for a stat id, or nil.
func globalStat(ps *Node, m *Mapping, stat string) *Node {
	groups := ps.Member(m.Resolve(nameStats))
	for i := range groups.Len() {
		g := groups.Index(i)
		if id, _ := g.Member(m.Resolve(nameGroupID)).String(); id != globalStats {
			continue
		}
		entries := g.Member(m.Resolve(nameStats))
		for j := range entries.Len() {
			e := entries.Index(j)
			if id, _ := e.Member(m.Resolve(nameID)).String(); id == stat {
				return e
			}
		}
	}
	return nil
}

// Standings reads every faction's global standing.
func Standings(ps *Node, m *Mapping) []StandingValue {
	out := make([]StandingValue, 0, len(Factions))
	for _, f := range Factions {
		sv := StandingValue{Faction: f}
		if e := globalStat(ps, m, f.Stat); e != nil {
			sv.Present = true
			sv.Value, _ = e.Member(m.Resolve(nameValue)).Member(m.Resolve(nameIntValue)).Int()
			sv.Level = StandingLevel(sv.Value)
		}
		out = append(out, sv)
	}
	return out
}

// setStanding writes one faction's global standing as a level (spec 008 R1).
func setStanding(ps *Node, m *Mapping, prefix string, f Faction, level int64) (Change, bool, error) {
	want, ok := LevelValue(int(level))
	if !ok {
		return Change{}, false, fmt.Errorf("%s level %d is outside 1..%d", f.Label, level, MaxLevel)
	}
	e := globalStat(ps, m, f.Stat)
	if e == nil {
		return Change{}, false, fmt.Errorf("the save has no %s entry under %s; the game creates it on first contact", f.Stat, globalStats)
	}
	value := e.Member(m.Resolve(nameValue))
	if value == nil || value.Type() != TypeObject {
		return Change{}, false, fmt.Errorf("%s has no Value union", f.Stat)
	}
	old, _ := value.Member(m.Resolve(nameIntValue)).Int()
	if StandingLevel(old) == int(level) {
		return Change{}, false, nil
	}
	fresh := NewObject()
	if want != 0 {
		fresh.Set(m.Resolve(nameIntValue), NewInt(want))
	}
	e.Set(m.Resolve(nameValue), fresh)
	return Change{
		Field: f.Label + " standing", Path: prefix + nameStats + "/" + globalStats + "/" + f.Stat,
		Old: fmt.Sprintf("level %d (%d)", StandingLevel(old), old), New: fmt.Sprintf("level %d (%d)", level, want),
	}, true, nil
}

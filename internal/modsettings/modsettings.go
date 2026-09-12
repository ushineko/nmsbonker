/*
Package modsettings reads and writes the game's own mod switch file (spec 004 R3.1).

`Binaries/SETTINGS/GCMODSETTINGS.MXML` is written by No Man's Sky, not by this
tool: it lists the loose-file mods the game has seen, whether each is enabled,
and one global `DisableAllMods` that the game sets to true after a crash. A
loose-file mod that has no entry with `Enabled=true` does not load, so deploying
a mod folder and leaving this file alone produces a mod that is installed and
inert.

It is edited line by line rather than through encoding/xml, and that is the
whole design of this package. The file carries a UTF-8 BOM, CRLF line endings
and tab indentation, all of which encoding/xml either rejects outright or
silently reformats; the game reads it back and a reformatted copy is a game
setting file this tool has taken ownership of. Line editing changes the two
values it means to change and leaves every other byte -- including properties
this build has never heard of -- exactly where the game put them.
*/
package modsettings

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Entry is one mod the game knows about.
type Entry struct {
	Name        string
	Enabled     bool
	EnabledVR   bool
	ModPriority int
	// first and last bound the entry's lines in the document.
	first, last int
}

// File is a parsed settings document that can be written back unchanged.
type File struct {
	// lines are the document's lines with their own terminators kept, so a
	// rewrite reproduces the file byte for byte outside the edits.
	lines []line
	// bom records that the document opened with a UTF-8 byte-order mark.
	bom bool
	// eol is the terminator new lines get: whatever the document mostly uses.
	eol string
	// indent is one level of indentation, taken from the document.
	indent string

	disableAll    bool
	disableAllAt  int
	entries       []Entry
	dataOpenAt    int
	dataCloseAt   int
	dataSelfClose bool
}

type line struct {
	text string
	end  string
}

var (
	// ErrNoFile reports that the game has never written a settings file. It is
	// not an error condition: a game that has never loaded a mod does not have
	// one, and the game creates it on first sight of a mod folder.
	ErrNoFile = errors.New("the game has not written GCMODSETTINGS.MXML yet")
	// ErrNoModList reports a settings file whose shape this package does not
	// recognise, so an entry cannot safely be added to it.
	ErrNoModList = errors.New(`GCMODSETTINGS.MXML has no <Property name="Data"> list`)
)

//nolint:gochecknoglobals // compiled once, read-only
var (
	propRE      = regexp.MustCompile(`name="([^"]*)"(?:\s+value="([^"]*)")?`)
	valueRE     = regexp.MustCompile(`(value=")([^"]*)(")`)
	indentRE    = regexp.MustCompile(`^[ \t]*`)
	dataOpenRE  = regexp.MustCompile(`<Property\s+name="Data"\s*>`)
	dataSelfRE  = regexp.MustCompile(`<Property\s+name="Data"\s*/>`)
	closePropRE = regexp.MustCompile(`^\s*</Property>\s*$`)
)

// utf8BOM is the byte-order mark the game writes at the head of the file. It is
// spelled in bytes rather than as a literal because a BOM inside a Go source
// file is a compile error.
const utf8BOM = "\xef\xbb\xbf"

// Read parses the settings file at path.
func Read(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoFile
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(b), nil
}

// Parse reads a settings document out of memory.
func Parse(b []byte) *File {
	text := string(b)
	f := &File{eol: "\n", indent: "\t", disableAllAt: -1, dataOpenAt: -1, dataCloseAt: -1}
	if strings.HasPrefix(text, utf8BOM) {
		f.bom = true
		text = strings.TrimPrefix(text, utf8BOM)
	}
	if strings.Count(text, "\r\n") > strings.Count(text, "\n")/2 {
		f.eol = "\r\n"
	}
	f.lines = splitLines(text)
	f.scan()
	return f
}

// splitLines keeps each line's own terminator so the document can be rebuilt
// byte for byte.
func splitLines(text string) []line {
	var out []line
	for len(text) > 0 {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			out = append(out, line{text: text})
			break
		}
		body, end := text[:i], "\n"
		if strings.HasSuffix(body, "\r") {
			body, end = body[:len(body)-1], "\r\n"
		}
		out = append(out, line{text: body, end: end})
		text = text[i+1:]
	}
	return out
}

/*
scan locates the two things this package can change and the mod list they live
in, without building a model of the document.

Everything it does not recognise stays where it is. That is the point: the game
writes properties this build has never heard of, and a round trip through a
parser that only knows about four of them is how a settings file loses the rest.
*/
func (f *File) scan() {
	var current *Entry
	for i, l := range f.lines {
		text := l.text
		switch {
		case dataSelfRE.MatchString(text) && !strings.Contains(text, "GcModSettingsInfo"):
			f.dataOpenAt, f.dataSelfClose = i, true
		case dataOpenRE.MatchString(text) && !strings.Contains(text, "GcModSettingsInfo"):
			f.dataOpenAt = i
			if ind := indentRE.FindString(text); ind != "" {
				f.indent = ind
			}
		}

		m := propRE.FindStringSubmatch(text)
		if m == nil {
			if current != nil && closePropRE.MatchString(text) {
				current.last = i
				f.entries = append(f.entries, *current)
				current = nil
			}
			continue
		}
		name, value := m[1], m[2]
		if value == "GcModSettingsInfo" {
			if current != nil {
				current.last = i - 1
				f.entries = append(f.entries, *current)
			}
			current = &Entry{first: i}
			continue
		}
		switch name {
		case "DisableAllMods":
			f.disableAll = strings.EqualFold(value, "true")
			f.disableAllAt = i
		case "Name":
			if current != nil {
				current.Name = value
			}
		case "Enabled":
			if current != nil {
				current.Enabled = strings.EqualFold(value, "true")
			}
		case "EnabledVR":
			if current != nil {
				current.EnabledVR = strings.EqualFold(value, "true")
			}
		case "ModPriority":
			if current != nil {
				if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
					current.ModPriority = n
				}
			}
		}
	}
	if current != nil {
		current.last = len(f.lines) - 1
		f.entries = append(f.entries, *current)
	}
	f.findDataClose()
}

// findDataClose locates the </Property> that ends the mod list, which is where
// a new entry is inserted.
func (f *File) findDataClose() {
	if f.dataOpenAt < 0 || f.dataSelfClose {
		return
	}
	open := len(indentRE.FindString(f.lines[f.dataOpenAt].text))
	for i := f.dataOpenAt + 1; i < len(f.lines); i++ {
		if !closePropRE.MatchString(f.lines[i].text) {
			continue
		}
		if len(indentRE.FindString(f.lines[i].text)) == open {
			f.dataCloseAt = i
			return
		}
	}
}

// DisableAllMods is the game's own kill switch (R3.3).
func (f *File) DisableAllMods() bool { return f.disableAll }

// Entries lists the mods the game knows about, in document order.
func (f *File) Entries() []Entry { return append([]Entry(nil), f.entries...) }

// Find looks an entry up by name, case-insensitively: the game writes the mod
// folder's name and a folder called "cosmos combine" is the same mod.
func (f *File) Find(name string) (Entry, bool) {
	for _, e := range f.entries {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return Entry{}, false
}

// SetDisableAllMods flips the kill switch, reporting whether anything changed.
func (f *File) SetDisableAllMods(off bool) bool {
	if f.disableAll == off {
		return false
	}
	if f.disableAllAt < 0 {
		return false
	}
	f.setValue(f.disableAllAt, boolText(off))
	f.disableAll = off
	return true
}

/*
EnableMod makes sure one mod is present and switched on (R3.2).

Enabled and EnabledVR both, because the game keeps them separately and a mod
that is on in flat mode and off in VR is a support question rather than a
feature. Every other property of the entry -- ID, AuthorID, LastUpdated,
ModPriority, Dependencies -- is left exactly as the game wrote it.

Reports whether the document changed and whether the entry had to be created.
*/
func (f *File) EnableMod(name string) (changed, added bool, err error) {
	for idx, e := range f.entries {
		if !strings.EqualFold(e.Name, name) {
			continue
		}
		for i := e.first; i <= e.last && i < len(f.lines); i++ {
			m := propRE.FindStringSubmatch(f.lines[i].text)
			if m == nil {
				continue
			}
			switch m[1] {
			case "Enabled", "EnabledVR":
				if !strings.EqualFold(m[2], "true") {
					f.setValue(i, "true")
					changed = true
				}
			}
		}
		f.entries[idx].Enabled, f.entries[idx].EnabledVR = true, true
		return changed, false, nil
	}
	if err := f.addEntry(name); err != nil {
		return false, false, err
	}
	return true, true, nil
}

/*
addEntry writes a new GcModSettingsInfo block into the mod list.

The shape is the game's own, field for field and in the game's order, because
the game parses this file with a template and a block missing a property it
expects is a block it may discard. `_index` continues the existing numbering.
*/
func (f *File) addEntry(name string) error {
	if f.dataOpenAt < 0 {
		return ErrNoModList
	}
	base := indentRE.FindString(f.lines[f.dataOpenAt].text)
	in1 := base + f.indent
	in2 := in1 + f.indent

	block := []line{
		{text: fmt.Sprintf(`%s<Property name="Data" value="GcModSettingsInfo" _index="%d">`,
			in1, len(f.entries)), end: f.eol},
		{text: fmt.Sprintf(`%s<Property name="Name" value="%s" />`, in2, escape(name)), end: f.eol},
		{text: in2 + `<Property name="Author" value="" />`, end: f.eol},
		{text: in2 + `<Property name="ID" value="0" />`, end: f.eol},
		{text: in2 + `<Property name="AuthorID" value="0" />`, end: f.eol},
		{text: in2 + `<Property name="LastUpdated" value="0" />`, end: f.eol},
		{text: in2 + `<Property name="ModPriority" value="0" />`, end: f.eol},
		{text: in2 + `<Property name="Enabled" value="true" />`, end: f.eol},
		{text: in2 + `<Property name="EnabledVR" value="true" />`, end: f.eol},
		{text: in2 + `<Property name="Dependencies" />`, end: f.eol},
		{text: in1 + `</Property>`, end: f.eol},
	}

	at := f.dataCloseAt
	if f.dataSelfClose {
		// The game writes `<Property name="Data" />` while it knows about no
		// mods at all. It has to become a list before anything can go in it.
		f.lines[f.dataOpenAt] = line{text: base + `<Property name="Data">`, end: f.eol}
		block = append(block, line{text: base + `</Property>`, end: f.eol})
		at = f.dataOpenAt + 1
		f.dataSelfClose = false
	}
	if at < 0 {
		return ErrNoModList
	}

	rest := append([]line(nil), f.lines[at:]...)
	f.lines = append(f.lines[:at], append(block, rest...)...)
	f.entries = append(f.entries, Entry{
		Name: name, Enabled: true, EnabledVR: true, first: at, last: at + len(block) - 1,
	})
	f.scanAgain()
	return nil
}

// scanAgain re-derives the line positions after an insertion.
func (f *File) scanAgain() {
	kept := f.lines
	bom, eol, indent := f.bom, f.eol, f.indent
	*f = File{lines: kept, bom: bom, eol: eol, indent: indent,
		disableAllAt: -1, dataOpenAt: -1, dataCloseAt: -1}
	f.scan()
}

// setValue rewrites the value= attribute of one line, leaving its indentation,
// its other attributes and its terminator alone.
func (f *File) setValue(i int, value string) {
	l := f.lines[i]
	l.text = valueRE.ReplaceAllString(l.text, "${1}"+escape(value)+"${3}")
	f.lines[i] = l
}

// Bytes renders the document.
func (f *File) Bytes() []byte {
	var b strings.Builder
	if f.bom {
		b.WriteString(utf8BOM)
	}
	for _, l := range f.lines {
		b.WriteString(l.text)
		b.WriteString(l.end)
	}
	return []byte(b.String())
}

// Write replaces the file at path with this document, atomically: a half-written
// settings file is a game that will not start its mod list.
func (f *File) Write(path string) error {
	tmp := path + ".nmsbonker-tmp"
	// 0644: the game reads this file, and it was already world-readable.
	if err := os.WriteFile(tmp, f.Bytes(), 0o644); err != nil { //nolint:gosec // read by the game
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// escape makes a value safe inside a double-quoted XML attribute. Mod folder
// names come from the user's settings and an ampersand in one must not produce
// a settings file the game cannot parse.
func escape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
	).Replace(s)
}

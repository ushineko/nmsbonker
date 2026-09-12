package steam

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ModSetting is one entry in GCMODSETTINGS.MXML (R3.5).
type ModSetting struct {
	Name        string
	Enabled     bool
	ModPriority int
}

// ModSettings is what the game's mod settings file says (R3.5).
type ModSettings struct {
	DisableAllMods bool
	Mods           []ModSetting
}

/*
The file is MXML, but this reads it with a regex rather than encoding/xml.

Two reasons. It is written by the game and by AMUMSS-era tools, and has been
seen with a UTF-8 BOM, CRLF endings, and no trailing newline -- encoding/xml
rejects the BOM outright. And this spec only reads it; spec 004 writes it, and
writing it must preserve whatever the game wrote byte-for-byte outside the
fields being changed, which an XML round-trip does not do. Using the same
line-oriented view for both keeps read and write agreeing about the document.
*/
var propertyRE = regexp.MustCompile(`name="([^"]*)"(?:\s+value="([^"]*)")?`)

// ReadModSettings parses the game's mod settings file.
//
// A missing file is not an error: a game that has never loaded a mod does not
// have one, and every caller here treats "no settings" as "no mods configured".
func ReadModSettings(path string) (*ModSettings, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ModSettings{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseModSettings(string(b)), nil
}

// ParseModSettings reads the settings out of an MXML document.
func ParseModSettings(text string) *ModSettings {
	out := &ModSettings{}
	var current *ModSetting
	flush := func() {
		if current != nil {
			out.Mods = append(out.Mods, *current)
			current = nil
		}
	}

	for _, line := range strings.Split(text, "\n") {
		m := propertyRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, value := m[1], m[2]
		// Each mod is introduced by a Property whose value names the struct.
		if value == "GcModSettingsInfo" {
			flush()
			current = &ModSetting{}
			continue
		}
		switch name {
		case "DisableAllMods":
			out.DisableAllMods = strings.EqualFold(value, "true")
		case "Name":
			if current != nil {
				current.Name = value
			}
		case "Enabled":
			if current != nil {
				current.Enabled = strings.EqualFold(value, "true")
			}
		case "ModPriority":
			if current != nil {
				if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
					current.ModPriority = n
				}
			}
		}
	}
	flush()
	return out
}

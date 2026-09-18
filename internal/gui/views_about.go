package gui

import (
	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
)

// --- About (R2.8) ----------------------------------------------------------

// projectURL is where the README lives. It is the one place this window sends a
// user outside itself, and it opens in the desktop's browser rather than in any
// view of ours.
const projectURL = "https://github.com/ushineko/nmsbonker"

/*
buildAbout is what nmsbonker is and what it will and will not do, in the
library's About shape.

The blocks answer what someone asks before letting a tool near a game they have
four hundred hours in: what it builds, what it touches, where it puts things.
Limitations stay in the README; a shortened copy here would only drift.
*/
func (u *ui) buildAbout() fyne.CanvasObject {
	return shell.AboutSection(u.about()).Build(u.sh)
}

// about describes this program for the About section.
func (u *ui) about() shell.About {
	return shell.About{
		Icon:    appIcon(),
		Name:    "nmsbonker",
		Version: u.version + " (" + u.commit + ")",
		Blurb: "Builds AMUMSS-format .lua mods against the game files you have installed, merges " +
			"them into one mod folder, and deploys it. Reads and edits your saves. Native Go " +
			"on a Steam/Proton install: no Wine, no Windows VM, no Python.",
		URL:     projectURL,
		URLText: "Project documentation",
		Notes: []shell.Note{
			{Title: "Build", Detail: "Every enabled script edits the same pristine game files, in your order. Two mods " +
				"touching one file yield one merged file, not two that fight."},
			{Title: "Ship only what compiles", Detail: "A merged file ships only if MBINCompiler recompiles it cleanly. A rejected edit is " +
				"retried without its structural changes, then dropped and named in the report."},
			{Title: "Touch little", Detail: "Deploy writes one folder under GAMEDATA/MODS and archives what it replaces. The " +
				"save editor writes one save and its manifest, after copying the whole profile " +
				"to the backup directory. Nothing else in the game is written; the .pak archives " +
				"never are."},
			{Title: "Keep files where you expect", Detail: "Scripts in the library, compilers in tools, extracted game files in the cache, " +
				"merged output in the workspace, replaced deployments in the archive, save " +
				"copies in save-backup. All under your XDG directories; all listed in Settings."},
			{Title: "Survive a game update", Detail: "After an update, the build report is your re-download list: a mod whose keys the " +
				"update renamed comes out WORKING~ or NOT BUILT, with the keys it could not find."},
		},
		Facts: []shell.Fact{
			{Label: "Compiler", Value: compilerFact(u)},
			{Label: "Game buildid", Value: gameFact(u)},
			{Label: "Settings file", Value: configFact(u)},
			{Label: "Licence", Value: "MIT"},
		},
	}
}

func compilerFact(u *ui) string {
	if !u.statusOK || !u.status.Compiler.Installed {
		return "none installed"
	}
	return u.status.Compiler.Tag + " (" + u.status.Compiler.Flavor + ")"
}

func gameFact(u *ui) string {
	if !u.statusOK || !u.status.Install.Found {
		return "no game found"
	}
	return widgets.OrNone(u.status.Install.BuildID, "unknown")
}

func configFact(u *ui) string {
	if u.status.ConfigPath != "" {
		return u.status.ConfigPath
	}
	return "not read yet"
}

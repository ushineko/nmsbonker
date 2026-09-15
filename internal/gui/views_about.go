// Copied from angou (same author) — keep in sync by hand; the text is this
// project's.

package gui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// --- About (R2.8) ----------------------------------------------------------

// projectURL is where the README lives. It is the one place this window sends a
// user outside itself, and it opens in the desktop's browser rather than in any
// view of ours.
const projectURL = "https://github.com/ushineko/nmsbonker"

/*
buildAbout is what nmsbonker is and what it will and will not do.

The blocks answer what someone asks before letting a tool near a game they have
four hundred hours in: what it builds, what it touches, where it puts things.
Limitations stay in the README; a shortened copy here would only drift.
*/
func (u *ui) buildAbout(version, commit string) fyne.CanvasObject {
	logo := canvas.NewImageFromResource(appIcon())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(72, 72))

	name := widget.NewLabelWithStyle("nmsbonker", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ver := widget.NewLabel(version + " (" + commit + ")")
	ver.Importance = widget.LowImportance
	blurb := widget.NewLabel(
		"Builds AMUMSS-format .lua mods against the game files you have installed, merges " +
			"them into one mod folder, and deploys it. Reads and edits your saves. Native Go " +
			"on a Steam/Proton install: no Wine, no Windows VM, no Python.")
	blurb.Wrapping = fyne.TextWrapWord

	head := container.NewBorder(nil, nil, container.NewPadded(logo), nil,
		container.NewVBox(name, ver, blurb))

	can := container.NewVBox(
		aboutNote("Build",
			"Every enabled script edits the same pristine game files, in your order. Two mods "+
				"touching one file yield one merged file, not two that fight."),
		aboutNote("Ship only what compiles",
			"A merged file ships only if MBINCompiler recompiles it cleanly. A rejected edit is "+
				"retried without its structural changes, then dropped and named in the report."),
		aboutNote("Touch little",
			"Deploy writes one folder under GAMEDATA/MODS and archives what it replaces. The "+
				"save editor writes one save and its manifest, after copying the whole profile "+
				"to the backup directory. Nothing else in the game is written; the .pak archives "+
				"never are."),
		aboutNote("Keep files where you expect",
			"Scripts in the library, compilers in tools, extracted game files in the cache, "+
				"merged output in the workspace, replaced deployments in the archive, save "+
				"copies in save-backup. All under your XDG directories; all listed in Settings."),
		aboutNote("Survive a game update",
			"After an update, the build report is your re-download list: a mod whose keys the "+
				"update renamed comes out WORKING~ or NOT BUILT, with the keys it could not find."),
	)

	facts := widget.NewForm(
		widget.NewFormItem("Compiler", widget.NewLabel(compilerFact(u))),
		widget.NewFormItem("Game buildid", widget.NewLabel(gameFact(u))),
		widget.NewFormItem("Settings file", widget.NewLabel(configFact(u))),
		widget.NewFormItem("Licence", widget.NewLabel("MIT")),
	)

	body := container.NewVBox(
		head,
		aboutLink(),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("What it does", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		can,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Facts", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		facts,
	)
	return container.NewVScroll(body)
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
	return orNone(u.status.Install.BuildID, "unknown")
}

func configFact(u *ui) string {
	if u.status.ConfigPath != "" {
		return u.status.ConfigPath
	}
	return "not read yet"
}

// aboutLink points at the README, which carries what this window deliberately
// does not: installation, the build pipeline in detail, and the account of
// where nmsbonker promises less than a reader might assume.
func aboutLink() fyne.CanvasObject {
	link, err := url.Parse(projectURL)
	if err != nil {
		// Unreachable for a constant that parses, but a window that panics on a
		// bad link is worse than one that shows the address as text.
		return widget.NewLabel(projectURL)
	}
	return widget.NewHyperlink("Project documentation", link)
}

func aboutNote(title, detail string) fyne.CanvasObject {
	t := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	d := widget.NewLabel(detail)
	d.Wrapping = fyne.TextWrapWord
	d.Importance = widget.LowImportance
	return container.NewVBox(t, d)
}

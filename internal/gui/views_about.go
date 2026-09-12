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

The three capability blocks are chosen to answer the questions someone asks
before letting a tool near a game they have four hundred hours in: what does it
build, what does it touch, and where does it put things. Limitations belong in
the README, which carries them at length; restating a shortened version here
would only produce a second, less careful copy to keep in sync.
*/
func (u *ui) buildAbout(version, commit string) fyne.CanvasObject {
	logo := canvas.NewImageFromResource(appIcon())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(72, 72))

	name := widget.NewLabelWithStyle("nmsbonker", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ver := widget.NewLabel(version + " (" + commit + ")")
	ver.Importance = widget.LowImportance
	blurb := widget.NewLabel(
		"nmsbonker rebuilds AMUMSS-format .lua mod scripts against the No Man's Sky files you " +
			"actually have installed, merges every enabled mod into one collision-free mod " +
			"folder, and deploys it. Native Go against a Steam/Proton install: no Wine, no " +
			"Windows VM, no Python.")
	blurb.Wrapping = fyne.TextWrapWord

	head := container.NewBorder(nil, nil, container.NewPadded(logo), nil,
		container.NewVBox(name, ver, blurb))

	can := container.NewVBox(
		aboutNote("What it builds",
			"One mod folder. Every enabled script's edits are applied to the same pristine game "+
				"files in build order, so two mods editing one file produce one merged file "+
				"rather than two that overwrite each other."),
		aboutNote("What it ships, and what it refuses to",
			"A merged file is shipped only if MBINCompiler recompiles it cleanly. A structural "+
				"edit the compiler rejects is retried without it, and dropped if that fails — "+
				"and the report names the file and the mod. Nothing the compiler rejected ever "+
				"reaches the game."),
		aboutNote("What it never touches",
			"The game directory is read-only except during a deploy, which writes one folder "+
				"under GAMEDATA/MODS and archives whatever it replaces first. Your saves are "+
				"never read or written. The game's own .pak archives are never modified. A "+
				"symlinked GAMEDATA/MODS is left alone unless you ask, and then only the link "+
				"goes — never what it points at."),
		aboutNote("Where files live",
			"Scripts in the library directory, downloaded compilers in the tools directory, "+
				"extracted game files in the cache, merged output in the workspace, and "+
				"replaced deployments in the archive — all under your XDG directories, all "+
				"shown in Settings."),
		aboutNote("What it says about a game update",
			"After an update, a build's report is the list of mods to re-download: a mod whose "+
				"keys the update renamed comes out WORKING~ or NOT BUILT, named, with the keys "+
				"it could not find."),
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

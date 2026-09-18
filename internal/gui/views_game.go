package gui

import (
	"context"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/steam"
)

/*
The operations that reach into the game (spec 004 R3.3, R4, R5).

Everything here changes something outside this program's own directories, so
every one of them is behind a confirmation that says what it touches and, as
importantly, what it does not. The game directory is read-only to this tool
except for these, for deploy, and for the save editor (views_saves.go).
*/

// --- save backup (R5.2) -----------------------------------------------------

// loadSaves fills the save-backup listing the Overview reports and the dialog
// lists.
func (u *ui) loadSaves() {
	if u.savesOK {
		return
	}
	u.savesOK = true
	u.load("Reading the save backups…", func() {
		res, err := core.ListSaveBackups(context.Background(),
			core.ListSaveBackupsRequest{Request: u.request()})
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the save backups", err) })
			return
		}
		fyne.Do(func() {
			u.saves = res
			u.sh.Refresh()
		})
	})
}

// saveBackupText is the Overview's one line about the saves: when the last copy
// was taken, and whether a deploy will take one.
func (u *ui) saveBackupText() string {
	if !u.savesOK {
		return "reading…"
	}
	when := "never"
	if len(u.saves.Backups) > 0 {
		b := u.saves.Backups[0]
		when = widgets.HumanAgo(b.Created) + " (" + b.Created.Format("2006-01-02 15:04") + ")"
	}
	before := "a deploy takes one first"
	if !u.saves.Enabled {
		before = "a deploy does not take one (save_backup is off)"
	}
	return when + " · " + before
}

/*
backupSaves copies the game's saves out of the Proton prefix.

No confirmation: it reads the prefix and writes into this tool's own directory,
and there is no state it can damage. The banner says where the copy went,
because the only thing a user can do with a backup is find it again.
*/
func (u *ui) backupSaves() {
	u.sh.Perform("Backing up the game's saves…", func(ctx context.Context) error {
		res, err := core.BackupSaves(ctx, core.BackupSavesRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			u.savesOK = false
			if res.Skipped != "" {
				u.sh.Flash("No saves were copied: "+res.Skipped, fd.StatusWarn)
				u.sh.Invalidate()
				return
			}
			msg := fmt.Sprintf("Copied %d save profile(s), %d file(s), to %s.",
				res.Profiles, res.Files, res.Dir)
			if len(res.Pruned) > 0 {
				msg += fmt.Sprintf(" %d older backup(s) were deleted to keep the newest %d.",
					len(res.Pruned), core.SaveRetention)
			}
			u.sh.Flash(msg, fd.StatusGood)
			u.sh.Invalidate()
		})
		return nil
	})
}

// --- the game's mod directory -----------------------------------------------

/*
openModsDir hands GAMEDATA/MODS to the desktop.

The one place a deploy writes into, and the one directory a user goes looking
for by hand when the game does not load a mod — which is why it is reachable
from Overview, from the game strip and from Report rather than only from the
row that reports its state.

A symlinked MODS is opened through the link, deliberately: the link is what the
game reads, so it is also what a person checking the game's mods wants to see.
The row above the button still says it is a symlink and where it points.
*/
func (u *ui) openModsDir() {
	u.openPath(u.status.Install.ModsDir)
}

// modsDirOpenable says whether there is a directory there to open. Absent is
// the ordinary state of a game that has never had a mod installed, and asking
// the desktop to open a path that does not exist gets a file manager's own
// error rather than this window's.
func modsDirOpenable(in core.InstallSummary) bool {
	if !in.Found || in.ModsDir == "" {
		return false
	}
	return in.ModsState == steam.ModsDir || in.ModsState == steam.ModsSymlink
}

// --- the game's own mod switch (R3.3) ---------------------------------------

/*
toggleAllMods flips DisableAllMods in the game's settings.

Turning mods off is the first thing to try when a game stops starting, and it is
worth having in front of someone at that moment: it changes nothing that has to
be rebuilt, and turning it back on restores exactly what was loading before.
*/
func (u *ui) toggleAllMods(off bool) {
	verb, past := "Enabling mods", "Mods are enabled again"
	if off {
		verb, past = "Disabling all mods", "Every mod is switched off"
	}
	u.sh.Perform(verb+"…", func(ctx context.Context) error {
		res, err := core.ModsToggle(ctx, core.ModsToggleRequest{
			Request: u.request(), DisableAll: off,
		})
		if err != nil {
			return err
		}
		if !res.Changed {
			fyne.Do(func() {
				u.sh.Flash("The game's switch was already set that way; nothing was written.",
					fd.StatusInfo)
			})
			return nil
		}
		fyne.Do(func() {
			u.sh.OK(past + " in the game's own settings. Nothing was removed: every mod folder is " +
				"still installed and every per-mod switch keeps its state.")
		})
		return nil
	})
}

// confirmDisableAllMods says what the switch does before flipping it.
func (u *ui) confirmDisableAllMods() {
	dialogs.ConfirmDestructive(u.sh.Window, "Stop the game loading any mod?",
		"This sets DisableAllMods in the game's own GCMODSETTINGS.MXML and changes nothing "+
			"else. Every mod folder stays where it is, every per-mod switch keeps its state, "+
			"and Enable mods puts it back exactly as it was. It is the cheapest thing to try "+
			"when the game stops starting, because nothing has to be rebuilt afterwards.",
		"Disable all mods", func() { u.toggleAllMods(true) })
}

// --- deploying into a symlinked MODS directory (R6.2) -----------------------

/*
replaceSymlinkAndDeploy turns a symlinked GAMEDATA/MODS into a real directory
and installs the last build into it.

The dialog states the three facts the CLI's refusal states, in the same order:
what the link is, that removing it removes the link and never its target, and
that a real directory takes its place. It names no cause -- a symlink there
could have been put there by anything, and guessing would make the message wrong
for everyone whose reason was different.
*/
func (u *ui) replaceSymlinkAndDeploy() {
	in := u.status.Install
	body := container.NewVBox(
		widgets.Wrapped("GAMEDATA/MODS is a symlink pointing at "+in.ModsTarget+"."),
		widgets.Wrapped("The game reads mods through the link, so installing here would write into "+
			"that directory rather than into the game."),
		widgets.Wrapped("Replacing it removes the link, and only the link: "+in.ModsTarget+" and "+
			"everything in it is left exactly as it is. A real directory is created in its "+
			"place, and the build in the workspace is installed into it."),
		widgets.Wrapped("This is reversible by hand — the link can be re-made with ln -sfn — and "+
			"Steam's \"verify integrity of game files\" restores the stock layout."),
	)
	dialogs.ConfirmWithBody(u.sh.Window, "Replace the symlink and deploy?", body, "Replace and deploy", func() {
		u.sh.Perform("Replacing the symlink and installing…", func(ctx context.Context) error {
			res, err := core.Deploy(ctx, core.DeployRequest{
				Request: u.request(), ReplaceSymlink: true,
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Installed %d file(s) to %s. The symlink to %s was removed; "+
				"its target was left alone.", res.Files, res.Dest, res.ReplacedSymlink)
			u.deployed(res, msg)
			return nil
		})
	}).Show()
}

// deployed reports a finished deploy, warnings included, and reloads.
func (u *ui) deployed(res core.DeployResult, msg string) {
	if res.SettingsAdded {
		msg += " The game had no entry for this mod, so one was added and switched on."
	}
	if len(res.Warnings) > 0 {
		fyne.Do(func() {
			u.savesOK = false
			u.sh.Flash(msg+" "+res.Warnings[0], fd.StatusWarn)
			u.sh.Invalidate()
		})
		return
	}
	fyne.Do(func() { u.savesOK = false })
	fyne.Do(func() { u.sh.OK(msg) })
}

// --- taking it back out (R4.2, R4.3) ----------------------------------------

// confirmUndeploy removes the installed mod folder, keeping a copy.
func (u *ui) confirmUndeploy() {
	dest := u.status.Install.ModsDir + "/" + widgets.OrNone(u.status.ModName, "COSMOS COMBINE")
	dialogs.ConfirmDestructive(u.sh.Window, "Remove the deployed mod?",
		"The folder at "+dest+" is moved into "+u.status.Paths.Archive+" under a timestamp, "+
			"so it can be rolled back. The game's own mod settings are left alone, and so is "+
			"the build in the workspace — this removes what is installed, not what would be "+
			"installed next. Your saves and every other mod folder are untouched.",
		"Remove", func() {
			u.sh.Perform("Removing the deployed mod…", func(ctx context.Context) error {
				res, err := core.Undeploy(ctx, core.UndeployRequest{Request: u.request()})
				if err != nil {
					return err
				}
				fyne.Do(func() {
					u.sh.OK(fmt.Sprintf("Removed %s. Its %d file(s) are in %s and `Roll back…` "+
						"puts them back.", res.Dest, res.Files, res.Archived))
				})
				return nil
			})
		})
}

// loadArchive fills the rollback listing.
func (u *ui) loadArchive() {
	if u.archiveOK {
		return
	}
	u.archiveOK = true
	u.load("Reading the archive…", func() {
		res, err := core.ListArchive(context.Background(),
			core.ListArchiveRequest{Request: u.request()})
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the archive", err) })
			return
		}
		fyne.Do(func() {
			u.archive = res
			u.sh.Refresh()
		})
	})
}

/*
showRollback lists the archive and rolls one entry back.

The entry has to be chosen rather than assumed: "roll back" usually means the
last one, but the case this exists for is a game update three deploys ago, and a
button that can only undo one step is a button that cannot reach it.
*/
func (u *ui) showRollback() {
	if !u.archiveOK || len(u.archive.Entries) == 0 {
		u.sh.Flash("Nothing has been deployed yet, so there is nothing to roll back to.",
			fd.StatusInfo)
		return
	}
	selected := 0

	t := table.New()
	t.Header("Taken", "Mod", "Holds", "Files", "Size")
	t.SetWidths(220, 220, 260, 80, 110)
	for _, e := range u.archive.Entries {
		st := fd.StatusInfo
		if e.HasMod {
			st = fd.StatusGood
		}
		t.Row(st, e.Created.Format("2006-01-02 15:04")+" · "+widgets.HumanAgo(e.Created),
			e.ModName, archiveHolds(e), strconv.Itoa(e.Files), widgets.HumanSize(e.Bytes))
	}
	table := t.Widget()
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(u.archive.Entries) {
			selected = id.Row
		}
	}

	head := container.NewVBox(
		widgets.PlainRow("Archive", u.archive.Dir),
		widgets.PlainRow("Kept", fmt.Sprintf("the newest %d", u.archive.Retention)),
		widgets.Wrapped("Rolling back swaps what is installed in the game for the entry you pick. "+
			"What is installed now becomes a new archive entry, so a rollback can itself be "+
			"rolled back, and the build in the workspace is not touched — this changes what "+
			"is in the game, not what the next deploy would install."),
	)
	body := container.NewBorder(head, nil, nil, nil, widgets.FixedHeight(table, 240))

	d := dialogs.ConfirmWithBody(u.sh.Window, "Roll back to an earlier deployment?", body, "Roll back", func() {
		entry := u.archive.Entries[selected]
		u.rollback(entry)
	})
	d.Show()
}

// archiveHolds says what an entry can restore, in the words the CLI uses.
func archiveHolds(e core.ArchiveEntry) string {
	switch {
	case e.HasMod && e.HasSettings:
		return "mod folder + mod settings"
	case e.HasMod:
		return "mod folder"
	case e.HasSettings:
		return "mod settings (nothing was installed)"
	}
	return "nothing"
}

// rollback restores one archive entry.
func (u *ui) rollback(entry core.ArchiveEntry) {
	u.sh.Perform("Rolling back to "+entry.Timestamp+"…", func(ctx context.Context) error {
		res, err := core.Rollback(ctx, core.RollbackRequest{
			Request: u.request(), Timestamp: entry.Timestamp,
		})
		if err != nil {
			return err
		}
		st := fd.StatusGood
		msg := fmt.Sprintf("Restored the deployment from %s: %d file(s) in %s.",
			entry.Created.Format("2006-01-02 15:04"), res.Files, res.Dest)
		if res.Removed {
			st = fd.StatusWarn
			msg = "That entry recorded the state before anything was installed, so " +
				res.Dest + " has been removed."
		}
		if res.SettingsRestored != "" {
			msg += " The game's mod settings were restored as well."
		}
		msg += " What was installed is now in " + res.Archived + "."
		fyne.Do(func() {
			u.archiveOK = false
			u.sh.Flash(msg, st)
			u.sh.Invalidate()
		})
		return nil
	})
}

// --- the Overview's game strip ----------------------------------------------

/*
gameActions is the second row of Overview buttons: the things that change what
the game loads, as opposed to the things that build.

Separated from Build and Deploy on purpose. The top row is the ordinary loop and
the bottom row is what you reach for when something is wrong, and mixing them
puts "Remove deployed mod" next to the button people press every day.
*/
func (u *ui) gameActions() fyne.CanvasObject {
	in := u.status.Install

	var switchMods *widget.Button
	if in.DisableAllMods {
		switchMods = widget.NewButtonWithIcon("Enable mods", theme.ConfirmIcon(),
			func() { u.toggleAllMods(false) })
	} else {
		switchMods = widget.NewButtonWithIcon("Disable all mods", theme.CancelIcon(),
			func() { u.confirmDisableAllMods() })
	}

	saves := widget.NewButtonWithIcon("Saves…", theme.StorageIcon(),
		func() { u.sh.Select("Saves") })
	openMods := widget.NewButtonWithIcon("Open mods folder", theme.FolderIcon(),
		func() { u.openModsDir() })
	remove := widget.NewButtonWithIcon("Remove deployed mod…", theme.DeleteIcon(),
		func() { u.confirmUndeploy() })
	remove.Importance = widget.DangerImportance

	u.sh.Gate(switchMods, saves, openMods, remove)
	if !in.Found {
		switchMods.Disable()
		saves.Disable()
		remove.Disable()
	}
	if !modsDirOpenable(in) {
		openMods.Disable()
	}
	if !in.ModSettingsOK {
		switchMods.Disable()
	}
	if in.ModsState != steam.ModsDir {
		// Nothing can be installed under a symlink or an absent directory, so
		// there is nothing to remove. Disabled rather than hidden: the strip
		// keeps its shape once the state is fixed.
		remove.Disable()
	}
	return container.NewHBox(switchMods, saves, openMods, remove)
}

// Copied from angou (same author) — keep in sync by hand. confirmDestructive,
// pathDialog, fixedHeight/fixedWidth, pickerStart and browseButton/withBrowse
// are angou's, rationale comments included; openPath is this project's.

package gui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/config"
)

// confirmDestructive is a confirmation that names what is about to happen in at
// least the detail the CLI gives, with the destructive button styled as
// destructive and the cancel as the safe default.
//
// The detail says what is *not* touched as well as what is (R6). Every
// destructive path in this window leaves something alone that a user might
// reasonably fear for — the game's own files, a symlink's target, the .lua in
// the library — and not saying so is how a confirmation dialog becomes the
// thing people click through without reading.
func (u *ui) confirmDestructive(title, detail, confirm string, do func()) {
	body := widget.NewLabel(detail)
	body.Wrapping = fyne.TextWrapWord

	d := dialog.NewCustomWithoutButtons(title, container.NewVBox(body), u.win)

	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	proceed := widget.NewButton(confirm, func() {
		d.Hide()
		do()
	})
	proceed.Importance = widget.DangerImportance

	d.SetButtons([]fyne.CanvasObject{cancel, proceed})
	d.Resize(fyne.NewSize(560, 320))
	d.Show()
}

// confirmWithBody is confirmDestructive with widgets instead of a paragraph,
// for the confirmations that carry a choice — "also delete the file", "replace
// the symlink". The caller shows it, so it can hold on to the body's widgets
// and read them in the callback.
func (u *ui) confirmWithBody(title string, body fyne.CanvasObject, confirm string, do func()) *dialog.CustomDialog {
	d := dialog.NewCustomWithoutButtons(title, body, u.win)
	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	proceed := widget.NewButton(confirm, func() {
		d.Hide()
		do()
	})
	proceed.Importance = widget.DangerImportance
	d.SetButtons([]fyne.CanvasObject{cancel, proceed})
	d.Resize(fyne.NewSize(600, 360))
	return d
}

// pathDialog is the shape most of these take: a heading, one or more fields,
// and a confirm that hands the values to a core call.
func (u *ui) pathDialog(title, confirm string, body fyne.CanvasObject, do func()) {
	d := dialog.NewCustomConfirm(title, confirm, "Cancel", body, func(ok bool) {
		if ok {
			do()
		}
	}, u.win)
	d.Resize(fyne.NewSize(620, 340))
	d.Show()
}

// showDetail puts a long, read-only answer on screen: the per-script problems
// `mods check` found, or the release listing. A dialog rather than a section
// because it is the answer to a question that was just asked, and it should go
// away when it has been read.
func (u *ui) showDetail(title string, body fyne.CanvasObject, w, h float32) {
	d := dialog.NewCustom(title, "Close", body, u.win)
	d.Resize(fyne.NewSize(w, h))
	d.Show()
}

// fixedHeight and fixedWidth pin a widget's minimum size. Fyne's list and table
// take all the space they are given; these keep a section's layout stable while
// it is being looked at, which is what stops the build log from growing the
// window a line at a time.
func fixedHeight(o fyne.CanvasObject, h float32) fyne.CanvasObject {
	pad := canvas.NewRectangle(nil)
	pad.SetMinSize(fyne.NewSize(0, h))
	return container.New(layout.NewStackLayout(), pad, o)
}

func fixedWidth(o fyne.CanvasObject, w float32) fyne.CanvasObject {
	pad := canvas.NewRectangle(nil)
	pad.SetMinSize(fyne.NewSize(w, 0))
	return container.New(layout.NewStackLayout(), pad, o)
}

// --- file chooser ----------------------------------------------------------

// pickerStart is where a chooser should open: the path already in the field if
// it names a directory, otherwise its parent, otherwise the home directory.
// A field left empty, or holding something that no longer exists, must not
// leave the chooser at whatever directory the process happens to be in.
func pickerStart(text string) fyne.ListableURI {
	candidates := []string{}
	if p := config.ExpandPath(text); p != "" {
		candidates = append(candidates, p, filepath.Dir(p))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, home)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err != nil || !fi.IsDir() {
			continue
		}
		if lu, err := storage.ListerForURI(storage.NewFileURI(c)); err == nil {
			return lu
		}
	}
	return nil
}

// browseButton is the affordance beside a path field: it opens the platform
// chooser and writes the chosen path back into the field. The field stays
// editable — a path can still be typed or pasted, which is the only way to
// reach somewhere the chooser will not show.
//
// `dir` picks a directory chooser rather than a file one.
func (u *ui) browseButton(field *widget.Entry, dir bool) *widget.Button {
	choose := func() {
		if dir {
			d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
				if err != nil || lu == nil {
					return
				}
				field.SetText(lu.Path())
			}, u.win)
			d.SetLocation(pickerStart(field.Text))
			// Resize only after Show. Before Show the dialog has no window,
			// and Resize asks it for its minimum size — which in fyne 2.8.1
			// dereferences that nil window and takes the process with it.
			d.Show()
			d.Resize(fyne.NewSize(760, 520))
			return
		}
		d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil || rc == nil {
				return
			}
			// The chooser hands back an open handle; core reads the file
			// itself, by path, so close it immediately rather than holding a
			// descriptor open for the life of the dialog.
			path := rc.URI().Path()
			_ = rc.Close()
			field.SetText(path)
		}, u.win)
		d.SetLocation(pickerStart(field.Text))
		d.Show()
		d.Resize(fyne.NewSize(760, 520))
	}
	return widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), choose)
}

// withBrowse lays a path field out with its chooser button on the right.
func (u *ui) withBrowse(field *widget.Entry, dir bool) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, u.browseButton(field, dir), field)
}

// luaFilter limits the file chooser to mod scripts. A library that has
// collected a stray .txt is a mod that will not load, reported an hour later by
// a build; refusing it at the chooser is cheaper for everyone.
func luaFilter() storage.FileFilter {
	return storage.NewExtensionFileFilter([]string{".lua"})
}

// chooseFiles opens a multi-select file chooser. Fyne has no multi-select file
// dialog in 2.8.1, so "Add…" opens the single-file chooser and can be pressed
// again; core.AddMod takes a list either way, which is what makes the CLI's
// `mods add a.lua b.lua` and this one operation rather than two.
func (u *ui) chooseFile(filter storage.FileFilter, then func(path string)) {
	d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		path := rc.URI().Path()
		_ = rc.Close()
		then(path)
	}, u.win)
	if filter != nil {
		d.SetFilter(filter)
	}
	d.SetLocation(pickerStart(""))
	d.Show()
	d.Resize(fyne.NewSize(760, 520))
}

// chooseFolder opens a directory chooser.
func (u *ui) chooseFolder(start string, then func(path string)) {
	d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil || lu == nil {
			return
		}
		then(lu.Path())
	}, u.win)
	d.SetLocation(pickerStart(start))
	d.Show()
	d.Resize(fyne.NewSize(760, 520))
}

/*
openPath hands a file or directory to the desktop.

This is the one place the window starts a process that is not MBINCompiler, and
it is deliberately the smallest possible one: xdg-open decides what a .lua is
worth opening in, because the desktop already knows and this program has no
business having an opinion. A machine without xdg-open — a bare window manager,
a container — gets a warning banner naming the path, which is still enough to
open it by hand.
*/
func (u *ui) openPath(path string) {
	if path == "" {
		u.flash("There is nothing to open yet.", StatusWarn)
		return
	}
	go func() {
		done := u.busy("Opening " + filepath.Base(path) + "…")
		defer done()
		// The path is one this program produced or read out of its own settings
		// -- a script in the library, the workspace, the report directory --
		// and it is handed to xdg-open as a single argv element, so no shell
		// parses it.
		//
		// context.Background, deliberately: the point of this call is to hand
		// the file to whatever the desktop opens it with and let go. Tying the
		// child to a context of ours would mean closing the window took the
		// user's text editor with it.
		cmd := exec.CommandContext(context.Background(), "xdg-open", path) //nolint:gosec // a path from this program's own directories, passed as one argv element
		if err := cmd.Start(); err != nil {
			fyne.Do(func() {
				u.flash(fmt.Sprintf("Could not ask the desktop to open %s: %v. "+
					"Open it by hand; nothing else was affected.", path, err), StatusWarn)
			})
		}
	}()
}

package gui

import (
	"context"
	"fmt"
	"runtime"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// --- Settings (R2.6) -------------------------------------------------------

// parallelAuto is what the Select shows for "decide from the CPU count", which
// is what a stored 0 means.
const parallelAuto = "auto"

/*
settingsForm holds the form's widgets, kept apart from the section so Save and
Revert can be driven without a window manager.

The failure this shape prevents is a Revert that does not revert. Reverting by
discarding the loaded copy and rebuilding the section looks right and is not:
the fields would keep whatever was typed into them until the reload landed, and
on a slow config read the user sees their edits still there after asking for
them to be thrown away.
*/
type settingsForm struct {
	body       fyne.CanvasObject
	gameDir    *widget.Entry
	libraryDir *widget.Entry
	modName    *widget.Entry
	jobs       *widget.Select
	flavor     *widget.Select
	save       *widget.Button
	revert     *widget.Button
}

// newSettingsForm builds the form from the loaded settings.
func (u *ui) newSettingsForm() *settingsForm {
	values := u.configValues()
	f := &settingsForm{}

	f.gameDir = widget.NewEntry()
	f.gameDir.SetPlaceHolder("empty means: find it from Steam's own manifests")
	f.libraryDir = widget.NewEntry()
	f.modName = widget.NewEntry()
	f.jobs = widget.NewSelect(parallelOptions(), nil)
	f.flavor = widget.NewSelect(
		[]string{config.FlavorAuto, config.FlavorDotnet10, config.FlavorSelfContained}, nil)
	f.set(values)

	auto := widget.NewButton("Use auto-detection", func() { f.gameDir.SetText("") })

	f.save = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		u.saveSettings(f.values())
	})
	f.save.Importance = widget.HighImportance
	f.revert = widget.NewButtonWithIcon("Revert", theme.ContentUndoIcon(), func() {
		f.set(u.configValues())
		u.flash("Put the fields back to what "+u.settings.Path+" says. Nothing was written.",
			StatusInfo)
	})
	u.gate(f.save, f.revert, auto)

	f.body = container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Game directory",
				container.NewBorder(nil, nil, nil, auto, u.withBrowse(f.gameDir, true))),
			widget.NewFormItem("Mod library", u.withBrowse(f.libraryDir, true)),
			widget.NewFormItem("Output folder name", f.modName),
			widget.NewFormItem("Parallel jobs", f.jobs),
			widget.NewFormItem("Compiler flavor", f.flavor),
		),
		container.NewHBox(f.save, f.revert),
	)
	return f
}

// set fills the widgets from a settings map.
func (f *settingsForm) set(values map[string]string) {
	f.gameDir.SetText(values["game_dir"])
	f.libraryDir.SetText(values["library_dir"])
	f.modName.SetText(values["mod_name"])
	f.jobs.SetSelected(parallelValue(values["parallel"]))
	f.flavor.SetSelected(orNone(values["mbincompiler.flavor"], config.FlavorAuto))
}

// values reads the widgets back into the map saveSettings compares.
func (f *settingsForm) values() map[string]string {
	return map[string]string{
		"game_dir":            f.gameDir.Text,
		"library_dir":         f.libraryDir.Text,
		"mod_name":            f.modName.Text,
		"parallel":            parallelSetting(f.jobs.Selected),
		"mbincompiler.flavor": f.flavor.Selected,
	}
}

/*
buildSettings is the settings document, as a form.

Every field here is a key in config.json, which is also what `nmsbonker config
set` writes -- one document, so the window and the terminal cannot disagree
about where the game is. The validation is core's for the same reason: a flavor
this form accepted and the installer rejected would be a difference with no
reason behind it.

Appearance is deliberately not here. It is the only thing this application keeps
in Fyne's own preference store, because it is the only setting the command line
has no use for.
*/
func (u *ui) buildSettings() fyne.CanvasObject {
	u.loadConfig()
	if !u.configOK {
		return container.NewVScroll(container.NewVBox(
			heading("Settings", "Reading the settings…")))
	}

	f := u.newSettingsForm()

	resolved := card("Where these resolve to",
		plainRow("Game directory", orNone(u.settings.GameDir, "not found")+
			" ["+u.settings.GameDirSource+"]"),
		plainRow("Mod library", u.settings.Paths.Library),
		plainRow("Tools", u.settings.Paths.Tools),
		plainRow("Cache", u.settings.Paths.Cache),
		plainRow("Workspace", u.settings.Paths.Workspace),
		plainRow("Deploy archive", u.settings.Paths.Archive),
	)

	body := container.NewVBox(
		heading("Settings", "What nmsbonker uses, and where it keeps things."),
		f.body,
		note("Saving writes "+u.settings.Path+". That file is the settings: `nmsbonker "+
			"config show` in a terminal reads the same one, and this window was started "+
			"against it.", StatusInfo),
		note("Parallel jobs at "+parallelAuto+" is half this machine's CPUs ("+
			strconv.Itoa(max(1, runtime.NumCPU()/2))+" here). MBINCompiler is one .NET process "+
			"per file, and the machine running the build is also running the desktop you are "+
			"looking at.", StatusInfo),
		note("The colour scheme, the font and the text size are not in this file. They are the "+
			"only thing this application keeps in Fyne's own preference store, because the "+
			"command line has no use for them — see Appearance.", StatusInfo),
		widget.NewSeparator(),
		resolved,
	)
	if u.settings.GameDirSource == "env "+config.GameDirEnv {
		body.Add(note("$"+config.GameDirEnv+" is set and outranks the setting above, so the "+
			"game directory in this form is not the one being used. Unset it to go back to "+
			"the saved value.", StatusWarn))
	}
	return container.NewVScroll(body)
}

// configValues flattens the settings into the map the form reads.
func (u *ui) configValues() map[string]string {
	out := map[string]string{}
	for _, e := range u.settings.Entries {
		out[e.Key] = e.Value
	}
	return out
}

// parallelOptions is "auto" plus every worker count this machine could use.
func parallelOptions() []string {
	out := []string{parallelAuto}
	for i := 1; i <= runtime.NumCPU(); i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

// parallelValue maps the stored setting onto the Select. A stored 0 is "decide
// from the CPU count", which is a choice rather than an absent value.
func parallelValue(stored string) string {
	if stored == "" || stored == "0" {
		return parallelAuto
	}
	return stored
}

// parallelSetting maps the Select back onto the stored value.
func parallelSetting(selected string) string {
	if selected == parallelAuto || selected == "" {
		return "0"
	}
	return selected
}

// --- settings operations ---------------------------------------------------

// loadConfig reads the settings document.
func (u *ui) loadConfig() {
	if u.configOK {
		return
	}
	u.configOK = true
	go func() {
		done := u.busy("Reading the settings…")
		defer done()
		res, err := core.ConfigShow(context.Background(), core.ConfigShowRequest{Request: u.request()})
		if err != nil {
			u.report("Read the settings", err)
			return
		}
		fyne.Do(func() {
			u.settings = res
			u.refresh()
		})
	}()
}

/*
saveSettings writes the changed keys, and only the changed ones.

One core call per key, because that is the operation core has and the CLI's
`config set KEY VALUE` is the same one. Writing every key on every Save would
also work, but it would rewrite values the user did not touch — and the first
time that matters is the day a setting means something this build does not know
about.
*/
func (u *ui) saveSettings(want map[string]string) {
	current := u.configValues()
	changed := map[string]string{}
	for k, v := range want {
		if current[k] != v {
			changed[k] = v
		}
	}
	if len(changed) == 0 {
		u.flash("Nothing changed, so nothing was written.", StatusInfo)
		return
	}
	u.perform("Saving the settings…", func(ctx context.Context) error {
		var written []string
		for key, value := range changed {
			res, err := core.ConfigSet(ctx, core.ConfigSetRequest{
				Request: u.request(), Key: key, Value: value,
			})
			if err != nil {
				// Stop at the first refusal rather than pressing on: the keys
				// after it would be written against a form the user is about to
				// come back and fix.
				return fmt.Errorf("%s: %w", key, err)
			}
			if !res.Unchanged {
				written = append(written, key)
			}
		}
		fyne.Do(func() { u.configOK = false })
		u.ok(fmt.Sprintf("Wrote %d setting(s) to %s: %s.",
			len(written), u.settings.Path, joinLines(written)))
		return nil
	})
}

// setConfig writes one key, for the shortcuts outside the Settings form — the
// game-directory field on Overview's first-run state.
func (u *ui) setConfig(key, value, label string) {
	u.perform("Saving "+label+"…", func(ctx context.Context) error {
		res, err := core.ConfigSet(ctx, core.ConfigSetRequest{
			Request: u.request(), Key: key, Value: value,
		})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.configOK = false })
		if res.Unchanged {
			u.ok(label + " was already " + res.New + ".")
			return nil
		}
		u.ok(label + " is now " + orNone(res.New, "auto-detected") + ", written to " + res.Path + ".")
		return nil
	})
}

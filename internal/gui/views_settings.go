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
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"

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
	// limits are the six audit thresholds, keyed by their config key
	// (spec 005 R3.3). Held in a map rather than six fields because they are
	// six numbers that differ only in their name, and six named fields would be
	// six chances to read one into another.
	limits map[string]*widget.Entry
	save   *widget.Button
	revert *widget.Button
}

/*
auditKeys are the six reward-amount limits, in the order the form lays them out
and with the label each gets (R3.3).

The order is absolute limits first, worst case last, and then the ratio, which
is the odd one out: the other five are "an amount this large is wrong" and the
ratio is "a change this large is suspicious whatever the amount".
*/
//
//nolint:gochecknoglobals // a fixed list, read-only after initialisation
var auditKeys = []struct{ key, label string }{
	{"audit.max_product", "Largest product reward"},
	{"audit.max_substance", "Largest substance reward"},
	{"audit.max_units", "Largest units reward"},
	{"audit.max_nanites", "Largest nanites reward"},
	{"audit.max_specials", "Largest quicksilver reward"},
	{"audit.max_ratio", "Largest change from stock (x)"},
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
	f.limits = map[string]*widget.Entry{}
	for _, l := range auditKeys {
		f.limits[l.key] = widget.NewEntry()
	}
	f.set(values)

	auto := widget.NewButton("Use auto-detection", func() { f.gameDir.SetText("") })

	f.save = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		u.saveSettings(f.values())
	})
	f.save.Importance = widget.HighImportance
	f.revert = widget.NewButtonWithIcon("Revert", theme.ContentUndoIcon(), func() {
		f.set(u.configValues())
		u.sh.Flash("Put the fields back to what "+u.settings.Path+" says. Nothing was written.",
			fd.StatusInfo)
	})
	u.sh.Gate(f.save, f.revert, auto)

	f.body = container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Game directory",
				container.NewBorder(nil, nil, nil, auto, dialogs.WithBrowse(u.sh.Window, f.gameDir, true))),
			widget.NewFormItem("Mod library", dialogs.WithBrowse(u.sh.Window, f.libraryDir, true)),
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
	f.flavor.SetSelected(widgets.OrNone(values["mbincompiler.flavor"], config.FlavorAuto))
	for key, entry := range f.limits {
		entry.SetText(values[key])
	}
}

// values reads the widgets back into the map saveSettings compares.
func (f *settingsForm) values() map[string]string {
	out := map[string]string{
		"game_dir":            f.gameDir.Text,
		"library_dir":         f.libraryDir.Text,
		"mod_name":            f.modName.Text,
		"parallel":            parallelSetting(f.jobs.Selected),
		"mbincompiler.flavor": f.flavor.Selected,
	}
	for key, entry := range f.limits {
		out[key] = entry.Text
	}
	return out
}

/*
auditGroup is the Audit limits block (R3.3).

Plain numeric fields rather than sliders: the useful values span four orders of
magnitude -- a product stack is five figures and a units payout is nine -- and a
slider across that range cannot be aimed. Reset puts the six back to the
defaults in the fields; nothing is written until Save, like the rest of the form.
*/
func (u *ui) auditGroup(f *settingsForm) fyne.CanvasObject {
	items := make([]*widget.FormItem, 0, len(auditKeys))
	for _, l := range auditKeys {
		items = append(items, widget.NewFormItem(l.label, f.limits[l.key]))
	}
	reset := widget.NewButtonWithIcon("Reset limits to defaults", theme.ContentUndoIcon(),
		func() {
			for key, value := range defaultAuditValues() {
				f.limits[key].SetText(value)
			}
			u.sh.Flash("The audit limits in the form are back to their defaults. "+
				"Save to write them.", fd.StatusInfo)
		})
	u.sh.Gate(reset)
	return widgets.Card("Audit limits",
		widgets.Note("What counts as a reward amount worth warning about. A build flags an amount "+
			"over one of these and names the mods that made it; nothing is changed or "+
			"disabled. The first five are absolute; the last is how many times the "+
			"game's own value an amount may reach. Re-check in the Report section "+
			"applies a new limit to the last build without rebuilding.", fd.StatusInfo),
		widget.NewForm(items...),
		container.NewHBox(reset),
	)
}

// defaultAuditValues is the audit package's defaults in the string form the
// form and `config set` both use.
func defaultAuditValues() map[string]string {
	d := config.DefaultAudit()
	cfg := config.Defaults()
	cfg.Audit = d
	out := map[string]string{}
	for _, l := range auditKeys {
		v, err := cfg.Get(l.key)
		if err != nil {
			continue
		}
		out[l.key] = v
	}
	return out
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
			widgets.Heading("Settings", "Reading the settings…")))
	}

	f := u.newSettingsForm()

	resolved := widgets.Card("Where these resolve to",
		widgets.PlainRow("Game directory", widgets.OrNone(u.settings.GameDir, "not found")+
			" ["+u.settings.GameDirSource+"]"),
		widgets.PlainRow("Mod library", u.settings.Paths.Library),
		widgets.PlainRow("Tools", u.settings.Paths.Tools),
		widgets.PlainRow("Cache", u.settings.Paths.Cache),
		widgets.PlainRow("Workspace", u.settings.Paths.Workspace),
		widgets.PlainRow("Deploy archive", u.settings.Paths.Archive),
	)

	body := container.NewVBox(
		widgets.Heading("Settings", "What nmsbonker uses, and where it keeps things."),
		f.body,
		widgets.Note("Saving writes "+u.settings.Path+". That file is the settings: `nmsbonker "+
			"config show` in a terminal reads the same one, and this window was started "+
			"against it.", fd.StatusInfo),
		widgets.Note("Parallel jobs at "+parallelAuto+" is half this machine's CPUs ("+
			strconv.Itoa(max(1, runtime.NumCPU()/2))+" here). MBINCompiler is one .NET process "+
			"per file, and the machine running the build is also running the desktop you are "+
			"looking at.", fd.StatusInfo),
		widgets.Note("The colour scheme, the font and the text size are not in this file. They are the "+
			"only thing this application keeps in Fyne's own preference store, because the "+
			"command line has no use for them — see Appearance.", fd.StatusInfo),
		widget.NewSeparator(),
		u.auditGroup(f),
		widget.NewSeparator(),
		resolved,
	)
	if u.settings.GameDirSource == "env "+config.GameDirEnv {
		body.Add(widgets.Note("$"+config.GameDirEnv+" is set and outranks the setting above, so the "+
			"game directory in this form is not the one being used. Unset it to go back to "+
			"the saved value.", fd.StatusWarn))
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
	u.load("Reading the settings…", func() {
		res, err := core.ConfigShow(context.Background(), core.ConfigShowRequest{Request: u.request()})
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the settings", err) })
			return
		}
		fyne.Do(func() {
			u.settings = res
			u.sh.Refresh()
		})
	})
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
		u.sh.Flash("Nothing changed, so nothing was written.", fd.StatusInfo)
		return
	}
	u.sh.Perform("Saving the settings…", func(ctx context.Context) error {
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
		fyne.Do(func() {
			u.sh.OK(fmt.Sprintf("Wrote %d setting(s) to %s: %s.",
				len(written), u.settings.Path, joinLines(written)))
		})
		return nil
	})
}

// setConfig writes one key, for the shortcuts outside the Settings form — the
// game-directory field on Overview's first-run state.
func (u *ui) setConfig(key, value, label string) {
	u.sh.Perform("Saving "+label+"…", func(ctx context.Context) error {
		res, err := core.ConfigSet(ctx, core.ConfigSetRequest{
			Request: u.request(), Key: key, Value: value,
		})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.configOK = false })
		if res.Unchanged {
			fyne.Do(func() { u.sh.OK(label + " was already " + res.New + ".") })
			return nil
		}
		fyne.Do(func() {
			u.sh.OK(label + " is now " + widgets.OrNone(res.New, "auto-detected") + ", written to " + res.Path + ".")
		})
		return nil
	})
}

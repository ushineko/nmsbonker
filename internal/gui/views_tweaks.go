package gui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/core"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

// --- Tweaks (spec 004 R2.1) -------------------------------------------------

/*
buildTweaks is the section this project's own mods live in.

A tweak is a mod with declared parameters, which is the whole difference: the
Mods section can only offer a mod's name and its place in the order, and here
there is a slider with a range and a label saying what the number means. Grouped
by subject rather than listed in build order, because someone arriving at this
section is looking for "make mining better", not for position seven.

Nothing here reflows while it is being used. A parameter that has moved since
the last build shows a warn-coloured line in its card header, and that line's
space is reserved whether or not there is anything in it -- a card that grows a
row when a slider is dragged would move the next card out from under the mouse.
*/
func (u *ui) buildTweaks() fyne.CanvasObject {
	u.loadTweaks()

	if !u.tweaksOK {
		return container.NewVScroll(container.NewVBox(
			heading("Tweaks", "Reading the built-in tweaks…")))
	}

	body := container.NewVBox(heading("Tweaks",
		"The mods that come with nmsbonker. Turn one on, set what it does, and build."))

	byGroup := map[string][]core.TweakInfo{}
	for _, tw := range u.tweaks.Tweaks {
		byGroup[tw.Group] = append(byGroup[tw.Group], tw)
	}
	for _, group := range u.tweaks.Groups {
		rows := []fyne.CanvasObject{}
		for i, tw := range byGroup[group] {
			if i > 0 {
				rows = append(rows, widget.NewSeparator())
			}
			rows = append(rows, u.tweakCard(tw))
		}
		body.Add(card(group, rows...))
		body.Add(widget.NewSeparator())
	}

	return container.NewBorder(nil, u.tweaksActions(), nil, nil, container.NewVScroll(body))
}

/*
tweakCard is one built-in: its switch, what it does, and its parameters.

The switch is a Check rather than a row action because a tweak is a thing you
turn on, and the alternative -- select the row, press Enable -- is two gestures
for a light switch.
*/
func (u *ui) tweakCard(tw core.TweakInfo) fyne.CanvasObject {
	on := widget.NewCheck(tw.Title, nil)
	on.Checked = tw.Enabled
	on.OnChanged = func(checked bool) { u.setModEnabled([]string{tw.Name}, checked) }

	// The unbuilt marker lives in a line that is always there, so turning a
	// slider does not move everything below it.
	unbuilt := widget.NewLabel("")
	unbuilt.Importance = widget.WarningImportance
	if tw.Unbuilt {
		unbuilt.SetText("changed since the last build — build to apply")
	}

	head := container.NewBorder(nil, nil,
		container.NewHBox(on, dim("#"+strconv.Itoa(tw.Order))), nil, unbuilt)

	desc := widget.NewLabel(tw.Desc)
	desc.Wrapping = fyne.TextWrapWord
	desc.Importance = widget.LowImportance

	rows := []fyne.CanvasObject{head, desc}
	if tw.Shadowed {
		rows = append(rows, note("A script of this name is also in your library. The built-in "+
			"is the one that builds; the library copy is ignored. Remove it from the Mods "+
			"section to stop it showing up there.", StatusWarn))
	}
	for _, p := range tw.Params {
		rows = append(rows, u.paramRow(tw.Name, p, unbuilt))
	}
	return container.NewVBox(rows...)
}

/*
paramRow is one parameter: a slider, a field that agrees with it, and Reset.

Both controls, not one. The slider is how you find a value you like by feel; the
field is how you type 250000 into a range that goes to a million without
dragging across nine hundred thousand of it. They are kept in step, and only one
of them commits: the slider on release, the field on Enter, so a drag is one
write to the settings rather than four hundred.
*/
func (u *ui) paramRow(mod string, p core.TweakParam, unbuilt *widget.Label) fyne.CanvasObject {
	label := widget.NewLabel(p.Label)
	label.Importance = widget.LowImportance

	entry := widget.NewEntry()
	entry.SetText(modscript.FormatValue(p.Current, p.Kind))

	def := dim("default " + modscript.FormatValue(p.Default, p.Kind))
	reset := widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), nil)

	var slider *widget.Slider
	commit := func(v float64) {
		u.applyParam(mod, p, v, unbuilt, func(applied float64) {
			text := modscript.FormatValue(applied, p.Kind)
			if entry.Text != text {
				entry.SetText(text)
			}
			if slider != nil && slider.Value != applied {
				slider.Value = applied
				slider.Refresh()
			}
		})
	}

	entry.OnSubmitted = func(text string) {
		v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			u.flash(p.Label+": "+strconv.Quote(text)+" is not a number.", StatusWarn)
			entry.SetText(modscript.FormatValue(p.Current, p.Kind))
			return
		}
		commit(v)
	}
	reset.OnTapped = func() { u.resetParam(mod, p, unbuilt, entry, slider) }

	control := fyne.CanvasObject(entry)
	if p.Bounded {
		slider = widget.NewSlider(p.Min, p.Max)
		slider.Step = p.Step
		slider.Value = p.Current
		// Live text while dragging, one write on release. OnChanged fires per
		// pixel, and a settings file written per pixel is a settings file
		// written four hundred times to move one multiplier.
		slider.OnChanged = func(v float64) { entry.SetText(modscript.FormatValue(v, p.Kind)) }
		slider.OnChangeEnded = commit
		control = container.NewBorder(nil, nil, nil, fixedWidth(entry, 120), slider)
	}

	return container.NewBorder(nil, nil,
		fixedWidth(label, 220),
		container.NewHBox(fixedWidth(def, 140), reset),
		control)
}

// applyParam writes one parameter and updates the card in place.
//
// In place rather than through invalidate(): rebuilding the section on every
// slider release would take the focus away from the control being used and
// scroll the card being read back to the top.
func (u *ui) applyParam(mod string, p core.TweakParam, v float64,
	unbuilt *widget.Label, show func(float64),
) {
	u.perform(fmt.Sprintf("Setting %s %s…", mod, p.Name), func(ctx context.Context) error {
		res, err := core.SetTweakParam(ctx, core.SetTweakParamRequest{
			Request: u.request(), Name: mod, Param: p.Name, Value: v,
		})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			show(res.New)
			u.noteParam(mod, p.Name, res.New, true)
			unbuilt.SetText("changed since the last build — build to apply")
			if res.Clamped {
				u.flash(fmt.Sprintf("%s only takes %s to %s, so %s was used.",
					p.Label, modscript.FormatValue(p.Min, p.Kind),
					modscript.FormatValue(p.Max, p.Kind),
					modscript.FormatValue(res.New, p.Kind)), StatusWarn)
			}
		})
		return nil
	})
}

// resetParam restores one parameter to the script's own value.
func (u *ui) resetParam(mod string, p core.TweakParam, unbuilt *widget.Label,
	entry *widget.Entry, slider *widget.Slider,
) {
	u.perform("Resetting "+mod+" "+p.Name+"…", func(ctx context.Context) error {
		if _, err := core.ResetTweak(ctx, core.ResetTweakRequest{
			Request: u.request(), Name: mod, Param: p.Name,
		}); err != nil {
			return err
		}
		fyne.Do(func() {
			entry.SetText(modscript.FormatValue(p.Default, p.Kind))
			if slider != nil {
				slider.Value = p.Default
				slider.Refresh()
			}
			u.noteParam(mod, p.Name, p.Default, false)
			unbuilt.SetText("changed since the last build — build to apply")
		})
		return nil
	})
}

// noteParam records a change in the loaded model so a later rebuild of the
// section draws what is on screen rather than what was loaded. On the UI thread.
func (u *ui) noteParam(mod, param string, value float64, overridden bool) {
	for i := range u.tweaks.Tweaks {
		if u.tweaks.Tweaks[i].Name != mod {
			continue
		}
		for j := range u.tweaks.Tweaks[i].Params {
			if u.tweaks.Tweaks[i].Params[j].Name != param {
				continue
			}
			u.tweaks.Tweaks[i].Params[j].Current = value
			u.tweaks.Tweaks[i].Params[j].Overridden = overridden
		}
		u.tweaks.Tweaks[i].Unbuilt = true
	}
	u.tweaks.Unbuilt = true
}

// tweaksActions is the bottom strip: the button the section exists to lead to,
// and a way back to the script's own numbers.
func (u *ui) tweaksActions() fyne.CanvasObject {
	build := widget.NewButtonWithIcon("Apply and build", theme.MediaPlayIcon(), func() {
		u.selectSection("Build")
		u.startBuild(false, false, false)
	})
	build.Importance = widget.HighImportance

	resetAll := widget.NewButtonWithIcon("Reset all to defaults", theme.ContentUndoIcon(),
		func() { u.resetAllTweaks() })
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.invalidate() })
	u.gate(build, resetAll, refresh)
	if !u.status.Install.Found || !u.status.Compiler.Installed {
		build.Disable()
	}

	state := widget.NewLabel("")
	state.Wrapping = fyne.TextWrapWord
	switch {
	case u.tweaks.Unbuilt:
		state.Importance = widget.WarningImportance
		state.SetText("Parameters have changed since the last build. Nothing in the game " +
			"changes until you build and deploy.")
	case u.tweaks.LastBuild != "":
		state.Importance = widget.LowImportance
		state.SetText("The last build, " + u.tweaks.LastBuild + ", used these values.")
	default:
		state.Importance = widget.LowImportance
		state.SetText("Nothing has been built yet.")
	}

	return container.NewVBox(widget.NewSeparator(), state,
		container.NewHBox(build, resetAll, refresh))
}

// resetAllTweaks restores every built-in's own values, with a confirmation:
// it discards every number the user has set, and there is no undo for it.
func (u *ui) resetAllTweaks() {
	changed := 0
	for _, tw := range u.tweaks.Tweaks {
		for _, p := range tw.Params {
			if p.Overridden {
				changed++
			}
		}
	}
	if changed == 0 {
		u.flash("Every tweak is already at the values its script carries.", StatusInfo)
		return
	}
	u.confirmDestructive("Reset every tweak?",
		fmt.Sprintf("%d parameter(s) you have changed go back to the values the scripts "+
			"carry. Nothing installed in the game changes until the next build and deploy, "+
			"and which tweaks are switched on is not affected.", changed),
		"Reset", func() {
			u.perform("Resetting every tweak…", func(ctx context.Context) error {
				for _, tw := range u.tweaks.Tweaks {
					if _, err := core.ResetTweak(ctx, core.ResetTweakRequest{
						Request: u.request(), Name: tw.Name,
					}); err != nil {
						return err
					}
				}
				u.ok(fmt.Sprintf("Reset %d parameter(s) to the values the scripts carry.", changed))
				return nil
			})
		})
}

package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"
)

// --- Appearance (R2.7) -----------------------------------------------------

// sampleLogLine is the monospace line the live sample shows: a build log line
// that looks like this program's own is the useful preview.
const sampleLogLine = "   OK  BigStacks: METADATA/REALITY/TABLES/REWARDTABLE.MBIN 14 edit(s) applied"

// buildAppearance is the scheme, font, and text-size picker. Fyne draws its own
// widgets, so these three settings are the whole of what makes the window look
// like it belongs on the user's desktop — which is why they are a section
// rather than a line in a preferences dialog.
func (u *ui) buildAppearance() fyne.CanvasObject {
	a := u.appearance
	apply := func() {
		u.appearance = a
		u.applyAppearance()
	}

	scheme := widget.NewSelect(fdtheme.SchemeNames(), func(name string) {
		a.Scheme = name
		apply()
	})
	scheme.SetSelected(a.Scheme)

	font := widget.NewSelect(fdtheme.FontNames(), func(name string) {
		a.Font = name
		apply()
	})
	font.SetSelected(a.Font)

	sizes := make([]string, 0, len(fdtheme.TextSizes()))
	for _, s := range fdtheme.TextSizes() {
		sizes = append(sizes, fmt.Sprintf("%g", s))
	}
	size := widget.NewSelect(sizes, func(v string) {
		for _, s := range fdtheme.TextSizes() {
			if fmt.Sprintf("%g", s) == v {
				a.TextSize = s
				apply()
				return
			}
		}
	})
	size.SetSelected(fmt.Sprintf("%g", a.TextSize))

	reset := widget.NewButton("Reset to defaults", func() {
		d := fdtheme.DefaultAppearance()
		a.Scheme, a.Font, a.TextSize = d.Scheme, d.Font, d.TextSize
		scheme.SetSelected(a.Scheme)
		font.SetSelected(a.Font)
		size.SetSelected(fmt.Sprintf("%g", a.TextSize))
		apply()
	})

	form := widget.NewForm(
		widget.NewFormItem("Color scheme", scheme),
		widget.NewFormItem("Font", font),
		widget.NewFormItem("Text size", size),
	)

	return container.NewVScroll(container.NewVBox(
		widgets.Heading("Appearance", "How this window looks. Fyne draws its own widgets, so this is what decides whether it sits well next to the rest of your desktop."),
		form,
		container.NewHBox(reset),
		widgets.Dim("These are saved and restored the next time the window opens. They are the only thing "+
			"this application keeps in Fyne's own preference store; everything else lives in "+
			"config.json, which the command line reads too."),
		widget.NewSeparator(),
		fdtheme.Sample(sampleLogLine),
		widget.NewSeparator(),
		widgets.Dim("The KDE schemes are transcribed from the desktop's colour-scheme files, the Adwaita ones from "+
			"libadwaita's named colours, the Windows and macOS ones from their published design tokens. They are "+
			"compiled in, so the window does not follow the desktop's current scheme and needs no desktop installed."),
		widgets.Dim("Fonts are read from the system font directories. Fyne draws its own text and does "+
			"not consult fontconfig, so this list is what was found on disk rather than what "+
			"the desktop is configured to use. A family with no bold or italic face is drawn "+
			"in its regular face for those styles."),
	))
}

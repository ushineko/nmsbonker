package gui

import (
	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/shell"
)

// --- Appearance (R2.7) -----------------------------------------------------

// sampleLogLine is the monospace line the Appearance section's live sample
// shows: a build log line that looks like this program's own is the useful
// preview.
const sampleLogLine = "   OK  BigStacks: METADATA/REALITY/TABLES/REWARDTABLE.MBIN 14 edit(s) applied"

// buildAppearance is the library's scheme, font, text-size and scale picker.
// Fyne draws its own widgets, so these settings are the whole of what makes
// the window look like it belongs on the user's desktop, which is why they are
// a section rather than a line in a preferences dialog. They are the only thing
// this application keeps in Fyne's own preference store; everything else lives
// in config.json, which the command line reads too.
func (u *ui) buildAppearance() fyne.CanvasObject {
	return shell.AppearanceSection(sampleLogLine).Build(u.sh)
}

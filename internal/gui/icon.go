// Copied from angou (same author) — keep in sync by hand.

package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/nmsbonker/internal/gui/assets"
)

// The window and taskbar icon.
//
// The bytes live in internal/gui/assets, embedded at build time.
// packaging/nmsbonker.svg is the same drawing again, on disk for the installer
// to place into the icon theme.
func appIcon() fyne.Resource { return fyne.NewStaticResource("nmsbonker.svg", assets.IconSVG()) }

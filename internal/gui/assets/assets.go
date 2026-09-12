/*
Package assets holds the files the desktop front end embeds.

The icon is a copy of packaging/nmsbonker.svg. It is duplicated rather than
referenced because go:embed cannot reach outside the package directory, and the
packaging copy has to stay on disk for the desktop entry and the icon theme to
install (spec 004 R7.1). Change one and change the other.
*/
package assets

import _ "embed"

//go:embed nmsbonker.svg
var iconSVG []byte

// IconSVG is the application icon: the window icon, the taskbar icon, and the
// image the About section draws at 72 px.
func IconSVG() []byte { return iconSVG }

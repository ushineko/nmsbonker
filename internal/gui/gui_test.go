package gui

import (
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	fdtheme "github.com/ushineko/fynedesygn/theme"
)

/*
The window's tests run headless.

Fyne's test driver draws into memory and runs fyne.Do inline, so everything here
works with no DISPLAY and no WAYLAND_DISPLAY -- which is the requirement, since
`make test` runs on machines that have neither.

What is deliberately not tested here is the sections' layout. A test asserting
that the Build section holds a Border holding a VBox tests the code against
itself and breaks on every rearrangement; the tests below pin behaviour instead
-- what a cell says, when a button is enabled, what the log keeps -- and each
one names the bug it prevents.
*/

// testUI is a window with no window: enough of a *ui for the parts that do not
// draw. Sections are not built from it unless the test says so, because a
// section asks the core for its data and a test has no game install.
func testUI(t *testing.T) *ui {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	u := &ui{app: app, win: test.NewWindow(widget.NewLabel("")), flashes: container.NewVBox()}
	u.loadAppearance()
	u.run.init()
	t.Cleanup(func() { u.win.Close() })
	return u
}

// The log pane's levels, named so a test reads as a sentence.
func logLevelWarn() logpane.Level  { return logpane.Warn }
func logLevelDebug() logpane.Level { return logpane.Debug }

// TestSectionNamesNeedsNoApp: the flag help lists these while parsing flags,
// before there is a Fyne app to construct a theme icon against. Asking for one
// then made angou-gui --version print seven Fyne errors before its answer.
//
// This test would not catch that on its own — the logging is a side effect, not
// a failure — so it asserts the thing underneath: the names are available
// without touching the section table, and they still agree with it.
func TestSectionNamesNeedsNoApp(t *testing.T) {
	names := SectionNames()
	require.Equal(t, []string{
		"Overview", "Mods", "Tweaks", "Build", "Report", "Saves", "Tools", "Settings", "Appearance", "About",
	}, names)

	// Every advertised name must have a section behind it. sections() walks
	// these titles, so a name with no builder would be offered by --section
	// while drawing nothing.
	for _, n := range names {
		require.Containsf(t, sectionBuilders(), n, "%q is advertised but has no section", n)
	}
	require.Len(t, sectionBuilders(), len(names),
		"a section exists that the navigation never shows")
}

// SchemeNames is in the --scheme help for the same reason, and an empty list
// would make that flag undocumented rather than broken, which is worse.
func TestSchemeNamesListsEveryPalette(t *testing.T) {
	require.Equal(t, fdtheme.SchemeNames(), SchemeNames())
	require.Subset(t, SchemeNames(), []string{
		"Breeze Dark", "Breeze Light", "Oxygen Dark", "Adwaita Dark", "Adwaita Light",
	}, "the schemes the previous build offered are still offered under the same names")
	require.Equal(t, fdtheme.DefaultScheme().Name, fdtheme.SchemeByName("no such scheme").Name,
		"a stale preference must fall back rather than fail")
}

// A name in Actions() is a claim that the GUI reaches that operation. A
// duplicate would make the parity test pass with one of them unimplemented.
func TestActionsAreUniqueAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Actions() {
		require.NotEmpty(t, a)
		require.Falsef(t, seen[a], "%q is listed twice", a)
		seen[a] = true
	}
	require.NotEmpty(t, seen)
}

// --- flash ---------------------------------------------

// A failure waits to be dismissed. One that removes itself on a timer is an
// error nobody read, describing an operation that has already not happened.
func TestFlashKeepsFailuresUntilDismissed(t *testing.T) {
	_, fades := flashHold(fd.StatusBad)
	require.False(t, fades, "a failure must not clear itself")

	warn, fades := flashHold(fd.StatusWarn)
	require.True(t, fades)
	good, _ := flashHold(fd.StatusGood)
	require.Greater(t, warn, good, "a warning names a condition to act on, so it stays longer")
	require.GreaterOrEqual(t, good.Seconds(), 5.0,
		"a banner must be up long enough to read, not merely long enough to notice")
}

// One banner at a time. The slot has a fixed height so nothing reflows when a
// result arrives, which only works if results replace each other rather than
// stacking up inside it.
func TestFlashShowsOneBannerAtATime(t *testing.T) {
	u := testUI(t)

	u.flash("first", fd.StatusGood)
	require.Len(t, u.flashes.Objects, 1)

	u.flash("second", fd.StatusBad)
	require.Len(t, u.flashes.Objects, 1, "a newer result replaces the older one")
}

// The fade timer of a banner that has already been replaced must not empty the
// slot underneath the banner that replaced it.
func TestClearFlashIgnoresAStaleTimer(t *testing.T) {
	u := testUI(t)

	u.flash("first", fd.StatusGood)
	stale := u.flashSeq
	u.flash("second", fd.StatusGood)

	u.clearFlash(stale)
	require.Len(t, u.flashes.Objects, 1, "the newer banner still owns the slot")

	u.clearFlash(u.flashSeq)
	require.Empty(t, u.flashes.Objects, "dismissing the current banner empties the slot")
}

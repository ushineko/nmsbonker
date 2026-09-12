package gui

import (
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
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
	u.run.init()
	t.Cleanup(func() { u.win.Close() })
	return u
}

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
		"Overview", "Mods", "Build", "Report", "Tools", "Settings", "Appearance", "About",
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
	require.Equal(t, []string{
		"Breeze Dark", "Breeze Light", "Oxygen Dark", "Adwaita Dark", "Adwaita Light",
	}, SchemeNames())
	require.Equal(t, "Breeze Dark", paletteByName("no such scheme").name,
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

// --- flash (copied from angou) ---------------------------------------------

// A failure waits to be dismissed. One that removes itself on a timer is an
// error nobody read, describing an operation that has already not happened.
func TestFlashKeepsFailuresUntilDismissed(t *testing.T) {
	_, fades := flashHold(StatusBad)
	require.False(t, fades, "a failure must not clear itself")

	warn, fades := flashHold(StatusWarn)
	require.True(t, fades)
	good, _ := flashHold(StatusGood)
	require.Greater(t, warn, good, "a warning names a condition to act on, so it stays longer")
	require.GreaterOrEqual(t, good.Seconds(), 5.0,
		"a banner must be up long enough to read, not merely long enough to notice")
}

// One banner at a time. The slot has a fixed height so nothing reflows when a
// result arrives, which only works if results replace each other rather than
// stacking up inside it.
func TestFlashShowsOneBannerAtATime(t *testing.T) {
	u := testUI(t)

	u.flash("first", StatusGood)
	require.Len(t, u.flashes.Objects, 1)

	u.flash("second", StatusBad)
	require.Len(t, u.flashes.Objects, 1, "a newer result replaces the older one")
}

// The fade timer of a banner that has already been replaced must not empty the
// slot underneath the banner that replaced it.
func TestClearFlashIgnoresAStaleTimer(t *testing.T) {
	u := testUI(t)

	u.flash("first", StatusGood)
	stale := u.flashSeq
	u.flash("second", StatusGood)

	u.clearFlash(stale)
	require.Len(t, u.flashes.Objects, 1, "the newer banner still owns the slot")

	u.clearFlash(u.flashSeq)
	require.Empty(t, u.flashes.Objects, "dismissing the current banner empties the slot")
}

// --- browse (copied from angou) --------------------------------------------

// Tapping Browse… must open a chooser, not take the process with it.
//
// The first version of this button in angou sized the dialog before showing it.
// In fyne 2.8.1 that path asks a dialog with no window yet for its minimum size
// and dereferences nil, so the crash was in the one interaction the button
// exists for. This drives the real widget under the test driver, which needs no
// display, so the regression cannot come back unnoticed.
func TestBrowseButtonOpensAChooser(t *testing.T) {
	u := testUI(t)
	for _, dir := range []bool{false, true} {
		field := widget.NewEntry()
		button := u.browseButton(field, dir)
		require.NotPanics(t, func() { test.Tap(button) }, "directory chooser: %v", dir)
	}
}

// The same for the two choosers the Mods toolbar opens, which take a callback
// rather than a field.
func TestModChoosersOpenWithoutPanicking(t *testing.T) {
	u := testUI(t)
	require.NotPanics(t, func() { u.chooseFile(luaFilter(), func(string) {}) })
	require.NotPanics(t, func() { u.chooseFolder("", func(string) {}) })
}

// A field holding a path that does not exist, or nothing at all, must still
// give the chooser somewhere to start rather than leaving it wherever the
// process happens to be.
func TestPickerStartFallsBackToHome(t *testing.T) {
	for _, text := range []string{"", "/nonexistent/path/for/a/test", "~"} {
		require.NotNil(t, pickerStart(text), "no starting location for %q", text)
	}
}

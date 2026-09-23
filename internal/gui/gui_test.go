package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/shell"
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

/*
testUI is a window with no window: the program's state over a headless shell,
with HOME, the XDG directories and the Steam root pointed at throwaway paths
so nothing touches the developer's install or their real configuration.
Sections built from it load inline (the shell is not on screen), so a test
sees finished state when the builder returns.
*/
func testUI(t *testing.T) *ui {
	t.Helper()
	fynetest.Sandbox(t)
	t.Setenv("STEAM_ROOT", filepath.Join(t.TempDir(), "no-steam-here"))
	app := test.NewApp()
	t.Cleanup(app.Quit)
	u := &ui{version: "test", commit: "0000000"}
	u.run.init()
	u.sh = shell.Headless(app, u.shellOptions(Options{}))
	// Headless has no window; the dialogs need one to hang off, and the tests
	// that drive them get this one. OnScreen stays false.
	win := test.NewWindow(widget.NewLabel(""))
	u.sh.Window = win
	t.Cleanup(win.Close)
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

// --- the shell's wiring ----------------------------------------------------

// The shell's banner slot is wired: a result shows, a newer one replaces it,
// and dismissing clears it. The timings and the fade are the library's.
func TestFlashShowsOneBannerAtATime(t *testing.T) {
	u := testUI(t)
	u.sh.Flash("first", fd.StatusGood)
	require.Equal(t, "first", u.sh.FlashText())
	u.sh.Flash("second", fd.StatusBad)
	require.Equal(t, "second", u.sh.FlashText(), "a newer result replaces the older one")
	u.sh.ClearFlash()
	require.Empty(t, u.sh.FlashText())
}

// --section opens the named section, case-insensitively; a typo opens the
// first rather than a dead window.
func TestSectionSelectionResolvesNamesAndFallsBackToTheFirst(t *testing.T) {
	u := testUI(t)
	require.Equal(t, "Overview", u.sh.Current().Title(), "no --section: the first")
	u.sh.Select("report")
	require.Equal(t, "Report", u.sh.Current().Title())
	// Select is navigation: a name that is not there leaves the reader where
	// they are rather than sending the window home (fynedesygn spec 028).
	u.sh.Select("nope")
	require.Equal(t, "Report", u.sh.Current().Title(), "a typo moved the navigation")

	// --section is a different question -- where to open -- and a typo there
	// opens the first section rather than a dead window.
	app := test.NewApp()
	t.Cleanup(app.Quit)
	typo := shell.Headless(app, u.shellOptions(Options{Section: "nope"}))
	require.Equal(t, "Overview", typo.Current().Title(), "--section nope opens the first")
	named := shell.Headless(app, u.shellOptions(Options{Section: "report"}))
	require.Equal(t, "Report", named.Current().Title(), "--section report opens Report")
}

// A saved appearance from the previous build is read unchanged: the keys are
// the ones that build wrote, so nobody loses their scheme on upgrade (spec
// 012 AC5).
func TestASavedAppearanceFromThePreviousBuildIsReadUnchanged(t *testing.T) {
	u := testUI(t)
	p := u.sh.App.Preferences()
	p.SetString("appearance.scheme", "Oxygen Dark")
	p.SetString("appearance.font", "Fyne default")
	p.SetFloat("appearance.textSize", 14)

	// The next launch: a shell built over the same preference store.
	u.sh = shell.Headless(u.sh.App, u.shellOptions(Options{}))
	th := u.sh.Appearance().Theme()
	require.Equal(t, "Oxygen Dark", th.Palette().Name)
	require.Equal(t, float32(14), th.TextSize())
	require.Equal(t, fdtheme.DefaultFontName, u.sh.Appearance().Font)
}

/*
Every section renders headlessly, before anything has loaded and in every
colour scheme.

The first half is the first few hundred milliseconds of every run: the loads
are still in flight and every builder must draw from zero values. The second
is the Appearance section's promise: a component that reads a palette role a
scheme does not carry would take the window down on the next click there.
*/
func TestEverySectionRendersHeadlesslyInEveryScheme(t *testing.T) {
	u := testUI(t)
	for _, s := range u.sh.Sections() {
		require.NotPanicsf(t, func() { _ = s.Build(u.sh) }, "nothing loaded: %s", s.Title())
	}
	for _, name := range fdtheme.SchemeNames() {
		a := u.sh.Appearance()
		a.Scheme = name
		u.sh.SetAppearance(a)
		for _, s := range u.sh.Sections() {
			require.NotPanicsf(t, func() { _ = s.Build(u.sh) }, "%s: %s", name, s.Title())
		}
	}
}

// The About section carries this program's facts, not the library's defaults.
func TestAboutNamesTheProgramAndItsFacts(t *testing.T) {
	u := testUI(t)
	text := fynetest.Text(u.buildAbout())
	require.Contains(t, text, "nmsbonker")
	require.Contains(t, text, "test (0000000)")
	require.Contains(t, text, "Ship only what compiles")
	require.Contains(t, text, "MIT")
	require.Contains(t, text, "no game found")
}

// The navigation's shape is the user's: titles with icons, icons alone, or
// hidden, down the left or along the top. A program declares what it allows
// and the library puts one control in the header offering exactly that; a
// program that declares nothing keeps the window it has, with no control and
// no shortcut, so this is what makes the feature reach the user at all.
func TestTheWindowOffersEveryNavigationShape(t *testing.T) {
	u := &ui{version: "test"}
	o := u.shellOptions(Options{})

	require.ElementsMatch(t,
		[]shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden}, o.NavModes)
	require.ElementsMatch(t,
		[]shell.NavPlacement{shell.NavLeft, shell.NavTop}, o.NavPlacements)
}

// Ten sections make the icons-only shapes worth having, and they are only
// usable because every section has an icon of its own: in them each icon
// carries its section's title as a hover tip, and a section without one gets a
// generic picture to guess at.
func TestEverySectionHasAnIconForTheIconsOnlyShapes(t *testing.T) {
	for title, b := range sectionBuilders() {
		require.NotNil(t, b.icon, "section %q has no icon", title)
		require.NotNil(t, b.icon(), "section %q resolves to no icon resource", title)
	}
}

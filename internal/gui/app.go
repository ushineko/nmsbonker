// Copied from angou (same author) — keep in sync by hand. The shell, the flash
// slot, the busy strip and the small shared widgets are angou's; the sections,
// the status bar's contents and the build log are this project's.

/*
Package gui is the desktop front end of spec 003.

The window renders internal/core and does nothing else: it holds no build logic
of its own and reaches no further than the core, which is the rule that keeps it
in step with the CLI (project rule: CLI/GUI parity).

Every core call runs off the UI thread and hops back with fyne.Do. Nothing
transient reflows the interface: result banners, the busy indicator and the
build's step markers all live in regions that keep their size whether or not
anything is in them.
*/
package gui

import (
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// section is one entry in the left navigation.
type section struct {
	title string
	icon  fyne.Resource
	build func(*ui) fyne.CanvasObject
}

// ui holds the window's widgets and everything it has loaded from the core.
type ui struct {
	app     fyne.App
	win     fyne.Window
	version string
	commit  string
	// configPath is the --config override, carried into every core request so
	// that the window and `nmsbonker --config …` read the same document.
	configPath string

	// appearance, persisted across runs. Held here so a change in one control
	// can rebuild the theme with the other two unchanged.
	scheme   string
	fontName string
	textSize float32

	content *container.Scroll
	nav     *widget.List
	frame   *fyne.Container // holds the status bar, so it can be redrawn
	current int             // the selected section, so an operation can rebuild it

	// busyCount is how many operations are running. A count rather than a flag:
	// loading a section can start more than one, and the indicator must not go
	// out when the first of them finishes.
	busyCount int
	busyWhat  string

	// Loaded from the core on a goroutine, read and written on the UI thread.
	//
	// Each has a companion flag rather than being tested for emptiness. A
	// section asks for its data when it has none and finishes by rebuilding
	// itself, so inferring "not loaded yet" from an empty slice makes an empty
	// library load forever. "Loaded and empty" and "not loaded" are different
	// states and need to be stored as such.
	status   core.StatusResult
	statusOK bool
	detect   core.DetectResult
	detectOK bool
	mods     core.ListModsResult
	modsOK   bool
	tweaks   core.ListTweaksResult
	tweaksOK bool
	saves    core.ListSaveBackupsResult
	savesOK  bool
	// slots is the Saves section's slot listing (spec 007 R8.1); slotsErr is
	// why it is empty when the listing failed, so the section reports it rather
	// than asking again on every rebuild. inspect is the save the editor card
	// shows, nil until a row is picked.
	slots    core.ListSaveSlotsResult
	slotsErr string
	slotsOK  bool
	inspect  *core.InspectSaveResult
	// draft holds what has been typed into the editor form but not yet
	// written, keyed by field label. The section is rebuilt whenever an
	// operation starts or stops (regate), and a rebuild that reset the form
	// would throw away the values Preview was just asked about.
	draft     map[string]string
	archive   core.ListArchiveResult
	archiveOK bool
	// checks are the per-script facts the Mods table's Author and Files columns
	// come from: what each .lua says about itself, loaded through the sandbox.
	checks       map[string]core.ModCheck
	checksOK     bool
	tools        core.ListToolsResult
	toolsOK      bool
	releases     core.ListReleasesResult
	releasesOK   bool
	cache        core.CacheInfoResult
	cacheOK      bool
	settings     core.ConfigShowResult
	configOK     bool
	lastReport   core.ReportResult
	lastReportOK bool
	/*
		compat is the round-trip compatibility check (spec 002 R3.4), which the
		Overview's Tools card reports.

		Run once per process rather than per section load, and in the background
		on first arrival at Overview. It starts two MBINCompiler processes and
		takes about two seconds, which is too slow to do on every navigation and
		far too useful to leave saying "not checked yet" -- that is what the
		card said for the whole of spec 003, on a machine where the answer was
		"compatible" and the build already knew it.
	*/
	compat      core.ToolCheckResult
	compatOK    bool
	compatError string
	// lastReportErr is why there is no report, which for a machine that has
	// never built is "no build yet" rather than a failure.
	lastReportErr string
	/*
		freshAudit is a re-check of the last build's reward amounts against the
		limits currently in force (spec 005 R3.1).

		Nil until the user asks, and then it outranks what the report file
		carries: the report records what the limits were when the build ran, and
		the whole point of Re-check is trying a different one. Cleared by
		invalidate() along with everything else loaded from the core.
	*/
	freshAudit *core.AuditResult

	// run is everything the Build section shows. It outlives the section, so a
	// build keeps streaming while the user reads the Report.
	run buildRun

	// flashes is the result-banner slot: one banner at a time, in a region of
	// the window that keeps its height whether or not anything is in it.
	flashes  *fyne.Container
	flashSeq int // identifies the banner that owns the slot, so a stale timer cannot clear a newer one
}

// Preference keys. Namespaced so a later setting cannot collide with one of
// these by accident.
const (
	prefScheme = "appearance.scheme"
	prefFont   = "appearance.font"
	prefSize   = "appearance.textSize"
)

// loadAppearance reads the saved appearance, falling back to the defaults. A
// stale value — a scheme that was renamed, a font since uninstalled — falls back
// rather than failing: paletteByName and loadFont both tolerate an unknown name.
func (u *ui) loadAppearance() {
	p := u.app.Preferences()
	u.scheme = p.StringWithFallback(prefScheme, palettes[0].name)
	u.fontName = p.StringWithFallback(prefFont, defaultFontName)
	u.textSize = float32(p.FloatWithFallback(prefSize, float64(defaultTextSize)))
}

// applyAppearance rebuilds the theme from the current settings and saves them.
func (u *ui) applyAppearance() {
	p := u.app.Preferences()
	p.SetString(prefScheme, u.scheme)
	p.SetString(prefFont, u.fontName)
	p.SetFloat(prefSize, float64(u.textSize))

	u.app.Settings().SetTheme(kdeTheme{
		p:    paletteByName(u.scheme),
		font: loadFont(u.fontName),
		text: u.textSize,
	})
}

// sectionTitles is the navigation in order.
//
// The order and the names live here, apart from the icons, because a theme icon
// cannot be constructed before an app exists: asking for one first makes Fyne
// log "Attempt to access current Fyne app when none is started", which is what
// angou-gui --version printed seven times, since the flag help lists these.
//
// This list plus sectionBuilders is the registry spec 004 adds Tweaks to. A
// title with no builder draws nothing, so the two are kept in step by
// TestSectionNamesNeedsNoApp rather than by memory.
var sectionTitles = []string{
	"Overview", "Mods", "Tweaks", "Build", "Report", "Saves", "Tools", "Settings", "Appearance", "About",
}

// sectionBuilders is what each section is made of. sections() walks
// sectionTitles and looks each one up here, so a title with no builder is a
// missing section rather than a silently different list from the one --section
// is told about.
//
// A function rather than a package variable: the builders reach back to
// sections() when a section rebuilds itself, and Go reports that as an
// initialization cycle in a package-level map.
func sectionBuilders() map[string]struct {
	icon  func() fyne.Resource
	build func(*ui) fyne.CanvasObject
} {
	return map[string]struct {
		icon  func() fyne.Resource
		build func(*ui) fyne.CanvasObject
	}{
		"Overview":   {theme.HomeIcon, (*ui).buildOverview},
		"Mods":       {theme.ListIcon, (*ui).buildMods},
		"Tweaks":     {theme.SettingsIcon, (*ui).buildTweaks},
		"Build":      {theme.MediaPlayIcon, (*ui).buildBuild},
		"Report":     {theme.DocumentIcon, (*ui).buildReport},
		"Saves":      {theme.StorageIcon, (*ui).buildSaves},
		"Tools":      {theme.ComputerIcon, (*ui).buildTools},
		"Settings":   {theme.SettingsIcon, (*ui).buildSettings},
		"Appearance": {theme.ColorPaletteIcon, (*ui).buildAppearance},
		"About": {theme.HelpIcon, func(u *ui) fyne.CanvasObject {
			return u.buildAbout(u.version, u.commit)
		}},
	}
}

func sections() []section {
	builders := sectionBuilders()
	out := make([]section, 0, len(sectionTitles))
	for _, title := range sectionTitles {
		b, ok := builders[title]
		if !ok {
			continue // a title with no builder draws nothing; see SectionNames
		}
		out = append(out, section{title: title, icon: b.icon(), build: b.build})
	}
	return out
}

// SectionNames lists the navigation entries, for --section and for a capture
// script to iterate.
//
// Reads the titles rather than building the sections: this is called while
// parsing flags, before there is an app to hang an icon on.
func SectionNames() []string { return append([]string(nil), sectionTitles...) }

// SchemeNames lists the colour schemes, for the same reason.
func SchemeNames() []string { return paletteNames() }

// Options configure a run. Section and Scheme exist so a capture script can
// deep-link into the window: refreshing the README set otherwise means clicking
// through every section against a timer, and a screenshot that is tedious to
// refresh is a screenshot that goes stale. They override the saved appearance
// for that run without saving over it.
type Options struct {
	Version string
	Commit  string
	// ConfigPath is the --config override; empty uses the default document.
	ConfigPath string
	Section    string // navigation entry to open on; empty means the first
	Scheme     string // color scheme to force; empty means the saved one
}

// Run opens the window and blocks until it is closed.
func Run(o Options) {
	// Before the toolkit starts: GLFW reads the cursor theme from the
	// environment at init, and there is no second chance once the window is up.
	applyCursorTheme()

	// The ID gives the app a preferences store, which Fyne writes under the
	// user's config directory. That file holds the appearance settings and
	// nothing else: every setting the CLI can also see lives in config.json,
	// so that `nmsbonker status` in a terminal and this window agree.
	a := app.NewWithID("io.ushineko.nmsbonker")
	u := &ui{app: a, version: o.Version, commit: o.Commit, configPath: o.ConfigPath}
	u.run.init()
	u.win = a.NewWindow("nmsbonker " + o.Version)
	u.win.SetIcon(appIcon())
	a.SetIcon(appIcon())
	u.loadAppearance()
	if o.Scheme != "" {
		// Forced for this run only, so a capture does not overwrite whatever
		// the user had chosen.
		u.scheme = paletteByName(o.Scheme).name
		u.app.Settings().SetTheme(kdeTheme{
			p: paletteByName(u.scheme), font: loadFont(u.fontName), text: u.textSize,
		})
	} else {
		u.applyAppearance()
	}

	u.content = container.NewScroll(widget.NewLabel(""))
	u.flashes = container.NewVBox()
	secs := sections()

	u.nav = widget.NewList(
		func() int { return len(secs) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.HomeIcon()), widget.NewLabel("placeholder"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*fyne.Container)
			row.Objects[0].(*widget.Icon).SetResource(secs[i].icon)
			row.Objects[1].(*widget.Label).SetText(secs[i].title)
		},
	)
	u.nav.OnSelected = func(i widget.ListItemID) {
		u.current = i
		u.swap(secs[i].build)
	}

	split := container.NewHSplit(u.nav, u.content)
	split.SetOffset(0.16)

	// Result banners get their own region above the status bar, and that region
	// keeps its height whether or not a banner is in it.
	//
	// Both of the obvious alternatives are worse. Stacked above the content,
	// every banner pushed the whole section down — the row under the pointer
	// moved out from under it, which is jarring in a window whose buttons
	// include Remove. Floated over the content, nothing reflowed but the banner
	// covered whatever was at the bottom of the section, which on Build is the
	// Cancel button.
	//
	// The reserved height is the price. The banner scrolls inside the slot
	// rather than growing it, so a long message cannot reflow the window either.
	u.frame = container.NewVBox(
		fixedHeight(container.NewVScroll(u.flashes), flashSlotHeight),
		u.statusBar(),
	)
	u.win.SetContent(container.NewBorder(u.header(), u.frame, nil, nil, split))
	// F5 and Ctrl+R reload, the two bindings people already try. The library is
	// a plain directory and the game is updated by Steam behind our back, so
	// "show me what is actually there" needs to be one keystroke rather than a
	// hunt for a button.
	for _, sc := range []fyne.Shortcut{
		&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl},
	} {
		u.win.Canvas().AddShortcut(sc, func(fyne.Shortcut) { u.invalidate() })
	}
	u.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == fyne.KeyF5 {
			u.invalidate()
		}
	})

	u.win.Resize(fyne.NewSize(1180, 760))
	u.nav.Select(sectionIndex(secs, o.Section))
	// The status bar names the game, the compiler and the mod library from every
	// section, so both are fetched here rather than left to Overview. Left to a
	// section, the bar read "reading…" and "—" everywhere else.
	u.loadStatus()
	u.loadMods()
	u.win.SetMaster()
	u.win.ShowAndRun()
}

// sectionIndex resolves a section name to its position. An unknown name opens
// the first section rather than failing: a typo in a capture script should
// produce a wrong screenshot, which is obvious, not a dead window.
func sectionIndex(secs []section, name string) int {
	if name == "" {
		return 0
	}
	for i, s := range secs {
		if strings.EqualFold(s.title, name) {
			return i
		}
	}
	return 0
}

// selectSection moves the navigation to a named section. Used by the buttons
// that hand the user on — "View report" after a build, "Install MBINCompiler"
// pointing at Tools.
func (u *ui) selectSection(name string) {
	if u.nav == nil {
		return
	}
	u.nav.Select(sectionIndex(sections(), name))
}

// rebuild redraws the whole window, including the status bar, which names the
// game and the compiler. refresh alone only replaces the content pane.
func (u *ui) rebuild() {
	if u.frame != nil {
		u.frame.Objects[frameStatusBar] = u.statusBar()
		u.frame.Refresh()
	}
	u.refresh()
}

// refresh rebuilds the current section, so a view showing the mod list picks up
// what an operation just changed. Called on the UI thread.
//
// A nil content pane means there is no window to draw into: a headless test, or
// a load that finished after the window closed. Both are ordinary, and building
// a section for nobody would start the loads that section asks for.
func (u *ui) refresh() {
	if u.content == nil {
		return
	}
	secs := sections()
	if u.current >= 0 && u.current < len(secs) {
		u.swap(secs[u.current].build)
	}
}

/*
swap replaces the content pane with a freshly built section.

The live build widgets are dropped before the new section is built, not after.
Built first and dropped afterwards -- which is what this did, inside show() --
the Build section registered its brand-new log list and step rows and then had
them thrown away by the very call that put them on screen, so a running build
streamed into nothing.
*/
func (u *ui) swap(build func(*ui) fyne.CanvasObject) {
	if u.content == nil {
		return
	}
	u.run.detach()
	u.show(build(u))
}

func (u *ui) show(o fyne.CanvasObject) {
	if u.content == nil {
		return
	}
	u.content.Content = o
	u.content.Refresh()
	u.content.ScrollToTop()
}

// header is the window's title strip. It carries Refresh because the two
// things this window describes — the game install and the mod library — are
// changed by Steam and by a file manager without telling us.
func (u *ui) header() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("nmsbonker", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	reload := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.invalidate() })
	bar := container.NewHBox(title, layout.NewSpacer(), reload)
	return container.NewVBox(container.NewPadded(bar), widget.NewSeparator())
}

// statusBar is R3: the game, the compiler, the library and the output folder,
// then the busy strip at the right-hand end.
func (u *ui) statusBar() fyne.CanvasObject {
	game := statusText("not found", StatusBad)
	if !u.statusOK {
		game = statusText("reading…", StatusInfo)
	} else if u.status.Install.Found {
		game = statusText(orNone(u.status.Install.BuildID, "no buildid"), StatusGood)
	}

	compiler := statusText("none", StatusBad)
	if !u.statusOK {
		compiler = statusText("reading…", StatusInfo)
	} else if u.status.Compiler.Installed {
		compiler = statusText(u.status.Compiler.Tag, compatStatus(u.status.Compatibility))
	}

	enabled, total := 0, len(u.mods.Mods)
	for _, m := range u.mods.Mods {
		if m.Enabled {
			enabled++
		}
	}
	modsText := fmt.Sprintf("%d/%d", enabled, total)
	if !u.modsOK {
		modsText = "—"
	}

	output := config.DefaultModName
	if u.statusOK && u.status.ModName != "" {
		output = u.status.ModName
	}

	// One HBox, laid out left to right, with a spacer pushing the busy slot to
	// the right-hand end. A border layout's centre region is sized from what is
	// left over rather than from the slot's own width, so a long compiler
	// version would push the two into each other. Sequential layout cannot
	// overlap.
	bar := container.NewHBox(
		dim("game"), game, sep(),
		dim("compiler"), compiler, sep(),
		dim("mods"), widget.NewLabel(modsText), sep(),
		dim("output"), widget.NewLabel(output),
		layout.NewSpacer(),
		u.busyStrip(),
	)
	return container.NewVBox(widget.NewSeparator(), container.NewPadded(bar))
}

// busyStrip is the right-hand end of the status bar: what is running, and a bar
// that says it is still running.
//
// It occupies the same height whether or not anything is happening. A slot that
// only exists while busy would resize the status bar as operations start and
// finish, which is the same reflow this placement exists to avoid, just at the
// other end of the window.
func (u *ui) busyStrip() fyne.CanvasObject {
	if u.busyCount == 0 {
		spacer := canvas.NewRectangle(nil)
		spacer.SetMinSize(fyne.NewSize(0, busyStripHeight))
		return spacer
	}

	label := widget.NewLabel(u.busyWhat)
	label.Truncation = fyne.TextTruncateEllipsis
	label.Alignment = fyne.TextAlignTrailing

	// Both halves are pinned to a width. An HBox hands a truncating label its
	// minimum size, which for a truncating label is nothing at all — the text
	// collapsed to an ellipsis and the indicator said nothing about what was
	// running. A progress bar left to itself has the opposite problem and
	// expands into whatever room the text beside it leaves.
	labelSlot := canvas.NewRectangle(nil)
	labelSlot.SetMinSize(fyne.NewSize(busyLabelWidth, busyStripHeight))

	barSlot := canvas.NewRectangle(nil)
	barSlot.SetMinSize(fyne.NewSize(120, busyStripHeight))

	return container.NewHBox(
		container.New(layout.NewStackLayout(), labelSlot, label),
		container.New(layout.NewStackLayout(), barSlot, widget.NewProgressBarInfinite()),
	)
}

// busyStripHeight keeps the status bar the same height whether or not something
// is running. busyLabelWidth is how much room the description gets: enough for a
// short phrase, and fixed so that starting an operation does not shuffle the
// rest of the bar sideways.
const (
	busyStripHeight = 18
	busyLabelWidth  = 260
)

/*
busy shows an indeterminate progress indicator until the returned function is
called.

Indeterminate on purpose for everything except the build, which has a step list
of its own: a directory scan does not know how many files it will walk until it
has walked them, and a GitHub round trip is one long step rather than many short
ones. A bar that filled steadily would be inventing a number.

Safe to call from a goroutine: it hops to the UI thread itself, and so does the
function it returns. Callers are core operations running off the UI thread, so
requiring them to marshal by hand would be an invitation to forget.

EVERY core call gets one. Not just the ones that are obviously slow: a cold pak
index reads 97 archives, `tools ensure` downloads a release, and the machine
this runs on is not the machine it was written on. A window that sits still with
no explanation reads as frozen, and the button that looks like it did nothing is
the button that gets clicked twice.
*/
func (u *ui) busy(what string) func() {
	fyne.Do(func() {
		u.busyCount++
		u.busyWhat = what
		u.redrawStatus()
		if u.busyCount == 1 {
			u.regate()
		}
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			fyne.Do(func() {
				u.busyCount--
				if u.busyCount <= 0 {
					u.busyCount, u.busyWhat = 0, ""
					u.regate()
				}
				u.redrawStatus()
			})
		})
	}
}

/*
regate rebuilds the current section when work starts and when it stops.

Every section disables the buttons that start work while something is running,
and it does that as it is built -- so a section built while an operation was in
flight comes out with dead buttons and nothing turns them back on. That is not
hypothetical: the two loads this window starts at launch are still running when
the first section is drawn, so the Build toolbar came up permanently disabled
and the window looked finished while refusing to do anything.

Rebuilding from state is the fix rather than walking a list of buttons, because
"disabled" has several causes at once -- no game found, no row selected, the
active compiler cannot be removed -- and only the builder knows all of them.
*/
func (u *ui) regate() {
	u.refresh()
	u.drawControls()
}

// working reports whether a core operation is in flight. R4.2: one at a time,
// with the other buttons disabled rather than hidden, so the window does not
// change shape as work starts and finishes.
func (u *ui) working() bool { return u.busyCount > 0 || u.run.running }

// redrawStatus repaints the status bar and nothing else.
//
// This is the whole reason progress lives down there. An indicator inserted
// above the content pushes everything below it down — the row the pointer is
// over moves out from under the pointer, mid-click, which is jarring in a
// window and dangerous in one whose buttons include Deploy.
func (u *ui) redrawStatus() {
	if u.frame == nil {
		return
	}
	u.frame.Objects[frameStatusBar] = u.statusBar()
	u.frame.Refresh()
}

// The banner slot's geometry and timings.
const (
	// frameStatusBar is the status bar's position in u.frame, which also holds
	// the banner slot above it.
	frameStatusBar = 1
	// flashSlotHeight reserves room for one banner. Reserved whether or not one
	// is showing, so nothing moves when one arrives — which is the point, and
	// also the cost: this is height the section below never gets back. A banner
	// measures 44 at its minimum, so this is that plus a little air, and the
	// rare message long enough to wrap scrolls inside the slot.
	flashSlotHeight = 48
	// How long a banner stays before it starts fading. A warning gets longer
	// because it usually names a condition to act on.
	flashHoldGood = 6 * time.Second
	flashHoldWarn = 12 * time.Second
	// The fade itself. Long enough to read as intentional, short enough that
	// the banner is not sitting there half-gone.
	flashFade = 700 * time.Millisecond
)

// flashHold says how long a banner of this status stays up, and whether it goes
// on its own at all.
//
// A failure does not: it waits to be dismissed, or until another operation
// replaces it. An error that removes itself on a timer is an error nobody read,
// and the operation it describes has already not happened.
func flashHold(st Status) (time.Duration, bool) {
	switch st {
	case StatusBad:
		return 0, false
	case StatusWarn:
		return flashHoldWarn, true
	default:
		return flashHoldGood, true
	}
}

/*
flash reports the result of an operation as a banner in the slot above the
status bar. One banner shows at a time: a newer result replaces an older one
rather than stacking, so the slot cannot overflow and the most recent thing that
happened is always the thing on screen.

Fyne animates properties, not opacity: a widget has no alpha to fade. So the
fade is on the banner's own background rectangle, whose colour animates from the
status tint to fully transparent. The text is left at full strength for the whole
life of the banner, which is the accessible choice anyway.

Build output does not come through here. A build emits hundreds of lines and one
banner per warning would be a slot flickering for three minutes; the log pane is
where those go, and one banner summarises the result (R4.1).
*/
func (u *ui) flash(text string, st Status) {
	u.flashSeq++
	seq := u.flashSeq

	tint := u.flashTint(st)
	bg := canvas.NewRectangle(tint)
	bg.CornerRadius = 2

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord

	// Dismissable, because a banner that only leaves on a timer leaves either
	// too early to read or too late to be rid of. This is also the only way to
	// clear a failure, which does not go on its own.
	dismiss := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { u.clearFlash(seq) })
	dismiss.Importance = widget.LowImportance

	banner := container.NewStack(bg, container.NewPadded(
		container.NewBorder(nil, nil, marker(st), dismiss, label)))
	u.flashes.Objects = []fyne.CanvasObject{banner}
	u.flashes.Refresh()

	hold, fades := flashHold(st)
	if !fades || !u.onScreen() {
		// Nothing to fade with no window: the timer would come back seconds
		// later to animate a rectangle nobody is drawing, on a goroutine the
		// test that made it has long since finished with.
		return
	}

	transparent := color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: 0}
	go func() {
		time.Sleep(hold)
		fyne.Do(func() {
			if u.flashSeq != seq {
				return // a newer banner owns the slot
			}
			fade := canvas.NewColorRGBAAnimation(tint, transparent, flashFade, func(c color.Color) {
				bg.FillColor = c
				canvas.Refresh(bg)
			})
			fade.Curve = fyne.AnimationEaseIn
			fade.Start()
		})
		time.Sleep(flashFade)
		fyne.Do(func() { u.clearFlash(seq) })
	}()
}

// clearFlash empties the slot, unless a newer banner has taken it. Called from
// the dismiss button and from the fade's own timer, which may arrive after the
// banner it belongs to has already been replaced.
func (u *ui) clearFlash(seq int) {
	if u.flashSeq != seq {
		return
	}
	u.flashes.Objects = nil
	u.flashes.Refresh()
}

// flashTint is the banner's starting colour: the status role from the active
// scheme, at low alpha so text stays readable over it in all five schemes.
func (u *ui) flashTint(st Status) color.NRGBA {
	p := paletteByName(u.scheme)
	var c color.Color
	switch st {
	case StatusGood:
		c = p.positive
	case StatusWarn:
		c = p.neutral
	case StatusBad:
		c = p.negative
	default:
		c = p.selectionBG
	}
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0x4d} //nolint:gosec // 16-bit channels; >>8 fits a byte
}

// --- small shared widgets -------------------------------------------------

func dim(s string) fyne.CanvasObject {
	l := widget.NewLabel(s)
	l.Importance = widget.LowImportance
	return l
}

func sep() fyne.CanvasObject { return widget.NewLabel("·") }

// statusText colours a value by its status. The colour is drawn from the active
// scheme's negative/positive/neutral roles, so it stays legible in all five.
func statusText(s string, st Status) fyne.CanvasObject {
	l := widget.NewLabel(s)
	switch st {
	case StatusGood:
		l.Importance = widget.SuccessImportance
	case StatusWarn:
		l.Importance = widget.WarningImportance
	case StatusBad:
		l.Importance = widget.DangerImportance
	}
	return l
}

// marker is the icon that ranks a row at a glance.
func marker(s Status) fyne.CanvasObject {
	switch s {
	case StatusGood:
		return widget.NewIcon(theme.ConfirmIcon())
	case StatusWarn:
		return widget.NewIcon(theme.WarningIcon())
	case StatusBad:
		return widget.NewIcon(theme.ErrorIcon())
	}
	return widget.NewIcon(theme.InfoIcon())
}

// wrapped is a paragraph that reflows rather than running off the edge. Used
// for the sentences in dialogs, which are the ones that state consequences.
func wrapped(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func heading(title, blurb string) fyne.CanvasObject {
	h := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	b := widget.NewLabel(blurb)
	b.Wrapping = fyne.TextWrapWord
	b.Importance = widget.LowImportance
	return container.NewVBox(h, b, widget.NewSeparator())
}

// action renders one operation as a titled block with its consequences stated,
// rather than as a bare button. The GUI has room the flag list does not.
func action(title, blurb, button string, danger bool, tapped func()) (fyne.CanvasObject, *widget.Button) {
	t := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	b := widget.NewLabel(blurb)
	b.Wrapping = fyne.TextWrapWord
	b.Importance = widget.LowImportance
	btn := widget.NewButton(button, tapped)
	if danger {
		btn.Importance = widget.DangerImportance
	}
	// Padded so the button does not sit flush against the window edge; the
	// border layout gives the trailing object exactly its minimum width.
	row := container.NewBorder(nil, nil, nil, container.NewPadded(container.NewVBox(btn)),
		container.NewVBox(t, b))
	return row, btn
}

// card is a titled block of facts with a rule under the title. Overview and
// Tools are both made of these.
func card(title string, body ...fyne.CanvasObject) fyne.CanvasObject {
	head := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewVBox(append([]fyne.CanvasObject{head, widget.NewSeparator()}, body...)...)
}

// factRow is one "label   value" line with an optional status marker, the shape
// the CLI prints as `label: value`.
func factRow(label, value string, st Status) fyne.CanvasObject {
	l := widget.NewLabel(label)
	l.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, fixedWidth(l, 190), nil,
		container.NewHBox(marker(st), statusText(value, st)))
}

// rowWithAction puts one button on a fact row's trailing edge, for a fact that
// names a place the desktop can open. The button is always drawn and disables
// with the fact behind it rather than appearing and disappearing, so the card
// keeps its shape when the state changes.
func rowWithAction(row fyne.CanvasObject, btn *widget.Button) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil,
		container.NewPadded(container.NewVBox(btn)), row)
}

// plainRow is factRow without the marker, for facts that carry no verdict. The
// marker column is still reserved so the values line up with the ranked rows
// above and below them.
func plainRow(label, value string) fyne.CanvasObject {
	l := widget.NewLabel(label)
	l.Importance = widget.LowImportance
	v := widget.NewLabel(value)
	v.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, fixedWidth(l, 190), nil,
		container.NewBorder(nil, nil, fixedWidth(widget.NewLabel(""), 26), nil, v))
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func humanAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// orNone substitutes a stand-in for an empty value, so a blank cell never reads
// as a rendering fault.
func orNone(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

/*
Actions is every operation this window can reach, named by the CLI command it
corresponds to.

It exists for the parity test in tests/parity: that test walks the cobra command
tree and this list and fails when either holds an operation the other does not.
Adding a command without a GUI affordance breaks the build, which is the point —
the two front ends drift silently otherwise, and a rule enforced only by review
is a rule that lasts until the first busy week.

A name here is a claim that the operation is reachable and wired, not that a
button exists. Do not add one to quiet the test.
*/
func Actions() []string {
	return []string{
		// Overview
		"status", // the install, tools and library cards, and the status bar
		"detect", // the "where nmsbonker looked" block, shown when no game is found
		// Mods
		"mods list", "mods add", "mods import", "mods remove",
		"mods enable", "mods disable", "mods move", "mods check",
		// Tweaks
		"tweaks list", "tweaks set", "tweaks reset", "tweaks enable", "tweaks disable",
		// Build, Report and what undoes them
		"build", "deploy", "report",
		"audit", // the Report section's amount-audit block and its Re-check button
		"undeploy", "rollback", "archive list",
		"mods-off", "mods-on",
		"saves backup", "saves list",
		// Saves (spec 007 R8)
		"saves slots", "saves inspect", "saves export", "saves import", "saves edit",
		// Tools
		"tools ensure", "tools list", "tools check", "tools pin", "tools unpin",
		"tools releases", "tools remove",
		"cache show", "cache clear",
		"pak list", "pak find", "pak extract", "pak reindex",
		// Settings
		"config show", "config set",
	}
}

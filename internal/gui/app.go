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
	fd "github.com/ushineko/fynedesygn"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

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

	// appearance, persisted across runs through the library's preference keys.
	appearance fdtheme.Appearance

	content *container.Scroll
	nav     *widget.List
	frame   *fyne.Container // holds the status bar, so it can be redrawn
	current int             // the selected section, so an operation can rebuild it

	// busyCount is how many operations are running. A count rather than a flag:
	// loading a section can start more than one, and the indicator must not go
	// out when the first of them finishes.
	busyCount int
	busyWhat  string
	// busyPop is the centred progress popup, up while busyCount > 0 once the
	// operation has run longer than busyPopDelay; busyLabel is its text.
	busyPop   *widget.PopUp
	busyLabel *widget.Label
	busySeq   int
	// flashPop carries the result banner over the content.
	flashPop *widget.PopUp

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
	draft map[string]string
	// reinspect is a save to read again once the operation that wrote it has
	// finished. Set inside that operation, acted on by the rebuild its end
	// triggers: starting a second core call from inside the first trips the
	// one-at-a-time guard.
	reinspect *core.SlotSelector
	// savesTab is the Saves section's selected tab, kept across rebuilds.
	savesTab int
	// rawPath, rawNode and rawText are the Raw JSON tab's browser: the path
	// being looked at, what core returned for it, and the text as edited (a
	// draft, kept across the rebuilds an operation causes).
	rawPath string
	rawNode *core.SaveNodeResult
	rawText string
	// shipIndex is the ship the editor's Starship group shows; -1 is the one
	// being flown.
	shipIndex int
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

// loadAppearance reads the saved appearance, falling back to the defaults. A
// stale value (a scheme that was renamed, a font since uninstalled) falls back
// rather than failing: the library tolerates an unknown name.
func (u *ui) loadAppearance() {
	u.appearance = fdtheme.LoadAppearance(u.app.Preferences())
}

// applyAppearance rebuilds the theme from the current settings and saves them.
func (u *ui) applyAppearance() { u.appearance.Apply(u.app) }

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
func SchemeNames() []string { return fdtheme.SchemeNames() }

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
	fdtheme.ApplyCursorTheme()

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
		u.appearance.Scheme = fdtheme.SchemeByName(o.Scheme).Name
		u.app.Settings().SetTheme(u.appearance.Theme())
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
		u.swap(secs[i].build, false)
	}

	split := container.NewHSplit(u.nav, u.content)
	split.SetOffset(0.16)

	// Result banners and the progress indicator float over the content as
	// popups (see flash and busy), so nothing below the header reflows when
	// an operation starts, finishes or reports. The frame is the status bar.
	u.frame = container.NewVBox(u.statusBar())
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
		u.swap(secs[u.current].build, true)
	}
}

/*
swap replaces the content pane with a freshly built section.

The live build widgets are dropped before the new section is built, not after.
Built first and dropped afterwards -- which is what this did, inside show() --
the Build section registered its brand-new log list and step rows and then had
them thrown away by the very call that put them on screen, so a running build
streamed into nothing.

keepScroll is for a rebuild of the section already on screen: every operation
rebuilds it when it starts and when it stops (regate), and a rebuild that
scrolled to the top threw the reader away from the slider they had just moved.
Navigating to a section starts at its top.
*/
func (u *ui) swap(build func(*ui) fyne.CanvasObject, keepScroll bool) {
	if u.content == nil {
		return
	}
	u.run.detach()
	u.show(build(u), keepScroll)
}

func (u *ui) show(o fyne.CanvasObject, keepScroll bool) {
	if u.content == nil {
		return
	}
	offset := u.content.Offset
	u.content.Content = o
	u.content.Refresh()
	if !keepScroll {
		u.content.ScrollToTop()
		return
	}
	u.content.Offset = offset
	u.content.Refresh()
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
	game := widgets.StatusText("not found", fd.StatusBad)
	if !u.statusOK {
		game = widgets.StatusText("reading…", fd.StatusInfo)
	} else if u.status.Install.Found {
		game = widgets.StatusText(widgets.OrNone(u.status.Install.BuildID, "no buildid"), fd.StatusGood)
	}

	compiler := widgets.StatusText("none", fd.StatusBad)
	if !u.statusOK {
		compiler = widgets.StatusText("reading…", fd.StatusInfo)
	} else if u.status.Compiler.Installed {
		compiler = widgets.StatusText(u.status.Compiler.Tag, compatStatus(u.status.Compatibility))
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

	bar := container.NewHBox(
		widgets.Dim("game"), game, widgets.Sep(),
		widgets.Dim("compiler"), compiler, widgets.Sep(),
		widgets.Dim("mods"), widget.NewLabel(modsText), widgets.Sep(),
		widgets.Dim("output"), widget.NewLabel(output),
	)
	return container.NewVBox(widget.NewSeparator(), container.NewPadded(bar))
}

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
		u.showBusy()
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
					u.hideBusy()
					u.regate()
				}
			})
		})
	}
}

// busyPopDelay is how long an operation runs before the progress popup
// appears. Most operations finish inside it, and a popup that blinks for a
// tenth of a second on every click is worse than none.
const busyPopDelay = 300 * time.Millisecond

/*
showBusy puts the progress popup up, centred and modal, once the operation has
lasted long enough to deserve one.

Modal on purpose: the window runs one operation at a time (R4.2) and every
button is disabled while one runs, so a popup that also swallows clicks changes
nothing about what can be done, and says plainly why the window is not
answering. It names the operation, in the words the caller of busy gave it.
*/
func (u *ui) showBusy() {
	if !u.onScreen() {
		return
	}
	if u.busyPop != nil {
		u.busyLabel.SetText(u.busyWhat)
		return
	}
	u.busySeq++
	seq := u.busySeq
	go func() {
		time.Sleep(busyPopDelay)
		fyne.Do(func() {
			if u.busySeq != seq || u.busyCount == 0 || u.busyPop != nil {
				return
			}
			u.busyLabel = widget.NewLabel(u.busyWhat)
			u.busyLabel.Alignment = fyne.TextAlignCenter
			bar := widget.NewProgressBarInfinite()
			body := container.NewPadded(container.NewVBox(u.busyLabel, widgets.FixedWidth(bar, 320)))
			u.busyPop = widget.NewModalPopUp(body, u.win.Canvas())
			u.busyPop.Show()
		})
	}()
}

// hideBusy takes the progress popup down.
func (u *ui) hideBusy() {
	u.busySeq++ // a pending showBusy timer finds a different sequence and stops
	if u.busyPop != nil {
		u.busyPop.Hide()
		u.busyPop, u.busyLabel = nil, nil
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

// The banner's geometry and timings.
const (
	// frameStatusBar is the status bar's position in u.frame.
	frameStatusBar = 0
	// flashWidth is how wide a banner is drawn, so a long message wraps
	// rather than spanning the window.
	flashWidth = 720
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
func flashHold(st fd.Status) (time.Duration, bool) {
	switch st {
	case fd.StatusBad:
		return 0, false
	case fd.StatusWarn:
		return flashHoldWarn, true
	default:
		return flashHoldGood, true
	}
}

/*
flash reports the result of an operation as a banner floated over the bottom of
the content, centred. One banner shows at a time: a newer result replaces an
older one rather than stacking, so the most recent thing that happened is always
the thing on screen, and nothing in the section behind it moves.

Fyne animates properties, not opacity: a widget has no alpha to fade. So the
fade is on the banner's own background rectangle, whose colour animates from the
status tint to fully transparent. The text is left at full strength for the whole
life of the banner, which is the accessible choice anyway.

Build output does not come through here. A build emits hundreds of lines and one
banner per warning would be a slot flickering for three minutes; the log pane is
where those go, and one banner summarises the result (R4.1).
*/
func (u *ui) flash(text string, st fd.Status) {
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
		container.NewBorder(nil, nil, widgets.Marker(st), dismiss, label)))
	u.flashes.Objects = []fyne.CanvasObject{banner}
	u.flashes.Refresh()
	u.showFlashPop()

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
	if u.flashPop != nil {
		u.flashPop.Hide()
	}
}

// showFlashPop floats the banner over the content, centred, a little above the
// status bar. Not modal: a result is something to read, not something to
// answer, and the section behind it stays usable.
func (u *ui) showFlashPop() {
	if !u.onScreen() {
		return
	}
	c := u.win.Canvas()
	if u.flashPop == nil {
		u.flashPop = widget.NewPopUp(widgets.FixedWidth(u.flashes, flashWidth), c)
	}
	cs := c.Size()
	width := min(float32(flashWidth), cs.Width-40)
	u.flashPop.Content = widgets.FixedWidth(u.flashes, width)
	size := u.flashPop.Content.MinSize()
	pos := fyne.NewPos((cs.Width-size.Width)/2, cs.Height-size.Height-56)
	u.flashPop.ShowAtPosition(pos)
}

// flashTint is the banner's starting colour: the status role from the active
// scheme, at low alpha so text stays readable over it in every scheme.
func (u *ui) flashTint(st fd.Status) color.NRGBA {
	p := fdtheme.SchemeByName(u.appearance.Scheme)
	var c color.Color
	switch st {
	case fd.StatusGood:
		c = p.Positive
	case fd.StatusWarn:
		c = p.Neutral
	case fd.StatusBad:
		c = p.Negative
	case fd.StatusInfo:
		c = p.SelectionBG
	}
	tint, _ := fdtheme.Alpha(c, 0x4d).(color.NRGBA)
	return tint
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
		"mods show", "mods write", // the script editor dialog (spec 010)
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
		"saves get", "saves set", // the Raw JSON tab's browser and editor (spec 009)
		// Tools
		"tools ensure", "tools list", "tools check", "tools pin", "tools unpin",
		"tools releases", "tools remove",
		"cache show", "cache clear",
		"pak list", "pak find", "pak extract", "pak reindex",
		// Settings
		"config show", "config set",
	}
}

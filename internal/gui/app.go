/*
Package gui is the desktop front end of spec 003.

The window renders internal/core and does nothing else: it holds no build logic
of its own and reaches no further than the core, which is the rule that keeps it
in step with the CLI (project rule: CLI/GUI parity).

The window itself (navigation, content pane, status bar, busy indicator and
result banners) is fynedesygn's shell; this package supplies the sections, the
status bar's segments and the build run. Every core call runs off the UI thread
and hops back with fyne.Do. Nothing transient reflows the interface: result
banners and the progress indicator float over the content as popups, and the
build's step list and log pane are fixed-size regions written to in place.
*/
package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/core"
)

// appID names the preference store and, on Wayland, the window's app_id,
// which the compositor matches to the desktop entry of the same basename.
const appID = "io.ushineko.nmsbonker"

// ui holds the program's state and the shell that draws it.
type ui struct {
	// sh is the window: navigation, content pane, status bar, busy indicator
	// and banners. The shell stores itself here through Options.OnCreate
	// before it builds the first section, so every builder can rely on it.
	sh      *shell.Shell
	version string
	commit  string
	// configPath is the --config override, carried into every core request so
	// that the window and `nmsbonker --config …` read the same document.
	configPath string

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
	// operation starts or stops, and a rebuild that reset the form would throw
	// away the values Preview was just asked about.
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
		onInvalidate along with everything else loaded from the core.
	*/
	freshAudit *core.AuditResult

	// run is everything the Build section shows. It outlives the section, so a
	// build keeps streaming while the user reads the Report.
	run buildRun
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

// sectionEntry is what a section is made of: a deferred icon, its builder,
// and the hook that releases the live widgets it holds when it is replaced.
type sectionEntry struct {
	icon   func() fyne.Resource
	build  func(*ui) fyne.CanvasObject
	detach func(*ui)
}

// sectionBuilders is what each section is made of. sections walks
// sectionTitles and looks each one up here, so a title with no builder is a
// missing section rather than a silently different list from the one --section
// is told about.
//
// A function rather than a package variable: the builders reach back to the
// sections when a section rebuilds itself, and Go reports that as an
// initialization cycle in a package-level map.
func sectionBuilders() map[string]sectionEntry {
	return map[string]sectionEntry{
		"Overview": {theme.HomeIcon, (*ui).buildOverview, nil},
		"Mods":     {theme.ListIcon, (*ui).buildMods, nil},
		"Tweaks":   {theme.SettingsIcon, (*ui).buildTweaks, nil},
		// The Build section holds the run's live widgets (the step rows, the
		// log list, the Cancel button), so it is told before it is replaced.
		"Build":      {theme.MediaPlayIcon, (*ui).buildBuild, func(u *ui) { u.run.detach() }},
		"Report":     {theme.DocumentIcon, (*ui).buildReport, nil},
		"Saves":      {theme.StorageIcon, (*ui).buildSaves, nil},
		"Tools":      {theme.ComputerIcon, (*ui).buildTools, nil},
		"Settings":   {theme.SettingsIcon, (*ui).buildSettings, nil},
		"Appearance": {theme.ColorPaletteIcon, (*ui).buildAppearance, nil},
		"About":      {theme.HelpIcon, (*ui).buildAbout, nil},
	}
}

// sections is the navigation as the shell takes it. A nil u is enough for the
// titles, which is all SectionNames needs.
func sections(u *ui) []shell.Section {
	builders := sectionBuilders()
	out := make([]shell.Section, 0, len(sectionTitles))
	for _, title := range sectionTitles {
		b, ok := builders[title]
		if !ok {
			continue // a title with no builder draws nothing; see SectionNames
		}
		sec := shell.NewSection(title, b.icon, func(*shell.Shell) fyne.CanvasObject { return b.build(u) })
		if b.detach != nil {
			sec.OnDetach(func() { b.detach(u) })
		}
		out = append(out, sec)
	}
	return out
}

// SectionNames lists the navigation entries, for --section and for a capture
// script to iterate.
//
// Reads the titles rather than building the sections: this is called while
// parsing flags, before there is an app to hang an icon on.
func SectionNames() []string { return shell.Names(sections(nil)) }

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
	u := &ui{version: o.Version, commit: o.Commit, configPath: o.ConfigPath}
	u.run.init()
	shell.Run(u.shellOptions(o))
}

// shellOptions describes this program to the shell.
//
// The preference store the app ID names holds the appearance settings and
// nothing else: every setting the CLI can also see lives in config.json, so
// that `nmsbonker status` in a terminal and this window agree.
func (u *ui) shellOptions(o Options) shell.Options {
	return shell.Options{
		AppID:     appID,
		Name:      "nmsbonker",
		Version:   u.version,
		Icon:      appIcon(),
		Sections:  sections(u),
		Section:   o.Section,
		Scheme:    o.Scheme,
		StatusBar: func(*shell.Shell) []fyne.CanvasObject { return u.statusSegments() },
		OnCreate:  func(s *shell.Shell) { u.sh = s },
		// The status bar names the game, the compiler and the mod library from
		// every section, so both are fetched here rather than left to
		// Overview. Left to a section, the bar read "reading…" and "—"
		// everywhere else.
		OnStart: func(*shell.Shell) {
			u.loadStatus()
			u.loadMods()
		},
		OnInvalidate: u.onInvalidate,
		// The build runs outside Perform, with a step list of its own; the
		// shell counts it as work so every section's buttons gate on it.
		AlsoWorking: func() bool { return u.run.running },
	}
}

// onInvalidate discards everything loaded from the core, which makes the
// sections fetch again, and reloads what the status bar shows from every
// section. The shell rebuilds afterwards. Off screen, clearing the flags is
// the whole of the work: fetching anyway would send a headless test off to
// read the user's Steam install.
//
// The build log is deliberately not part of this: F5 while a build is running
// must not throw away the output it has produced so far.
func (u *ui) onInvalidate(s *shell.Shell) {
	u.statusOK = false
	u.detectOK = false
	u.modsOK = false
	u.tweaksOK = false
	u.savesOK = false
	u.slotsOK = false
	u.archiveOK = false
	u.toolsOK = false
	u.releasesOK = false
	u.cacheOK = false
	u.configOK = false
	u.lastReportOK = false
	u.freshAudit = nil
	if !s.OnScreen() {
		return
	}
	u.loadStatus()
	u.loadMods()
}

// statusSegments is R3: the game, the compiler, the library and the output
// folder, from every section.
func (u *ui) statusSegments() []fyne.CanvasObject {
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

	return []fyne.CanvasObject{
		widgets.Dim("game"), game, widgets.Sep(),
		widgets.Dim("compiler"), compiler, widgets.Sep(),
		widgets.Dim("mods"), widget.NewLabel(modsText), widgets.Sep(),
		widgets.Dim("output"), widget.NewLabel(output),
	}
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

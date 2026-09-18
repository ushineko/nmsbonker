package gui

import (
	"context"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/nmsbonker/internal/core"
)

// --- Tools (R2.5) ----------------------------------------------------------

/*
buildTools is everything the build depends on that is not a mod: the compiler,
the caches, and the game's archives.

The three cards are ordered by how often they are the answer. The compiler is
what a game update breaks. The caches are what a strange build result is fixed
by. The archives are where someone goes when a mod names a file and nothing can
find it.
*/
func (u *ui) buildTools() fyne.CanvasObject {
	u.loadStatus()
	u.loadTools()

	body := container.NewVBox(
		widgets.Heading("Tools", "MBINCompiler, the caches, and the game archives they are read from."),
		u.compilerCard(),
		widget.NewSeparator(),
		u.cacheCard(),
		widget.NewSeparator(),
		u.archivesCard(),
	)
	return container.NewVScroll(body)
}

// compilerCard is the installed releases and what can be done to them.
func (u *ui) compilerCard() fyne.CanvasObject {
	selected := -1
	entries := u.tools.Entries

	t := table.New()
	t.Header("", "Tag", "Flavor", "Save mapping", "Reports", "Path")
	t.SetWidths(30, 150, 130, 110, 260, 420)
	for _, e := range entries {
		marker, st := "", fd.StatusInfo
		if e.Active {
			marker, st = "*", fd.StatusGood
		}
		mapping := "absent"
		if e.Mapping {
			mapping = "present"
		}
		t.Row(st, marker, e.Tag, widgets.OrNone(e.Flavor, "—"), mapping, e.Version, e.Dir)
	}
	table := t.Widget()

	pin := widget.NewButtonWithIcon("Pin", theme.ConfirmIcon(), nil)
	remove := widget.NewButtonWithIcon("Remove…", theme.DeleteIcon(), nil)
	remove.Importance = widget.DangerImportance
	rowActions := []*widget.Button{pin, remove}
	for _, b := range rowActions {
		b.Disable()
	}
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(entries) {
			return
		}
		selected = id.Row
		for _, b := range rowActions {
			b.Enable()
		}
		if entries[id.Row].Active {
			// Removing the release builds are using would leave the next build
			// reaching for a compiler that is not there; core refuses it, and
			// so does the button.
			remove.Disable()
		}
		u.gate(rowActions...)
	}
	pin.OnTapped = func() { u.pinTool(entries[selected].Tag) }
	remove.OnTapped = func() { u.removeToolDialog(entries[selected]) }

	check := widget.NewButtonWithIcon("Check for updates", theme.SearchIcon(),
		func() { u.loadReleases() })
	install := widget.NewButtonWithIcon("Install the newest match", theme.DownloadIcon(),
		func() { u.ensureTools() })
	compat := widget.NewButtonWithIcon("Check compatibility", theme.QuestionIcon(),
		func() { u.toolCheck() })
	unpin := widget.NewButtonWithIcon("Unpin", theme.CancelIcon(), func() { u.pinTool("") })
	if u.tools.Pin == "" {
		unpin.Disable()
	}
	u.gate(check, install, compat, unpin)

	rows := []fyne.CanvasObject{}
	if u.toolsOK && len(entries) == 0 {
		rows = append(rows,
			widgets.FactRow("MBINCompiler", "nothing installed", fd.StatusBad),
			widgets.Note("Install the newest match downloads the release that fits this game from "+
				"the MBINCompiler releases on GitHub, into the tools directory. Nothing is "+
				"installed system-wide and the game is not touched.", fd.StatusInfo))
	} else {
		rows = append(rows, widgets.FixedHeight(table, 150))
	}
	rows = append(rows,
		widgets.PlainRow("Tools directory", u.tools.ToolsDir),
		widgets.PlainRow("Pinned", widgets.OrNone(u.tools.Pin, "none — the newest match is chosen automatically")),
		widgets.FactRow(".NET 10 runtime",
			dotnetText(u.status.Compiler.Dotnet10, u.status.Compiler.Flavor),
			dotnetStatus(u.status.Compiler.Dotnet10, u.status.Compiler.Flavor)),
		container.NewHBox(check, install, compat, pin, unpin, remove),
	)
	return widgets.Card("MBINCompiler", rows...)
}

// cacheCard is the derived data and the one button that throws it away.
func (u *ui) cacheCard() fyne.CanvasObject {
	measure := widget.NewButtonWithIcon("Measure", theme.SearchIcon(), func() { u.loadCache() })
	clearBtn := widget.NewButtonWithIcon("Clear cache…", theme.DeleteIcon(),
		func() { u.clearCacheDialog() })
	clearBtn.Importance = widget.DangerImportance
	u.gate(measure, clearBtn)

	rows := []fyne.CanvasObject{widgets.PlainRow("Cache directory", u.status.Paths.Cache)}
	if !u.cacheOK {
		rows = append(rows,
			widgets.Note("The pristine cache holds every game file a build has decompiled, keyed by the "+
				"game's Steam buildid. Measuring it walks the tree, so it is asked for rather "+
				"than done on arrival.", fd.StatusInfo))
	} else {
		rows = append(rows,
			widgets.PlainRow("Pristine cache", fmt.Sprintf("%s in %d file(s) across %d game build(s)",
				widgets.HumanSize(u.cache.TotalBytes), u.cache.TotalFiles, len(u.cache.Games))),
			widgets.PlainRow("Current buildid", widgets.OrNone(u.cache.CurrentBuildID, "unknown")),
			widgets.PlainRow("Pak index", widgets.HumanSize(u.cache.PakIndexBytes)+" · "+u.cache.PakIndexPath),
			widgets.PlainRow("Release listing", widgets.HumanSize(u.cache.ReleasesBytes)),
		)
		if len(u.cache.Games) > 1 {
			t := table.New()
			t.Header("", "Game buildid", "Files", "Size", "Written")
			t.SetWidths(30, 160, 90, 110, 140)
			for _, g := range u.cache.Games {
				marker, st := "", fd.StatusInfo
				if g.Current {
					marker, st = "*", fd.StatusGood
				}
				t.Row(st, marker, g.BuildID, strconv.Itoa(g.Files), widgets.HumanSize(g.Bytes),
					widgets.HumanAgo(g.Modified))
			}
			rows = append(rows, widgets.FixedHeight(t.Widget(), 120))
			rows = append(rows, widgets.Note("Trees for game builds you no longer have are kept "+
				"deliberately: rolling a game update back finds its cache still there.",
				fd.StatusInfo))
		}
	}
	rows = append(rows, container.NewHBox(measure, clearBtn))
	return widgets.Card("Caches", rows...)
}

// archivesCard is the game's own .pak files: the index over them, and the two
// ways to look inside.
func (u *ui) archivesCard() fyne.CanvasObject {
	idx := u.status.PakIndex
	indexText, indexStatus := "not built yet", fd.StatusInfo
	switch {
	case !idx.Exists:
	case idx.Stale == 0 && idx.Missing == 0:
		indexText = fmt.Sprintf("current: %d paks, %d files, written %s",
			idx.Paks, idx.Files, widgets.HumanAgo(idx.Written))
		indexStatus = fd.StatusGood
	default:
		indexText = fmt.Sprintf("stale: %d pak(s) changed, %d gone, written %s",
			idx.Stale, idx.Missing, widgets.HumanAgo(idx.Written))
		indexStatus = fd.StatusWarn
	}

	rebuild := widget.NewButtonWithIcon("Rebuild index", theme.ViewRefreshIcon(),
		func() { u.rebuildIndex() })
	browse := widget.NewButtonWithIcon("Archives…", theme.StorageIcon(), func() { u.browseArchives() })
	find := widget.NewButtonWithIcon("Find a file…", theme.SearchIcon(), func() { u.findInArchivesDialog() })
	u.gate(rebuild, browse, find)
	if !u.status.Install.Found {
		rebuild.Disable()
		browse.Disable()
		find.Disable()
	}

	return widgets.Card("Game archives",
		widgets.PlainRow("PCBANKS", widgets.OrNone(u.status.Install.PCBanksDir, "no game found")),
		widgets.PlainRow("Archives", fmt.Sprintf("%d .pak", u.status.Install.PakCount)),
		widgets.FactRow("Index", indexText, indexStatus),
		widgets.Note("The index maps every path inside every archive, so a build does not open "+
			"ninety-seven files to find one. It re-reads only the archives whose size or "+
			"timestamp changed; rebuilding is for when that is not enough.", fd.StatusInfo),
		container.NewHBox(rebuild, browse, find),
	)
}

// --- tool operations -------------------------------------------------------

// ensureTools installs the release this game needs.
func (u *ui) ensureTools() {
	u.perform("Installing MBINCompiler…", func(ctx context.Context) error {
		res, err := core.EnsureTools(ctx, core.EnsureToolsRequest{Request: u.request()})
		if err != nil {
			// The attempts are worth showing even on failure: "dotnet10 does
			// not run here, self-contained was not published for this release"
			// is the whole diagnosis, and it is lost if only the error is shown.
			if len(res.Attempts) > 0 {
				fyne.Do(func() {
					u.flash("Installing MBINCompiler failed: "+err.Error()+" — "+
						joinLines(res.Attempts), fd.StatusBad)
				})
				return nil
			}
			return err
		}
		what := "Installed"
		if res.AlreadyPresent {
			what = "Already had"
		}
		u.ok(fmt.Sprintf("%s MBINCompiler %s (%s): %s. Release listing came from %s.",
			what, res.Tag, res.Flavor, res.Reason, res.ListingSource))
		return nil
	})
}

// toolCheck round-trips known game files through the installed compiler.
func (u *ui) toolCheck() {
	u.perform("Checking compiler compatibility…", func(ctx context.Context) error {
		res, err := core.ToolCheck(ctx, core.ToolCheckRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			// The Overview's Compatibility row reads this, so an explicit check
			// here answers it there as well rather than leaving two verdicts on
			// two cards.
			u.compat, u.compatOK, u.compatError = res, true, ""
			u.statusOK = false
			u.loadStatus()
			u.showToolCheck(res)
		})
		return nil
	})
}

// showToolCheck puts the round-trip evidence on screen: which file proved what.
func (u *ui) showToolCheck(res core.ToolCheckResult) {
	t := table.New()
	t.Header("File", "Result")
	t.SetWidths(420, 460)
	for _, f := range res.Files {
		st, state := fd.StatusGood, "round-trips byte-identical outside the header"
		if !f.OK {
			st, state = fd.StatusWarn, f.Reason
		}
		t.Row(st, f.Name, state)
	}
	for name, why := range res.Skipped {
		t.Row(fd.StatusInfo, name, "skipped: "+why)
	}

	body := container.NewVBox(
		container.NewHBox(widgets.Marker(compatStatus(res.Status)),
			widgets.StatusText(res.Status, compatStatus(res.Status))),
		widgets.PlainRow("Compiler", widgets.OrNone(res.CompilerVersion, "none")),
		widgets.Wrapped(res.Detail),
		widgets.FixedHeight(t.Widget(), 160),
	)
	if res.Advice != "" {
		body.Add(widgets.Note(res.Advice, fd.StatusWarn))
	}
	body.Add(widgets.Wrapped("A mismatch does not stop a build. It means the compiler and this game " +
		"install disagree about a file format, so what a build produces is worth doubting — " +
		"the report says which file proved it."))
	dialogs.ShowDetail(u.win, "Compiler compatibility", body, 900, 520)
}

// showReleases lists what GitHub offers.
func (u *ui) showReleases() {
	t := table.New()
	t.Header("", "Tag", "State", "Assets")
	t.SetWidths(30, 190, 170, 80)
	for _, r := range u.releases.Releases {
		st := fd.StatusInfo
		marker := ""
		switch {
		case r.Active:
			st, marker = fd.StatusGood, "*"
		case r.Selected:
			st = fd.StatusWarn
		}
		t.Row(st, marker, r.Tag, releaseStateText(r), strconv.Itoa(r.Assets))
	}

	install := widget.NewButtonWithIcon("Install the newest match", theme.DownloadIcon(),
		func() { u.ensureTools() })
	install.Importance = widget.HighImportance

	body := container.NewBorder(
		container.NewVBox(
			widgets.PlainRow("Listing from", u.releases.Source),
			widgets.PlainRow("Game data version", u.releases.GameDataVersion),
			widgets.PlainRow("Pinned", widgets.OrNone(u.releases.Pin, "none")),
			widgets.PlainRow("Would install", widgets.OrNone(u.releases.Selected, "nothing")),
			widgets.Wrapped(u.releases.Reason),
		),
		container.NewHBox(install), nil, nil, widgets.FixedHeight(t.Widget(), 300))
	dialogs.ShowDetail(u.win, "MBINCompiler releases", body, 780, 580)
}

// releaseStateText names a release's relationship to this machine in one word.
func releaseStateText(r core.ReleaseInfo) string {
	switch {
	case r.Active:
		return "in use"
	case r.Installed:
		return "installed"
	case r.Selected:
		return "would install"
	case r.Prerelease:
		return "prerelease"
	}
	return ""
}

// pinTool fixes, or releases, the compiler choice.
func (u *ui) pinTool(tag string) {
	what := "Pinning " + tag + "…"
	if tag == "" {
		what = "Removing the pin…"
	}
	u.perform(what, func(ctx context.Context) error {
		res, err := core.PinTool(ctx, core.PinToolRequest{Request: u.request(), Tag: tag})
		if err != nil {
			return err
		}
		if res.Pin == "" {
			u.ok("Removed the pin. The newest release that matches this game is chosen again.")
			return nil
		}
		msg := "Pinned " + res.Pin + ". Every build uses it until you unpin."
		if !res.Installed {
			msg += " It is not installed yet; Install the newest match will fetch it."
		}
		u.ok(msg)
		return nil
	})
}

// removeToolDialog deletes an installed release that is not in use.
func (u *ui) removeToolDialog(e core.ToolEntry) {
	dialogs.ConfirmDestructive(u.win, "Remove MBINCompiler "+e.Tag+"?",
		"This deletes "+e.Dir+" and nothing else.\n\n"+
			"The release currently in use is not this one, so builds are unaffected. The "+
			"pristine cache built with this compiler is kept: it is keyed by the game build, "+
			"not by the compiler, and it is still correct.\n\n"+
			"It can be downloaded again from GitHub at any time.",
		"Remove", func() {
			u.perform("Removing MBINCompiler "+e.Tag+"…", func(ctx context.Context) error {
				res, err := core.RemoveTool(ctx, core.RemoveToolRequest{
					Request: u.request(), Tag: e.Tag,
				})
				if err != nil {
					return err
				}
				u.ok(fmt.Sprintf("Removed %s, freeing %s.", res.Tag, widgets.HumanSize(res.Bytes)))
				return nil
			})
		})
}

// clearCacheDialog throws away derived data.
func (u *ui) clearCacheDialog() {
	all := widget.NewCheck("Clear every game build's cache, not only this one's", nil)
	index := widget.NewCheck("Also delete the pak index", nil)
	body := container.NewVBox(
		widgets.Wrapped("This deletes the game files a build has already extracted and decompiled. "+
			"They are rebuilt by the next build, which costs the few seconds the cache "+
			"normally saves."),
		widgets.Wrapped("Not touched: your mod library, the build output in the workspace, the mod "+
			"folder installed in the game, and the game's own files."),
		all, index,
	)
	dialogs.ConfirmWithBody(u.win, "Clear the cache?", body, "Clear", func() {
		everything, withIndex := all.Checked, index.Checked
		u.perform("Clearing the cache…", func(ctx context.Context) error {
			res, err := core.ClearCache(ctx, core.ClearCacheRequest{
				Request: u.request(), All: everything, IncludeIndex: withIndex,
			})
			if err != nil {
				return err
			}
			if len(res.Removed) == 0 {
				u.ok("Nothing was cached, so nothing was removed.")
				return nil
			}
			fyne.Do(func() { u.cacheOK = false })
			u.ok(fmt.Sprintf("Cleared %d file(s), freeing %s.", res.Files, widgets.HumanSize(res.Bytes)))
			return nil
		})
	}).Show()
}

// rebuildIndex reads every archive again.
func (u *ui) rebuildIndex() {
	u.perform("Rebuilding the pak index…", func(ctx context.Context) error {
		res, err := core.RebuildIndex(ctx, core.RebuildIndexRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.cacheOK = false })
		u.ok(fmt.Sprintf("Indexed %d archive(s) and %d file(s) in %s.",
			res.Paks, res.Files, res.Duration.Round(1e6)))
		return nil
	})
}

// browseArchives lists the archives, and then one archive's contents.
func (u *ui) browseArchives() {
	u.perform("Listing the archives…", func(ctx context.Context) error {
		res, err := core.PakList(ctx, core.PakListRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.showArchives(res) })
		return nil
	})
}

func (u *ui) showArchives(res core.PakListResult) {
	selected := -1
	t := table.New()
	t.Header("Archive", "Files", "Size")
	t.SetWidths(420, 100, 120)
	for _, p := range res.Paks {
		t.Row(fd.StatusInfo, p.Name, strconv.Itoa(p.Files), widgets.HumanSize(p.Size))
	}
	table := t.Widget()

	glob := widget.NewEntry()
	glob.SetPlaceHolder("optional pattern, for example *.mbin")

	list := widget.NewButtonWithIcon("List files in the selected archive", theme.ListIcon(), nil)
	list.Disable()
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(res.Paks) {
			return
		}
		selected = id.Row
		list.Enable()
	}
	list.OnTapped = func() {
		pak, pattern := res.Paks[selected].Name, glob.Text
		u.perform("Listing "+pak+"…", func(ctx context.Context) error {
			out, err := core.PakList(ctx, core.PakListRequest{
				Request: u.request(), Pak: pak, Glob: pattern,
			})
			if err != nil {
				return err
			}
			fyne.Do(func() { u.showArchiveFiles(pak, out) })
			return nil
		})
	}

	body := container.NewBorder(
		widgets.PlainRow("PCBANKS", res.PCBanksDir), container.NewBorder(nil, nil, nil, list, glob),
		nil, nil, widgets.FixedHeight(table, 360))
	dialogs.ShowDetail(u.win, "Game archives", body, 780, 560)
}

func (u *ui) showArchiveFiles(pak string, res core.PakListResult) {
	t := table.New()
	t.Header("File", "Bytes")
	t.SetWidths(620, 120)
	for _, e := range res.Entries {
		t.Row(fd.StatusInfo, e.Name, strconv.FormatUint(e.Size, 10))
	}
	body := container.NewBorder(
		widget.NewLabel(fmt.Sprintf("%d file(s) in %s", len(res.Entries), pak)),
		nil, nil, nil, widgets.FixedHeight(t.Widget(), 420))
	dialogs.ShowDetail(u.win, pak, body, 860, 600)
}

// findInArchivesDialog searches every archive at once through the index.
func (u *ui) findInArchivesDialog() {
	glob := widget.NewEntry()
	glob.SetPlaceHolder("a name or a pattern, for example *rewardtable*")
	dialogs.Prompt(u.win, "Find a file in the archives", "Find", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Pattern", glob)),
		widgets.Wrapped("A pattern with no wildcard matches anywhere in a path. The search reads the "+
			"index rather than the archives, so it is instant once the index is warm."),
	), func() {
		pattern := glob.Text
		u.perform("Searching the archives…", func(ctx context.Context) error {
			res, err := core.PakFind(ctx, core.PakFindRequest{Request: u.request(), Glob: pattern})
			if err != nil {
				return err
			}
			fyne.Do(func() { u.showFindResults(res) })
			return nil
		})
	})
}

func (u *ui) showFindResults(res core.PakFindResult) {
	selected := -1
	t := table.New()
	t.Header("File", "Archive")
	t.SetWidths(560, 240)
	for _, m := range res.Matches {
		t.Row(fd.StatusInfo, m.Name, m.Pak)
	}
	table := t.Widget()

	extract := widget.NewButtonWithIcon("Extract…", theme.DownloadIcon(), nil)
	extract.Disable()
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(res.Matches) {
			return
		}
		selected = id.Row
		extract.Enable()
	}
	extract.OnTapped = func() { u.extractDialog(res.Matches[selected].Name) }

	summary := fmt.Sprintf("%d match(es) for %q in %d file(s) across %d archive(s).",
		len(res.Matches), res.Glob, res.IndexedFiles, res.IndexedPaks)
	body := container.NewBorder(widget.NewLabel(summary), container.NewHBox(extract), nil, nil,
		widgets.FixedHeight(table, 380))
	dialogs.ShowDetail(u.win, "Search results", body, 880, 560)
}

// extractDialog writes one file out of the archives, keeping its internal path.
func (u *ui) extractDialog(name string) {
	dest := widget.NewEntry()
	dest.SetPlaceHolder("directory to write into")
	dialogs.Prompt(u.win, "Extract "+name, "Extract", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Destination", dialogs.WithBrowse(u.win, dest, true))),
		widgets.Wrapped("The file's directory structure inside the archive is recreated under this "+
			"directory, so what is written can be compared with the archive it came from. "+
			"The game is not modified — this reads the archives and writes a copy."),
	), func() {
		out := dest.Text
		u.perform("Extracting "+name+"…", func(ctx context.Context) error {
			res, err := core.PakExtract(ctx, core.PakExtractRequest{
				Request: u.request(), Name: name, OutDir: out,
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Wrote %s (%s) from %s.", res.Path, widgets.HumanSize(int64(res.Size)), res.Pak)
			if res.ByBasename {
				msg += " The exact path is in no archive; this matched by basename."
			}
			u.ok(msg)
			return nil
		})
	})
}

// joinLines renders a short list of attempts for one banner.
func joinLines(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += "; "
		}
		out += s
	}
	return out
}

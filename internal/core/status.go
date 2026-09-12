package core

import (
	"context"
	"time"

	"github.com/ushineko/nmsbonker/internal/buildinfo"
	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// StatusRequest asks for a summary of everything the tool can see.
type StatusRequest struct {
	Request
}

// InstallSummary is the game side of `status` (R7.2).
type InstallSummary struct {
	Found           bool
	Error           string
	Dir             string
	Source          string
	LibraryDir      string
	Name            string
	BuildID         string
	PCBanksDir      string
	PakCount        int
	ModsDir         string
	ModsState       string
	ModsTarget      string
	ModSettingsPath string
	ModSettingsOK   bool
	DisableAllMods  bool
	Mods            []steam.ModSetting
	CompatDataDir   string
}

// CompilerSummary is the tool side of `status` (R7.2).
type CompilerSummary struct {
	Installed bool
	Tag       string
	Flavor    string
	Bin       string
	// Version is what the binary printed, or the reason it could not be asked.
	Version string
	// Others are installed releases that are not the active one.
	Others []string
}

// IndexSummary reports the pak index's freshness without rebuilding it (R7.2).
type IndexSummary struct {
	Exists  bool
	Written time.Time
	Paks    int
	Files   int
	Stale   int
	Missing int
}

// StatusResult is everything `status` reports.
type StatusResult struct {
	Version         string
	Commit          string
	ConfigPath      string
	Paths           config.Paths
	Install         InstallSummary
	Compiler        CompilerSummary
	GameDataVersion string
	Compatibility   string
	LibraryMods     int
	PakIndex        IndexSummary
}

/*
Status answers "what does this machine look like" in one call (R7.2).

Everything here is read-only and nothing is fatal. A missing game, a missing
compiler and an empty cache are the three states a new user is in, and `status`
is the command they run to find out which; failing on any of them would make the
diagnostic command the one that cannot run.
*/
func Status(ctx context.Context, req StatusRequest) (StatusResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return StatusResult{}, err
	}

	out := StatusResult{
		Version:       buildinfo.Version,
		Commit:        buildinfo.Commit,
		ConfigPath:    s.cfg.Path(),
		Paths:         s.paths,
		LibraryMods:   countLuaScripts(s.paths.Library),
		Compatibility: mbin.CompatUnknown,
	}

	out.Install = InstallSummary{Source: s.gameSource}
	if s.installErr != nil {
		out.Install.Error = s.installErr.Error()
	} else {
		in := s.install
		out.Install.Found = true
		out.Install.Dir = in.Dir
		out.Install.LibraryDir = in.LibraryDir
		out.Install.Name = in.Name
		out.Install.BuildID = in.BuildID
		out.Install.PCBanksDir = in.PCBanksDir
		out.Install.ModsDir = in.ModsDir
		out.Install.ModsState = in.ModsState
		out.Install.ModsTarget = in.ModsTarget
		out.Install.ModSettingsPath = in.ModSettingsPath
		out.Install.ModSettingsOK = in.ModSettingsOK
		out.Install.CompatDataDir = in.CompatDataDir

		paks, err := in.PakFiles()
		if err != nil {
			req.Events.logf(LevelWarn, "listing paks: %v", err)
		}
		out.Install.PakCount = len(paks)

		settings, err := steam.ReadModSettings(in.ModSettingsPath)
		if err != nil {
			req.Events.logf(LevelWarn, "reading %s: %v", in.ModSettingsPath, err)
		} else {
			out.Install.DisableAllMods = settings.DisableAllMods
			out.Install.Mods = settings.Mods
		}

		fresh := hgpak.IndexFreshness(s.paths.PakIndex, paks)
		out.PakIndex = IndexSummary(fresh)
	}

	tags := mbin.Installed(s.paths.Tools)
	if compiler, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin); err == nil {
		out.Compiler.Installed = true
		out.Compiler.Tag = compiler.Tag
		out.Compiler.Flavor = compiler.Flavor
		out.Compiler.Bin = compiler.Bin
		if v, err := compiler.Version(ctx); err == nil {
			out.Compiler.Version = v
		} else {
			out.Compiler.Version = "does not run: " + err.Error()
		}
		for _, tag := range tags {
			if tag != compiler.Tag {
				out.Compiler.Others = append(out.Compiler.Others, tag)
			}
		}
	} else {
		out.Compiler.Others = tags
	}

	gdv, err := gameDataVersion(ctx, s, req.Events)
	if err != nil {
		return out, err
	}
	if gdv.Known {
		out.GameDataVersion = gdv.Version.Numeric()
	} else {
		out.GameDataVersion = mbin.VersionUnknown
	}

	compilerVersion, haveCompiler := mbin.ParseVersion(out.Compiler.Tag)
	out.Compatibility = mbin.Compatibility(compilerVersion, gdv.Version,
		haveCompiler && out.Compiler.Installed, gdv.Known)
	return out, nil
}

// DetectRequest asks where the game was looked for.
type DetectRequest struct {
	Request
}

// DetectResult lists every candidate examined and why each was rejected (R7.2).
type DetectResult struct {
	Source     string
	Found      bool
	GameDir    string
	Error      string
	Roots      []string
	Candidates []steam.Candidate
}

/*
Detect explains discovery (R7.2).

It exists because "installation not found" is useless on its own: Steam
libraries move to a second drive, a Flatpak install puts the root somewhere
else, and a partially uninstalled game leaves an appmanifest with no PCBANKS.
Listing every place examined and the reason each was rejected turns that into
something a user can act on without reading this source.
*/
func Detect(_ context.Context, req DetectRequest) (DetectResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return DetectResult{}, err
	}
	out := DetectResult{
		Source:     s.gameSource,
		Roots:      steam.Roots(),
		Candidates: s.install.Candidates,
	}
	if s.installErr != nil {
		// Detect reports a failed search as data, not as an error: the whole
		// point of the command is to explain one.
		out.Error = s.installErr.Error()
		return out, nil //nolint:nilerr // the failure is the result
	}
	out.Found = true
	out.GameDir = s.install.Dir
	return out, nil
}

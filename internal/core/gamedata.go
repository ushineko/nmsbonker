package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

// GlobalsPak and GlobalsFile are where the game's data version is read from
// (R6.1). The globals live at the pak root, not under GLOBALS/, whatever the
// scripts write.
const (
	GlobalsPak  = "NMSARC.globals.pak"
	GlobalsFile = "gcgameplayglobals.global.mbin"
)

// GameDataVersionRequest asks what libMBIN version the installed game's data
// was built with.
type GameDataVersionRequest struct {
	Request
}

// GameDataVersionResult is the answer (R6.1).
type GameDataVersionResult struct {
	// Version is the parsed M.m.p, valid only when Known is true.
	Version mbin.Version
	Known   bool
	// Raw is what MBINCompiler printed, kept because its format is not
	// documented and a future change should be visible rather than swallowed.
	Raw string
	// Reason explains an unknown answer.
	Reason string
}

/*
GameDataVersion reads the game's data version through MBINCompiler (R6.1).

There is no version string in the pak, and no Go parser for MBIN, so the only
way to ask is to hand a known MBIN to the compiler and read what it says the
file was made with. gcgameplayglobals is the natural choice: it is small, it is
in every install, and every build already reads it.

An unknown answer is never fatal. Selection falls back to the highest release
(R5.2) and compatibility reports "unknown" (R6.2); refusing to run because the
version could not be read would make a cosmetic failure into a blocking one.
*/
func GameDataVersion(ctx context.Context, req GameDataVersionRequest) (GameDataVersionResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return GameDataVersionResult{}, err
	}
	return gameDataVersion(ctx, s, req.Events)
}

func gameDataVersion(ctx context.Context, s *session, ev Events) (GameDataVersionResult, error) {
	if err := s.requireInstall(); err != nil {
		return GameDataVersionResult{Reason: err.Error()}, nil
	}
	compiler, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	if err != nil {
		return GameDataVersionResult{Reason: "no MBINCompiler is installed yet; run `nmsbonker tools ensure`"}, nil
	}

	pak, err := hgpak.Open(filepath.Join(s.install.PCBanksDir, GlobalsPak))
	if err != nil {
		return GameDataVersionResult{Reason: err.Error()}, nil
	}
	defer func() { _ = pak.Close() }()

	data, err := pak.ReadFile(GlobalsFile)
	if err != nil {
		return GameDataVersionResult{Reason: err.Error()}, nil
	}

	// A temporary file, not the workspace: this is a probe, and leaving a copy
	// of a game asset lying around under a directory the user browses would
	// invite it being mistaken for build output.
	dir, err := os.MkdirTemp("", "nmsbonker-version-")
	if err != nil {
		return GameDataVersionResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// The extension must be .MBIN: `version <file>` refuses anything else.
	probe := filepath.Join(dir, "gcgameplayglobals.global.MBIN")
	if err := os.WriteFile(probe, data, 0o600); err != nil {
		return GameDataVersionResult{}, fmt.Errorf("write %s: %w", probe, err)
	}

	raw, err := compiler.FileVersion(ctx, probe)
	if err != nil {
		ev.logf(LevelDebug, "reading the game data version failed: %v", err)
		return GameDataVersionResult{Raw: raw, Reason: err.Error()}, nil
	}
	if notCompiledRE.MatchString(raw) {
		return GameDataVersionResult{Raw: raw, Reason: notCompiledReason}, nil
	}
	v, ok := mbin.FindVersion(raw)
	if !ok {
		return GameDataVersionResult{Raw: raw, Reason: "MBINCompiler's answer held no M.m.p version"}, nil
	}
	if !plausibleGameVersion(v) {
		return GameDataVersionResult{Raw: raw, Reason: fmt.Sprintf(
			"MBINCompiler read %s out of %s, which is not a game version; %s",
			v.Numeric(), GlobalsFile, notCompiledReason)}, nil
	}
	return GameDataVersionResult{Version: v, Known: true, Raw: raw}, nil
}

/*
notCompiledRE and plausibleGameVersion guard against a version that is not one.

R6.1 assumes `version <file>` on a pak-extracted MBIN yields the game's data
version. Measured on this machine (2026-09-11), it does not: the game's own
MBINs are not produced by MBINCompiler, so the field libMBIN reads holds
something else. metadata/reality/tables/rewardtable.mbin answers "Unknown MBIN
version! Not compiled by MBINCompiler."; gcgameplayglobals.global.mbin has a
non-zero value there and answers "Compiled with MBINCompiler v61.124.112.44",
which is four header bytes read as a version quad. Both MBINCompiler 7.01.0-pre1
and 7.02.0-pre1 agree, so this is the format, not a bug in one release.

The mechanism is kept because it is correct for the files it was meant for --
everything in the phase-2 build cache is MBINCompiler output and does carry a
real stamp -- and because a future libMBIN may learn to read the game's field.
The guard is what stops a garbage quad from being reported as the game version
and steering release selection (R5.2) with it. An unknown version is a
first-class outcome there: selection falls back to the highest release.
*/
var notCompiledRE = regexp.MustCompile(`(?i)not compiled by MBINCompiler|Unknown MBIN version`)

const notCompiledReason = "the game's own MBINs are not produced by MBINCompiler and carry no libMBIN version"

// plausibleGameVersion rejects a quad that cannot be a No Man's Sky release.
// Release tags run 6.45, 7.01, 7.02; a component in the hundreds is header
// bytes being read as a version.
func plausibleGameVersion(v mbin.Version) bool {
	return v.Major >= 1 && v.Major <= 99 && v.Minor <= 99 && v.Patch <= 99
}

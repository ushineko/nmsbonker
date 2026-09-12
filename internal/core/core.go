/*
Package core holds every user-facing operation as a headless function
(spec 001 R7).

The rule the whole project turns on: an operation is a function taking a request
struct and returning a result struct, and the front ends only render. The cobra
CLI (cmd/nmsbonker) and the Fyne GUI (spec 003) therefore cannot drift apart in
behaviour, only in presentation, and a new operation is one place to add rather
than two places to keep in step.

Nothing here writes to the game directory. Reading paks, listing releases and
installing tools all happen under the workspace and cache directories; spec 004's
deploy is the only operation that will write under GAMEDATA/MODS.
*/
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/nmsbonker/internal/buildinfo"
	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/mbin"
	"github.com/ushineko/nmsbonker/internal/steam"
)

// Level ranks a log line so a front end can present it by importance. The CLI
// prints debug only under -v; the GUI colours warnings.
type Level int

// Log levels.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String names a level for the CLI's stderr output.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "?"
	}
}

// Progress reports how far through a long operation we are (R7.1).
type Progress struct {
	Step  int
	Total int
	What  string
}

/*
Events is how a long operation talks back to whichever front end started it
(R7.1).

Both fields may be nil and every call site goes through the helpers below, so a
caller that does not care about progress writes nothing. That matters more than
it sounds: the alternative -- an interface with a no-op implementation -- makes
every request struct carry a non-nil field that a zero value cannot satisfy, and
`core.Status(ctx, core.StatusRequest{})` should work.
*/
type Events struct {
	Log      func(level Level, msg string)
	Progress func(p Progress)
}

func (e Events) logf(level Level, format string, args ...any) {
	if e.Log == nil {
		return
	}
	e.Log(level, fmt.Sprintf(format, args...))
}

func (e Events) progress(step, total int, what string) {
	if e.Progress == nil {
		return
	}
	e.Progress(Progress{Step: step, Total: total, What: what})
}

/*
Request is what every operation carries: which config, which game, and whether
it may use the network.

It is embedded rather than passed separately so that each operation has exactly
one argument, which is what makes the front ends mechanical.
*/
type Request struct {
	// ConfigPath overrides the config file location; "" uses the default.
	ConfigPath string
	// GameDir overrides both the config and the environment; "" resolves
	// normally.
	GameDir string
	// NoNetwork forbids network access for this operation.
	NoNetwork bool
	// Events receives log lines and progress. Both fields may be nil.
	Events Events
}

// session is the resolved state an operation works from.
type session struct {
	cfg     *config.Config
	paths   config.Paths
	install *steam.Install
	// installErr is why the game was not found, if it was not. Most operations
	// report it rather than failing: `status` on a machine with no game
	// installed is a legitimate thing to run.
	installErr error
	gameSource string
}

// open resolves configuration and the game install for one operation.
func open(req Request) (*session, error) {
	var (
		cfg *config.Config
		err error
	)
	if req.ConfigPath != "" {
		cfg, err = config.LoadFrom(req.ConfigPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}

	s := &session{cfg: cfg, paths: cfg.Paths()}

	dir, source := cfg.ResolvedGameDir()
	if req.GameDir != "" {
		dir, source = config.ExpandPath(req.GameDir), "--game-dir"
	}
	s.gameSource = source
	s.install, s.installErr = steam.Locate(dir)

	// The compiler concurrency limit is process-wide, so it is set from the
	// configuration once, here, rather than by whichever operation happens to
	// start a conversion first.
	mbin.SetMaxProcesses(cfg.Workers())
	return s, nil
}

// requireInstall is for the operations that genuinely cannot proceed without
// the game: reading paks.
func (s *session) requireInstall() error {
	if s.installErr != nil {
		return s.installErr
	}
	return nil
}

// index builds or loads the pak index for this session.
func (s *session) index(ctx context.Context, ev Events) (*hgpak.Index, hgpak.IndexStats, error) {
	if err := s.requireInstall(); err != nil {
		return nil, hgpak.IndexStats{}, err
	}
	paks, err := s.install.PakFiles()
	if err != nil {
		return nil, hgpak.IndexStats{}, err
	}
	if err := config.MkdirAll(s.paths.Cache); err != nil {
		return nil, hgpak.IndexStats{}, err
	}
	idx, stats, err := hgpak.BuildIndex(ctx, s.paths.PakIndex, paks, s.cfg.Workers(),
		func(done, total int, what string) { ev.progress(done, total, "indexing "+what) })
	if err != nil {
		return nil, stats, err
	}
	ev.logf(LevelDebug, "pak index: %d paks, %d files, %d re-read, %d reused, %s",
		stats.Paks, stats.Files, stats.Reindexed, stats.Reused, stats.Duration.Round(1e6))
	return idx, stats, nil
}

// releaseClient builds the GitHub client for this session (R5.1).
func (s *session) releaseClient(req Request) *mbin.Client {
	return &mbin.Client{
		CachePath: s.paths.Releases,
		UserAgent: buildinfo.UserAgent(),
		// Honoured from the environment only, never persisted and never logged
		// (project rule: no credentials).
		Token:     os.Getenv("GITHUB_TOKEN"),
		NoNetwork: req.NoNetwork,
		Debug:     func(msg string) { req.Events.logf(LevelDebug, "%s", msg) },
	}
}

// countLuaScripts reports how many mod scripts the library holds (R7.2 Status).
// A missing library directory is zero, not an error: it is created by the first
// operation that puts something in it.
func countLuaScripts(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".lua" {
			n++
		}
	}
	return n
}

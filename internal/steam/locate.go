package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModsState says what GAMEDATA/MODS currently is (R3.4).
//
// The distinction matters because it is sometimes a symlink at a directory
// outside the game install; deploy (spec 004) must not replace a symlink the
// user put there on purpose without saying so.
const (
	ModsAbsent  = "absent"
	ModsDir     = "dir"
	ModsSymlink = "symlink"
)

// ErrNotFound reports that no candidate held an installed copy of the game.
var ErrNotFound = errors.New("no installation of No Man's Sky was found")

// Candidate is one place that was examined, and what was found there (R3.2).
// Every candidate is retained even after a hit so that `detect` can explain why
// it chose what it chose.
type Candidate struct {
	// Root is the Steam root, or "" for a directory given with --game-dir.
	Root string
	// LibraryDir is the Steam library the manifest came from.
	LibraryDir string
	// GameDir is where the game would be if it were installed here.
	GameDir string
	// Reason is empty when the candidate is valid, otherwise why it was rejected.
	Reason string
}

// Install describes a located game directory (R3.4).
type Install struct {
	Dir             string
	LibraryDir      string
	BuildID         string
	Name            string
	LastUpdated     string
	StateFlags      string
	PCBanksDir      string
	ModsDir         string
	ModsState       string
	ModsTarget      string
	ModSettingsPath string
	ModSettingsOK   bool
	CompatDataDir   string
	// Candidates is every place examined, in the order examined.
	Candidates []Candidate
}

// PakFiles lists the .pak archives in PCBANKS, sorted (R3.4).
//
// Sorted rather than in readdir order so that an index build, a report and a
// second run all agree on which pak "wins" a duplicate internal path.
func (in *Install) PakFiles() ([]string, error) {
	entries, err := os.ReadDir(in.PCBanksDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", in.PCBanksDir, err)
	}
	var paks []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pak") {
			continue
		}
		paks = append(paks, filepath.Join(in.PCBanksDir, e.Name()))
	}
	sort.Strings(paks)
	return paks, nil
}

// Roots lists the Steam roots to examine, in order (R3.1).
func Roots() []string {
	var roots []string
	if v := os.Getenv("STEAM_ROOT"); v != "" {
		roots = append(roots, v)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		roots = append(roots,
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".steam", "root"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
			filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
		)
	}
	return roots
}

// Libraries lists the library directories a Steam root knows about.
//
// The root itself is always first: it is a library whether or not it appears in
// libraryfolders.vdf, and on a machine with a single library that file sometimes
// lists it with a path that resolves to the same place through a symlink.
func Libraries(root string) []string {
	libs := []string{root}
	seen := map[string]bool{canonical(root): true}

	b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return libs
	}
	doc := parseVDF(string(b))
	folders := doc.child("libraryfolders")
	if folders == nil {
		// Very old clients wrote the entries at the document root.
		folders = doc
	}
	for _, entry := range folders.children {
		path := entry.node.values["path"]
		if path == "" {
			continue
		}
		if seen[canonical(path)] {
			continue
		}
		seen[canonical(path)] = true
		libs = append(libs, path)
	}
	return libs
}

// canonical is the key used to de-duplicate library paths. EvalSymlinks so that
// ~/.steam/root (a symlink to ~/.local/share/Steam on most installs) is not
// examined twice and reported twice in `detect`.
func canonical(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// Locate finds the game (R3.3).
//
// An explicit overrideDir skips discovery but is validated identically: a
// --game-dir pointing at a directory with no GAMEDATA/PCBANKS is a mistake worth
// naming, not a reason to silently fall back to whatever Steam has installed.
func Locate(overrideDir string) (*Install, error) {
	if overrideDir != "" {
		lib := libraryOf(overrideDir)
		in, cand := describe("", lib, overrideDir)
		in.Candidates = []Candidate{cand}
		if cand.Reason != "" {
			return nil, fmt.Errorf("%s: %s", overrideDir, cand.Reason)
		}
		// An override usually points at the Steam install anyway (it is how the
		// integration tests and a second library are addressed), so the
		// manifest is read when it is there. Without this, --game-dir silently
		// blanks the buildid that `status` reports.
		applyManifest(in, lib)
		return in, nil
	}

	var candidates []Candidate
	for _, root := range Roots() {
		if _, err := os.Stat(root); err != nil {
			candidates = append(candidates, Candidate{Root: root, Reason: "no such directory"})
			continue
		}
		for _, lib := range Libraries(root) {
			manifest := filepath.Join(lib, "steamapps", "appmanifest_"+AppID+".acf")
			b, err := os.ReadFile(manifest)
			if err != nil {
				candidates = append(candidates, Candidate{
					Root: root, LibraryDir: lib,
					Reason: "no appmanifest_" + AppID + ".acf",
				})
				continue
			}
			app := parseVDF(string(b)).child("AppState")
			if app == nil {
				candidates = append(candidates, Candidate{
					Root: root, LibraryDir: lib,
					Reason: "appmanifest has no AppState block",
				})
				continue
			}
			installDir := app.values["installdir"]
			if installDir == "" {
				candidates = append(candidates, Candidate{
					Root: root, LibraryDir: lib,
					Reason: "appmanifest has no installdir",
				})
				continue
			}
			gameDir := filepath.Join(lib, "steamapps", "common", installDir)
			in, cand := describe(root, lib, gameDir)
			candidates = append(candidates, cand)
			if cand.Reason != "" {
				continue
			}
			in.BuildID = app.values["buildid"]
			in.Name = app.values["name"]
			in.LastUpdated = app.values["LastUpdated"]
			in.StateFlags = app.values["StateFlags"]
			in.Candidates = candidates
			return in, nil
		}
	}

	err := fmt.Errorf("%w: looked in %d place(s); set %s or pass --game-dir",
		ErrNotFound, len(candidates), "NMSBONKER_GAME_DIR")
	return &Install{Candidates: candidates}, err
}

// libraryOf recovers the Steam library holding a game directory, on the
// assumption Steam's own layout holds: <library>/steamapps/common/<installdir>.
// It returns "" when the path is not shaped that way, and the caller then has
// no manifest to read -- which is correct for a game copied out of Steam.
func libraryOf(gameDir string) string {
	common := filepath.Dir(filepath.Clean(gameDir))
	if filepath.Base(common) != "common" {
		return ""
	}
	steamapps := filepath.Dir(common)
	if filepath.Base(steamapps) != "steamapps" {
		return ""
	}
	return filepath.Dir(steamapps)
}

// applyManifest fills the fields that only the appmanifest knows.
func applyManifest(in *Install, lib string) {
	if lib == "" {
		return
	}
	b, err := os.ReadFile(filepath.Join(lib, "steamapps", "appmanifest_"+AppID+".acf"))
	if err != nil {
		return
	}
	app := parseVDF(string(b)).child("AppState")
	if app == nil {
		return
	}
	in.BuildID = app.values["buildid"]
	in.Name = app.values["name"]
	in.LastUpdated = app.values["LastUpdated"]
	in.StateFlags = app.values["StateFlags"]
}

// describe fills in everything derivable from a game directory and reports
// whether it is actually a game directory.
func describe(root, lib, gameDir string) (*Install, Candidate) {
	cand := Candidate{Root: root, LibraryDir: lib, GameDir: gameDir}
	in := &Install{
		Dir:             gameDir,
		LibraryDir:      lib,
		PCBanksDir:      filepath.Join(gameDir, "GAMEDATA", "PCBANKS"),
		ModsDir:         filepath.Join(gameDir, "GAMEDATA", "MODS"),
		ModSettingsPath: filepath.Join(gameDir, "Binaries", "SETTINGS", "GCMODSETTINGS.MXML"),
		ModsState:       ModsAbsent,
	}
	if lib != "" {
		compat := filepath.Join(lib, "steamapps", "compatdata", AppID)
		if _, err := os.Stat(compat); err == nil {
			in.CompatDataDir = compat
		}
	}

	fi, err := os.Stat(in.PCBanksDir)
	switch {
	case err != nil:
		cand.Reason = "GAMEDATA/PCBANKS is missing"
		return in, cand
	case !fi.IsDir():
		cand.Reason = "GAMEDATA/PCBANKS is not a directory"
		return in, cand
	}

	// Lstat, not Stat: the whole point of ModsState is to distinguish a symlink
	// from the directory it points at.
	if li, err := os.Lstat(in.ModsDir); err == nil {
		switch {
		case li.Mode()&os.ModeSymlink != 0:
			in.ModsState = ModsSymlink
			if target, err := os.Readlink(in.ModsDir); err == nil {
				in.ModsTarget = target
			}
		case li.IsDir():
			in.ModsState = ModsDir
		}
	}
	if _, err := os.Stat(in.ModSettingsPath); err == nil {
		in.ModSettingsOK = true
	}
	return in, cand
}

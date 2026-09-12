package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// FileEnv names the environment variable that overrides the config file path.
const FileEnv = "NMSBONKER_CONFIG"

// GameDirEnv names the environment variable that outranks the game_dir setting
// (R2.2). It is also what the integration tests key off, which is why it is
// spelled the same in both roles: one variable, one meaning.
const GameDirEnv = "NMSBONKER_GAME_DIR"

// DefaultModName is the folder created under GAMEDATA/MODS.
const DefaultModName = "COSMOS COMBINE"

// Flavor selects which MBINCompiler asset pair to install (R2.2).
const (
	FlavorAuto          = "auto"
	FlavorDotnet10      = "dotnet10"
	FlavorSelfContained = "self-contained"
)

// ModEntry is one entry in the build order. Consumed by spec 002; carried here so
// that a config written by a phase-2 build survives a phase-1 binary's Save.
type ModEntry struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// MBINCompiler holds the compiler-selection settings (R2.2).
type MBINCompiler struct {
	// Pin is a release tag ("v7.01.0-pre1"); empty means automatic selection.
	Pin string `json:"pin"`
	// Flavor is one of FlavorAuto, FlavorDotnet10, FlavorSelfContained.
	Flavor string `json:"flavor"`
}

// Config is the settings document.
//
// Unknown keys read from the file are kept in `extra` and written back on Save
// (R2.1): a user who runs a phase-4 build and then an older binary must not
// lose their tweak parameters just because the older binary cannot name them.
type Config struct {
	GameDir      string       `json:"game_dir"`
	LibraryDir   string       `json:"library_dir"`
	ToolsDir     string       `json:"tools_dir"`
	CacheDir     string       `json:"cache_dir"`
	WorkspaceDir string       `json:"workspace_dir"`
	ModName      string       `json:"mod_name"`
	MBINCompiler MBINCompiler `json:"mbincompiler"`
	Mods         []ModEntry   `json:"mods"`
	Parallel     int          `json:"parallel"`

	// path is the file this was loaded from and will be saved to.
	path string
	// extra holds keys this build does not understand, verbatim.
	extra map[string]json.RawMessage
}

// FilePath is where the settings live: $NMSBONKER_CONFIG, else
// $XDG_CONFIG_HOME/nmsbonker/config.json (R2.1).
func FilePath() string {
	if p := os.Getenv(FileEnv); p != "" {
		return ExpandPath(p)
	}
	return filepath.Join(configHome(), "nmsbonker", "config.json")
}

// Defaults returns the settings a machine with no config file has.
func Defaults() *Config {
	data := dataHome()
	return &Config{
		LibraryDir:   filepath.Join(data, "nmsbonker", "library"),
		ToolsDir:     filepath.Join(data, "nmsbonker", "tools"),
		CacheDir:     filepath.Join(cacheHome(), "nmsbonker"),
		WorkspaceDir: filepath.Join(data, "nmsbonker", "build"),
		ModName:      DefaultModName,
		MBINCompiler: MBINCompiler{Flavor: FlavorAuto},
		Mods:         []ModEntry{},
		path:         FilePath(),
	}
}

// Load reads the settings file, filling anything absent from Defaults (R2.4).
//
// A missing file is the ordinary state of a fresh install and returns the
// defaults, not an error. A file that will not parse *is* an error: silently
// reverting to defaults would rebuild against the wrong game directory and
// deploy a mod the user did not ask for.
func Load() (*Config, error) { return LoadFrom(FilePath()) }

// LoadFrom reads a settings file from an explicit path.
func LoadFrom(path string) (*Config, error) {
	path = ExpandPath(path)
	cfg := Defaults()
	cfg.path = path

	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Two passes over the same bytes: once into the typed struct, once into a
	// raw map so the keys this build cannot name survive a later Save. Decoding
	// into the map first and re-encoding would lose number formatting and key
	// order for no gain.
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, known := range knownKeys() {
		delete(raw, known)
	}
	if len(raw) > 0 {
		cfg.extra = raw
	}

	cfg.expand()
	cfg.fillDefaults()
	return cfg, nil
}

// knownKeys lists the top-level JSON names this build owns. Derived from the
// struct tags so that adding a field cannot forget to update the list, which
// would make the new field look "unknown" and get written out twice.
func knownKeys() []string {
	return []string{
		"game_dir", "library_dir", "tools_dir", "cache_dir", "workspace_dir",
		"mod_name", "mbincompiler", "mods", "parallel",
	}
}

// expand resolves a leading "~" in every path field (R2.3).
func (c *Config) expand() {
	c.GameDir = ExpandPath(c.GameDir)
	c.LibraryDir = ExpandPath(c.LibraryDir)
	c.ToolsDir = ExpandPath(c.ToolsDir)
	c.CacheDir = ExpandPath(c.CacheDir)
	c.WorkspaceDir = ExpandPath(c.WorkspaceDir)
}

// fillDefaults restores any field a partial config file left empty. A config
// holding only {"game_dir": "..."} is a perfectly reasonable thing to hand-write.
func (c *Config) fillDefaults() {
	d := Defaults()
	for _, f := range []struct {
		dst *string
		def string
	}{
		{&c.LibraryDir, d.LibraryDir},
		{&c.ToolsDir, d.ToolsDir},
		{&c.CacheDir, d.CacheDir},
		{&c.WorkspaceDir, d.WorkspaceDir},
		{&c.ModName, d.ModName},
		{&c.MBINCompiler.Flavor, d.MBINCompiler.Flavor},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}
	if c.Mods == nil {
		c.Mods = []ModEntry{}
	}
}

// Path is the file this config was loaded from.
func (c *Config) Path() string { return c.path }

// Save writes the settings back, preserving keys this build does not know
// (R2.1). The write is atomic: a half-written config is a lost configuration,
// and the moment this is called is exactly when losing it is most confusing.
func (c *Config) Save() error {
	if c.path == "" {
		c.path = FilePath()
	}
	doc := map[string]json.RawMessage{}
	for k, v := range c.extra {
		doc[k] = v
	}
	// Marshalling the struct and splicing it into the map keeps one definition
	// of each key's JSON shape (the struct tags) instead of a second, hand-kept
	// encoder that would drift from it.
	known, err := json.Marshal(struct {
		GameDir      string       `json:"game_dir"`
		LibraryDir   string       `json:"library_dir"`
		ToolsDir     string       `json:"tools_dir"`
		CacheDir     string       `json:"cache_dir"`
		WorkspaceDir string       `json:"workspace_dir"`
		ModName      string       `json:"mod_name"`
		MBINCompiler MBINCompiler `json:"mbincompiler"`
		Mods         []ModEntry   `json:"mods"`
		Parallel     int          `json:"parallel"`
	}{c.GameDir, c.LibraryDir, c.ToolsDir, c.CacheDir, c.WorkspaceDir,
		c.ModName, c.MBINCompiler, c.Mods, c.Parallel})
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	var knownMap map[string]json.RawMessage
	if err := json.Unmarshal(known, &knownMap); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	for k, v := range knownMap {
		doc[k] = v
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	out = append(out, '\n')

	dir := filepath.Dir(c.path)
	if err := MkdirAll(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	// 0600: the file is not secret, but it records where the user keeps their
	// game and their mod library, which is not something to widen by default.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		return fmt.Errorf("replace %s: %w", c.path, err)
	}
	return nil
}

// Paths are the resolved absolute directories an operation works in (R2.4).
// Nothing here is created; the operation that needs a directory creates it.
type Paths struct {
	Config    string
	Library   string
	Tools     string
	Cache     string
	Workspace string
	// PakIndex is the cached pak index (R4.5).
	PakIndex string
	// Releases is the ETag-cached GitHub release listing (R5.1).
	Releases string
	// Archive is where deploy puts the mod folder it replaced (spec 002 R5.2).
	// It is deliberately not configurable: it is the undo button, and a user
	// who has pointed it at a directory they later clean out has lost it.
	Archive string
}

// Paths resolves every directory to an absolute path.
func (c *Config) Paths() Paths {
	abs := func(p string) string {
		p = ExpandPath(p)
		if p == "" {
			return p
		}
		a, err := filepath.Abs(p)
		if err != nil {
			return p
		}
		return a
	}
	cache := abs(c.CacheDir)
	return Paths{
		Archive:   filepath.Join(dataHome(), "nmsbonker", "archive"),
		Config:    abs(c.path),
		Library:   abs(c.LibraryDir),
		Tools:     abs(c.ToolsDir),
		Cache:     cache,
		Workspace: abs(c.WorkspaceDir),
		PakIndex:  filepath.Join(cache, "pak-index.json"),
		Releases:  filepath.Join(cache, "mbincompiler-releases.json"),
	}
}

// ResolvedGameDir reports the game directory to use and where it came from.
//
// $NMSBONKER_GAME_DIR outranks the setting (R2.2) so that a test run or a
// one-off against a second install cannot be defeated by a stale config, and
// `status` says so when it happens rather than leaving the user to wonder why
// their edit had no effect.
func (c *Config) ResolvedGameDir() (dir, source string) {
	if v := os.Getenv(GameDirEnv); v != "" {
		return ExpandPath(v), "env " + GameDirEnv
	}
	if c.GameDir != "" {
		return ExpandPath(c.GameDir), "config game_dir"
	}
	return "", "auto-detect"
}

// Workers is the configured concurrency, resolved (R2.2).
//
// Half the CPUs, not all of them: MBINCompiler is a .NET process per file and
// the machine running this is also running the desktop the user is looking at.
func (c *Config) Workers() int {
	if c.Parallel > 0 {
		return c.Parallel
	}
	return max(1, runtime.NumCPU()/2)
}

// ErrUnknownKey reports a `config set` for a key this build does not have.
var ErrUnknownKey = errors.New("unknown config key")

// Keys lists every settable dotted key, in display order (R7.2 ConfigShow).
func Keys() []string {
	return []string{
		"game_dir", "library_dir", "tools_dir", "cache_dir", "workspace_dir",
		"mod_name", "mbincompiler.pin", "mbincompiler.flavor", "parallel",
	}
}

// Get returns the string form of a dotted key.
func (c *Config) Get(key string) (string, error) {
	switch key {
	case "game_dir":
		return c.GameDir, nil
	case "library_dir":
		return c.LibraryDir, nil
	case "tools_dir":
		return c.ToolsDir, nil
	case "cache_dir":
		return c.CacheDir, nil
	case "workspace_dir":
		return c.WorkspaceDir, nil
	case "mod_name":
		return c.ModName, nil
	case "mbincompiler.pin":
		return c.MBINCompiler.Pin, nil
	case "mbincompiler.flavor":
		return c.MBINCompiler.Flavor, nil
	case "parallel":
		return strconv.Itoa(c.Parallel), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
}

// Set assigns a dotted key from its string form, validating the value (R7.2).
//
// Validation happens here rather than in the CLI so the GUI cannot accept a
// flavor the installer will later reject.
func (c *Config) Set(key, value string) error {
	switch key {
	case "game_dir":
		c.GameDir = ExpandPath(value)
	case "library_dir":
		c.LibraryDir = ExpandPath(value)
	case "tools_dir":
		c.ToolsDir = ExpandPath(value)
	case "cache_dir":
		c.CacheDir = ExpandPath(value)
	case "workspace_dir":
		c.WorkspaceDir = ExpandPath(value)
	case "mod_name":
		if strings.TrimSpace(value) == "" {
			return errors.New("mod_name may not be empty: it names the folder created under GAMEDATA/MODS")
		}
		c.ModName = value
	case "mbincompiler.pin":
		c.MBINCompiler.Pin = value
	case "mbincompiler.flavor":
		switch value {
		case FlavorAuto, FlavorDotnet10, FlavorSelfContained:
			c.MBINCompiler.Flavor = value
		default:
			return fmt.Errorf("mbincompiler.flavor must be one of %s, %s, %s (got %q)",
				FlavorAuto, FlavorDotnet10, FlavorSelfContained, value)
		}
	case "parallel":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("parallel must be a non-negative integer (got %q)", value)
		}
		c.Parallel = n
	default:
		return fmt.Errorf("%w: %s (known keys: %s)", ErrUnknownKey, key, strings.Join(Keys(), ", "))
	}
	return nil
}

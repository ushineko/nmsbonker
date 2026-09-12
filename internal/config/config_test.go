package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/config"
)

// isolate points every XDG base directory at a scratch tree so a test can never
// read or write the developer's real configuration.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(config.FileEnv, "")
	t.Setenv(config.GameDirEnv, "")
	return root
}

// A machine that has never run the tool has no config file, and every command
// has to work anyway. Returning an error here would make `status` -- the command
// you run to find out what is wrong -- the command that cannot run. R2.1.
func TestAMissingConfigFileYieldsDefaults(t *testing.T) {
	root := isolate(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, config.DefaultModName, cfg.ModName)
	require.Equal(t, config.FlavorAuto, cfg.MBINCompiler.Flavor)
	require.Empty(t, cfg.GameDir, "an unset game_dir means auto-detect, not an error")

	paths := cfg.Paths()
	require.Equal(t, filepath.Join(root, "data", "nmsbonker", "library"), paths.Library)
	require.Equal(t, filepath.Join(root, "data", "nmsbonker", "tools"), paths.Tools)
	require.Equal(t, filepath.Join(root, "cache", "nmsbonker"), paths.Cache)
	require.Equal(t, filepath.Join(root, "data", "nmsbonker", "build"), paths.Workspace)
	require.Equal(t, filepath.Join(root, "cache", "nmsbonker", "pak-index.json"), paths.PakIndex)

	// R2.4: Load creates nothing. Directories appear when an operation needs one.
	require.NoDirExists(t, paths.Library)
	require.NoDirExists(t, paths.Cache)
}

// A newer build writes keys this one cannot name -- tweak parameters, deploy
// history. Dropping them on Save would silently reset a user's settings every
// time they ran an older binary, and they would have no way to tell. R2.1.
func TestSavePreservesKeysThisBuildDoesNotUnderstand(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.FileEnv, path)

	require.NoError(t, os.WriteFile(path, []byte(`{
	  "mod_name": "MY COMBINE",
	  "tweaks": {"nanite_multiplier": 50},
	  "future_key": ["a", "b"]
	}`), 0o600))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "MY COMBINE", cfg.ModName)

	cfg.Parallel = 4
	require.NoError(t, cfg.Save())

	var doc map[string]json.RawMessage
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &doc))
	require.JSONEq(t, `{"nanite_multiplier": 50}`, string(doc["tweaks"]))
	require.JSONEq(t, `["a","b"]`, string(doc["future_key"]))
	require.JSONEq(t, `4`, string(doc["parallel"]))
	require.JSONEq(t, `"MY COMBINE"`, string(doc["mod_name"]))
}

// A tilde typed into a GUI field or quoted on a command line arrives literally.
// Without expansion the tool would create a directory named "~" -- which is the
// trap CheckCreatablePath exists to refuse. R2.3.
func TestALeadingTildeIsExpandedOnLoadAndOnSet(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.FileEnv, path)
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path,
		[]byte(`{"game_dir": "~/Games/NMS", "library_dir": "~/mods"}`), 0o600))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "Games", "NMS"), cfg.GameDir)
	require.Equal(t, filepath.Join(home, "mods"), cfg.LibraryDir)

	require.NoError(t, cfg.Set("workspace_dir", "~/build"))
	require.Equal(t, filepath.Join(home, "build"), cfg.WorkspaceDir)
}

// A directory literally named "~" is a trap: from its parent the obvious way to
// remove it is `rm -rf ~`, which the shell expands to the home directory before
// rm runs. angou created one of these once. R2.3.
func TestNothingIsCreatedUnderABareTildeComponent(t *testing.T) {
	isolate(t)
	dir := filepath.Join(t.TempDir(), "~", "nmsbonker")
	require.ErrorIs(t, config.MkdirAll(dir), config.ErrTildeComponent)
	require.NoDirExists(t, dir)

	// A leading tilde is fine: it is expanded before it ever reaches here.
	require.NoError(t, config.CheckCreatablePath(config.ExpandPath("~/nmsbonker")))
}

// A stale game_dir in a config file must not be able to defeat an explicit
// environment override -- that is how the integration tests and a second
// install are pointed at, and silently ignoring it would waste an afternoon.
// R2.2.
func TestTheGameDirEnvironmentVariableOutranksTheSetting(t *testing.T) {
	isolate(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.GameDir = "/from/config"

	dir, source := cfg.ResolvedGameDir()
	require.Equal(t, "/from/config", dir)
	require.Equal(t, "config game_dir", source)

	t.Setenv(config.GameDirEnv, "/from/env")
	dir, source = cfg.ResolvedGameDir()
	require.Equal(t, "/from/env", dir)
	require.Contains(t, source, config.GameDirEnv, "status has to say why the setting was ignored")

	cfg.GameDir = ""
	t.Setenv(config.GameDirEnv, "")
	dir, source = cfg.ResolvedGameDir()
	require.Empty(t, dir)
	require.Equal(t, "auto-detect", source)
}

// `config set` is the GUI's write path too, so validation lives in the config
// package. A flavor the installer will later reject must be refused at the
// point the user typed it, not two commands later. R7.2.
func TestConfigSetValidatesValuesRatherThanStoringNonsense(t *testing.T) {
	isolate(t)
	cfg, err := config.Load()
	require.NoError(t, err)

	require.NoError(t, cfg.Set("mbincompiler.flavor", config.FlavorDotnet10))
	require.Equal(t, config.FlavorDotnet10, cfg.MBINCompiler.Flavor)
	require.Error(t, cfg.Set("mbincompiler.flavor", "wine"))
	require.Equal(t, config.FlavorDotnet10, cfg.MBINCompiler.Flavor, "a rejected value must not be stored")

	require.NoError(t, cfg.Set("parallel", "6"))
	require.Equal(t, 6, cfg.Workers())
	require.Error(t, cfg.Set("parallel", "-1"))
	require.Error(t, cfg.Set("parallel", "lots"))

	require.Error(t, cfg.Set("mod_name", "  "), "an empty mod name would name the deploy folder")
	require.ErrorIs(t, cfg.Set("no_such_key", "x"), config.ErrUnknownKey)

	// Every key `config show` lists must be readable and settable.
	for _, key := range config.Keys() {
		v, err := cfg.Get(key)
		require.NoError(t, err, key)
		require.NoError(t, cfg.Set(key, v), key)
	}
}

// Zero means "decide for me", and the decision is half the CPUs: MBINCompiler
// is a .NET process per file and the machine is also running the desktop the
// user is looking at. R2.2.
func TestParallelZeroMeansHalfTheCPUs(t *testing.T) {
	isolate(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	require.Zero(t, cfg.Parallel)
	require.GreaterOrEqual(t, cfg.Workers(), 1)
}

// A config that will not parse is not the same as one that is absent: it holds
// a game_dir and a mod list the user set on purpose, and quietly reverting to
// defaults would rebuild against the wrong install and deploy under the wrong
// name. R2.1.
func TestARuinedConfigFileIsAnErrorRatherThanSilentDefaults(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.FileEnv, path)
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	_, err := config.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), path)
}

// A partial config -- the shape a user hand-writes -- must not blank out every
// directory it did not mention.
func TestAPartialConfigKeepsTheDefaultsForEverythingElse(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.FileEnv, path)
	require.NoError(t, os.WriteFile(path, []byte(`{"game_dir":"/opt/nms"}`), 0o600))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "/opt/nms", cfg.GameDir)
	require.Equal(t, config.DefaultModName, cfg.ModName)
	require.Equal(t, filepath.Join(root, "data", "nmsbonker", "tools"), cfg.Paths().Tools)
}

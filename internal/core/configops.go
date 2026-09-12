package core

import (
	"context"

	"github.com/ushineko/nmsbonker/internal/config"
)

// ConfigShowRequest asks for the current settings.
type ConfigShowRequest struct {
	Request
}

// ConfigEntry is one settable key and its value.
type ConfigEntry struct {
	Key   string
	Value string
}

// ConfigShowResult is the settings as the tool sees them (R7.2).
type ConfigShowResult struct {
	Path    string
	Exists  bool
	Entries []ConfigEntry
	Paths   config.Paths
	// GameDir and GameDirSource show the resolved answer, which is not always
	// the game_dir value: the environment outranks it (R2.2).
	GameDir       string
	GameDirSource string
}

// ConfigShow reports the settings (R7.2).
func ConfigShow(_ context.Context, req ConfigShowRequest) (ConfigShowResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ConfigShowResult{}, err
	}
	out := ConfigShowResult{Path: s.cfg.Path(), Paths: s.paths}
	out.GameDir, out.GameDirSource = s.cfg.ResolvedGameDir()
	for _, key := range config.Keys() {
		value, err := s.cfg.Get(key)
		if err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, ConfigEntry{Key: key, Value: value})
	}
	return out, nil
}

// ConfigSetRequest assigns one dotted key.
type ConfigSetRequest struct {
	Request
	Key   string
	Value string
}

// ConfigSetResult reports the change that was written.
type ConfigSetResult struct {
	Path      string
	Key       string
	Old       string
	New       string
	Unchanged bool
}

// ConfigSet writes one setting (R7.2).
//
// Validation lives in the config package, not here, so that the GUI's settings
// panel and this command reject the same values for the same reasons.
func ConfigSet(_ context.Context, req ConfigSetRequest) (ConfigSetResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ConfigSetResult{}, err
	}
	old, err := s.cfg.Get(req.Key)
	if err != nil {
		return ConfigSetResult{}, err
	}
	if err := s.cfg.Set(req.Key, req.Value); err != nil {
		return ConfigSetResult{}, err
	}
	updated, err := s.cfg.Get(req.Key)
	if err != nil {
		return ConfigSetResult{}, err
	}
	if err := s.cfg.Save(); err != nil {
		return ConfigSetResult{}, err
	}
	return ConfigSetResult{
		Path:      s.cfg.Path(),
		Key:       req.Key,
		Old:       old,
		New:       updated,
		Unchanged: old == updated,
	}, nil
}

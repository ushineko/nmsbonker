package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ushineko/nmsbonker/internal/config"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

// Mod statuses reported by ListMods (spec 002 R6.1).
const (
	// ModOK means the library holds the script this entry names.
	ModOK = "ok"
	// ModMissing means it does not. The entry is kept: a file moved out of the
	// library for ten minutes should not lose its place in the build order.
	ModMissing = "missing"
)

// ModInfo is one entry in the build order.
type ModInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
}

// ListModsRequest asks for the reconciled build order.
type ListModsRequest struct {
	Request
}

// ListModsResult is the build order, with anything new or missing called out.
type ListModsResult struct {
	LibraryDir string    `json:"libraryDir"`
	Mods       []ModInfo `json:"mods"`
	// Notices describe what reconciling changed, for the front end to show.
	Notices []string `json:"notices,omitempty"`
}

/*
ListMods reconciles the configured build order with the library (R6.1).

Two directions, handled differently on purpose. A script in the library that the
config has never seen is appended *disabled*, because a file appearing in a
directory is not consent to build it into the mod the user is about to install.
A configured entry whose file is gone is kept and reported missing, because the
usual cause is a file being edited or moved for a moment and silently dropping
it would lose the position it holds in a hand-tuned load order.
*/
func ListMods(_ context.Context, req ListModsRequest) (ListModsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListModsResult{}, err
	}
	out, changed, err := reconcile(s)
	if err != nil {
		return out, err
	}
	if changed {
		if err := s.cfg.Save(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// libraryScripts lists the .lua files in the library, by name.
func libraryScripts(dir string) (map[string]os.FileInfo, []string, error) {
	found := map[string]os.FileInfo{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return found, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".lua") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		found[name] = info
		names = append(names, name)
	}
	sort.Strings(names)
	return found, names, nil
}

// reconcile builds the mod list and reports whether the config changed.
func reconcile(s *session) (ListModsResult, bool, error) {
	out := ListModsResult{LibraryDir: s.paths.Library}
	found, names, err := libraryScripts(s.paths.Library)
	if err != nil {
		return out, false, err
	}

	known := map[string]bool{}
	for _, entry := range s.cfg.Mods {
		known[entry.Name] = true
		info := ModInfo{Name: entry.Name, Enabled: entry.Enabled, Status: ModMissing}
		if fi, ok := found[entry.Name]; ok {
			info.Status = ModOK
			info.Path = filepath.Join(s.paths.Library, entry.Name+".lua")
			info.Size = fi.Size()
		}
		out.Mods = append(out.Mods, info)
	}

	var added []string
	for _, name := range names {
		if known[name] {
			continue
		}
		added = append(added, name)
		s.cfg.Mods = append(s.cfg.Mods, config.ModEntry{Name: name, Enabled: false})
		out.Mods = append(out.Mods, ModInfo{
			Name: name, Enabled: false, Status: ModOK,
			Path: filepath.Join(s.paths.Library, name+".lua"), Size: found[name].Size(),
		})
	}
	if len(added) > 0 {
		out.Notices = append(out.Notices, fmt.Sprintf(
			"added %d script(s) found in the library, disabled: %s", len(added), strings.Join(added, ", ")))
	}
	var missing []string
	for _, m := range out.Mods {
		if m.Status == ModMissing {
			missing = append(missing, m.Name)
		}
	}
	if len(missing) > 0 {
		out.Notices = append(out.Notices, fmt.Sprintf(
			"%d configured mod(s) have no .lua in the library: %s", len(missing), strings.Join(missing, ", ")))
	}
	return out, len(added) > 0, nil
}

// AddModRequest copies scripts into the library (R6.2).
//
// Paths rather than the spec's single Path: `mods add a.lua b.lua` is one
// user action and should be one operation, so that the GUI's file picker --
// which is multi-select -- does not need a loop the CLI does not have.
type AddModRequest struct {
	Request
	Paths []string
	// Replace allows overwriting a script of the same name.
	Replace bool
	// Enabled is the state new entries take. An explicit add is an intent to
	// use the mod, so callers normally pass true.
	Enabled bool
}

// AddModResult says what landed in the library.
type AddModResult struct {
	LibraryDir string   `json:"libraryDir"`
	Added      []string `json:"added"`
	Replaced   []string `json:"replaced"`
}

// AddMod copies .lua scripts into the library and puts them in the build order.
func AddMod(_ context.Context, req AddModRequest) (AddModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return AddModResult{}, err
	}
	out := AddModResult{LibraryDir: s.paths.Library}
	if err := config.MkdirAll(s.paths.Library); err != nil {
		return out, err
	}
	for _, p := range req.Paths {
		name, replaced, err := s.addOne(config.ExpandPath(p), req.Replace, req.Enabled)
		if err != nil {
			return out, err
		}
		out.Added = append(out.Added, name)
		if replaced {
			out.Replaced = append(out.Replaced, name)
		}
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *session) addOne(src string, replace, enabled bool) (name string, replaced bool, err error) {
	base := filepath.Base(src)
	if !strings.EqualFold(filepath.Ext(base), ".lua") {
		return "", false, fmt.Errorf("%s is not a .lua mod script", src)
	}
	name = strings.TrimSuffix(base, filepath.Ext(base))
	dest := filepath.Join(s.paths.Library, name+".lua")
	if _, err := os.Stat(dest); err == nil {
		if !replace {
			return "", false, fmt.Errorf("%s is already in the library; pass --replace to overwrite it", name)
		}
		replaced = true
	}
	if err := copyFile(src, dest); err != nil {
		return "", false, err
	}
	for _, e := range s.cfg.Mods {
		if e.Name == name {
			return name, replaced, nil
		}
	}
	s.cfg.Mods = append(s.cfg.Mods, config.ModEntry{Name: name, Enabled: enabled})
	return name, replaced, nil
}

// ImportDirRequest adds every matching script in a directory (R6.2).
type ImportDirRequest struct {
	Request
	Dir string
	// Pattern defaults to "*.lua".
	Pattern string
	Replace bool
	// Enabled is the state imported entries take; an import is an explicit
	// intent to build them.
	Enabled bool
}

// ImportDir copies a directory of scripts into the library, in name order.
func ImportDir(ctx context.Context, req ImportDirRequest) (AddModResult, error) {
	pattern := req.Pattern
	if pattern == "" {
		pattern = "*.lua"
	}
	dir := config.ExpandPath(req.Dir)
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return AddModResult{}, fmt.Errorf("match %s in %s: %w", pattern, dir, err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return AddModResult{}, fmt.Errorf("no files matching %s in %s", pattern, dir)
	}
	return AddMod(ctx, AddModRequest{
		Request: req.Request, Paths: matches, Replace: req.Replace, Enabled: req.Enabled,
	})
}

// RemoveModRequest takes a mod out of the build order (R6.2).
type RemoveModRequest struct {
	Request
	Name string
	// DeleteFile also removes the .lua from the library.
	DeleteFile bool
}

// RemoveModResult says what was removed.
type RemoveModResult struct {
	Name    string `json:"name"`
	Deleted string `json:"deleted,omitempty"`
}

// RemoveMod drops a config entry, and optionally the script itself.
func RemoveMod(_ context.Context, req RemoveModRequest) (RemoveModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RemoveModResult{}, err
	}
	out := RemoveModResult{Name: req.Name}
	idx := indexOfMod(s.cfg.Mods, req.Name)
	if idx < 0 {
		return out, fmt.Errorf("no mod named %q is in the build order", req.Name)
	}
	s.cfg.Mods = append(s.cfg.Mods[:idx], s.cfg.Mods[idx+1:]...)
	if req.DeleteFile {
		path := filepath.Join(s.paths.Library, req.Name+".lua")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return out, fmt.Errorf("remove %s: %w", path, err)
		}
		out.Deleted = path
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

// SetModEnabledRequest turns mods on or off (R6.2).
type SetModEnabledRequest struct {
	Request
	Names   []string
	Enabled bool
}

// SetModEnabledResult lists what changed.
type SetModEnabledResult struct {
	Changed []string `json:"changed"`
	Enabled bool     `json:"enabled"`
}

// SetModEnabled flips the enabled flag on one or more entries.
func SetModEnabled(_ context.Context, req SetModEnabledRequest) (SetModEnabledResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SetModEnabledResult{}, err
	}
	out := SetModEnabledResult{Enabled: req.Enabled}
	for _, name := range req.Names {
		idx := indexOfMod(s.cfg.Mods, name)
		if idx < 0 {
			return out, fmt.Errorf("no mod named %q is in the build order", name)
		}
		s.cfg.Mods[idx].Enabled = req.Enabled
		out.Changed = append(out.Changed, name)
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

// MoveModRequest changes a mod's position in the build order (R6.2).
//
// To is the 1-based position `mods list` shows, because that is the number the
// user is looking at when they decide to move something.
type MoveModRequest struct {
	Request
	Name string
	To   int
}

// MoveModResult reports the new order.
type MoveModResult struct {
	Name string   `json:"name"`
	From int      `json:"from"`
	To   int      `json:"to"`
	Mods []string `json:"mods"`
}

/*
MoveMod reorders the build.

Order is the conflict-resolution rule: later mods are applied later and win on a
value two mods both change. It is the only control the user has over two mods
that edit the same field, so it has to be adjustable without editing JSON.
*/
func MoveMod(_ context.Context, req MoveModRequest) (MoveModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return MoveModResult{}, err
	}
	from := indexOfMod(s.cfg.Mods, req.Name)
	if from < 0 {
		return MoveModResult{}, fmt.Errorf("no mod named %q is in the build order", req.Name)
	}
	to := req.To - 1
	if to < 0 || to >= len(s.cfg.Mods) {
		return MoveModResult{}, fmt.Errorf("position %d is outside 1..%d", req.To, len(s.cfg.Mods))
	}
	entry := s.cfg.Mods[from]
	rest := append(s.cfg.Mods[:from:from], s.cfg.Mods[from+1:]...)
	s.cfg.Mods = append(rest[:to:to], append([]config.ModEntry{entry}, rest[to:]...)...)

	out := MoveModResult{Name: req.Name, From: from + 1, To: req.To}
	for _, e := range s.cfg.Mods {
		out.Mods = append(out.Mods, e.Name)
	}
	if err := s.cfg.Save(); err != nil {
		return out, err
	}
	return out, nil
}

// ModCheck is one script's load result (R6.2).
type ModCheck struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	OK          bool     `json:"ok"`
	Error       string   `json:"error,omitempty"`
	ModFilename string   `json:"modFilename,omitempty"`
	Author      string   `json:"author,omitempty"`
	NMSVersion  string   `json:"nmsVersion,omitempty"`
	Targets     []string `json:"targets,omitempty"`
	Blocks      int      `json:"blocks"`
	Unsupported []string `json:"unsupported,omitempty"`
	// Globals are the script's tuning constants, rendered as the report would.
	Globals map[string]string `json:"globals,omitempty"`
	// Dump is the decoded container, present only when the caller asked.
	Dump json.RawMessage `json:"dump,omitempty"`
}

// CheckModsRequest loads every enabled script and reports what it says (R6.2).
type CheckModsRequest struct {
	Request
	// All checks disabled mods too.
	All bool
	// IncludeDump attaches the decoded container to each result.
	IncludeDump bool
}

// CheckModsResult is the per-script summary.
type CheckModsResult struct {
	Mods   []ModCheck `json:"mods"`
	OK     int        `json:"ok"`
	Failed int        `json:"failed"`
}

/*
CheckMods runs every script through the loader without building anything.

This is the sandbox's user-facing edge (AC7): a script that tries to call
os.execute is rejected here, by name, before anything reads its change tables.
It is also the fastest way to find out that a mod downloaded for an older game
version no longer parses.
*/
func CheckMods(ctx context.Context, req CheckModsRequest) (CheckModsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return CheckModsResult{}, err
	}
	list, _, err := reconcile(s)
	if err != nil {
		return CheckModsResult{}, err
	}
	var out CheckModsResult
	for _, m := range list.Mods {
		if !m.Enabled && !req.All {
			continue
		}
		check := ModCheck{Name: m.Name, Path: m.Path}
		switch {
		case m.Status == ModMissing:
			check.Error = "no .lua in the library"
		default:
			def, err := modscript.Load(ctx, m.Path)
			if err != nil {
				check.Error = err.Error()
			} else {
				check.OK = true
				check.ModFilename = def.ModFilename
				check.Author = def.Author
				check.NMSVersion = def.NMSVersion
				check.Targets = def.Targets()
				check.Blocks = def.BlockCount()
				check.Unsupported = def.UnsupportedKeys()
				if len(def.Globals) > 0 {
					check.Globals = map[string]string{}
					for k, v := range def.Globals {
						check.Globals[k] = v.String()
					}
				}
				if req.IncludeDump {
					check.Dump = modscript.DumpJSON(def)
				}
			}
		}
		if check.OK {
			out.OK++
		} else {
			out.Failed++
		}
		out.Mods = append(out.Mods, check)
	}
	return out, nil
}

func indexOfMod(mods []config.ModEntry, name string) int {
	for i, e := range mods {
		if e.Name == name {
			return i
		}
	}
	return -1
}

// copyFile copies a file's contents, creating or truncating the destination.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return nil
}

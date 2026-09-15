package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
Editing a library script in place (spec 010).

A mod is a .lua file, and the quickest tweak to one is a line changed in it. The
operations here read a script's text and write it back, with two guards: the
new text has to load through the sandbox (a script that does not load is a mod
that silently drops out of the next build), and the previous text is kept
beside the file as `<name>.lua.bak`, which the library listing ignores because
it only counts `.lua`.

Built-in tweaks are compiled into the binary and are not editable here; their
numbers are parameters, changed in Tweaks. Reading one is allowed, so a person
can see what a built-in does.
*/

// ErrReadOnlyScript reports a write to a built-in tweak.
var ErrReadOnlyScript = errors.New("built-in tweaks are compiled in and cannot be edited; change their parameters in Tweaks instead")

// ModScriptRequest names a mod.
type ModScriptRequest struct {
	Request
	Name string
}

// ModScriptResult is a script's text and where it came from.
type ModScriptResult struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	// Path is the library file, "" for a built-in.
	Path     string    `json:"path,omitempty"`
	Text     string    `json:"text"`
	Bytes    int       `json:"bytes"`
	Modified time.Time `json:"modified,omitempty"`
	// ReadOnly is true for a built-in.
	ReadOnly bool `json:"read_only"`
}

// ReadModScript returns a mod's script text (R1).
func ReadModScript(_ context.Context, req ModScriptRequest) (ModScriptResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ModScriptResult{}, err
	}
	m, err := s.findMod(req.Name)
	if err != nil {
		return ModScriptResult{}, err
	}
	if m.Status == ModMissing {
		return ModScriptResult{}, fmt.Errorf("%s: the script file is missing (%s)", m.Name, m.Path)
	}
	out := ModScriptResult{Name: m.Name, Source: m.Source, ReadOnly: m.Source == SourceBuiltin}
	if !out.ReadOnly {
		out.Path = m.Path
		if fi, err := os.Stat(m.Path); err == nil {
			out.Modified = fi.ModTime()
		}
	}
	src, err := rawScript(m)
	if err != nil {
		return out, err
	}
	out.Text = string(src)
	out.Bytes = len(src)
	return out, nil
}

// WriteModScriptRequest replaces a library script's text (R2).
type WriteModScriptRequest struct {
	Request
	Name string
	Text string
	// Check only loads the text through the sandbox and reports; nothing is
	// written.
	Check bool
	// Force writes text that does not load. The build will then report the
	// mod NOT BUILT, which is sometimes what a person wants while working on it.
	Force bool
}

// WriteModScriptResult says what happened.
type WriteModScriptResult struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Loads is whether the text loaded through the sandbox; LoadError says why
	// not.
	Loads     bool   `json:"loads"`
	LoadError string `json:"load_error,omitempty"`
	// Blocks is how many change blocks the loaded script declares.
	Blocks int `json:"blocks"`
	// Written is false for a check, and for text identical to the file.
	Written bool `json:"written"`
	// Backup is the previous text's file, "" when nothing was written.
	Backup string `json:"backup,omitempty"`
	Bytes  int    `json:"bytes"`
}

// WriteModScript validates the text and writes it over the library script,
// keeping the previous text as a .bak beside it.
func WriteModScript(ctx context.Context, req WriteModScriptRequest) (WriteModScriptResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return WriteModScriptResult{}, err
	}
	m, err := s.findMod(req.Name)
	if err != nil {
		return WriteModScriptResult{}, err
	}
	out := WriteModScriptResult{Name: m.Name, Path: m.Path, Bytes: len(req.Text)}
	if m.Source == SourceBuiltin {
		return out, fmt.Errorf("%w: %s", ErrReadOnlyScript, m.Name)
	}
	if m.Status == ModMissing {
		return out, fmt.Errorf("%s: the script file is missing (%s)", m.Name, m.Path)
	}

	def, err := modscript.LoadSource(ctx, m.Path, []byte(req.Text))
	if err != nil {
		out.LoadError = err.Error()
	} else {
		out.Loads = true
		out.Blocks = len(def.Modifications)
	}
	if req.Check {
		return out, nil
	}
	if !out.Loads && !req.Force {
		return out, fmt.Errorf("the script does not load and was not written: %s", out.LoadError)
	}

	current, err := os.ReadFile(m.Path)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", m.Path, err)
	}
	if string(current) == req.Text {
		return out, nil
	}
	out.Backup = m.Path + ".bak"
	// The path is the library file's own, from the reconciled build order,
	// with a fixed suffix; nothing of the request reaches it.
	if err := os.WriteFile(out.Backup, current, 0o600); err != nil { //nolint:gosec // see above
		return out, fmt.Errorf("keep the previous text: %w", err)
	}
	if err := replaceFile(m.Path, []byte(req.Text)); err != nil {
		return out, err
	}
	out.Written = true
	req.Events.logf(LevelInfo, "wrote %s (%d bytes); the previous text is %s", m.Path, len(req.Text), out.Backup)
	return out, nil
}

// rawScript is a mod's text as it is on disk or in the binary, without the
// parameter overrides scriptSource substitutes: the editor shows the file.
func rawScript(m ModInfo) ([]byte, error) {
	if m.Source == SourceBuiltin {
		b, ok := tweaks.Source(m.Name)
		if !ok {
			return nil, fmt.Errorf("no built-in tweak named %q", m.Name)
		}
		return b, nil
	}
	b, err := os.ReadFile(m.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", m.Path, err)
	}
	return b, nil
}

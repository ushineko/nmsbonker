package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ushineko/nmsbonker/internal/buildinfo"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

/*
Exporting a mod's script (spec 022).

nmsbonker already imports AMUMSS scripts; this is the other direction. An export
is the script exactly as the next build would load it -- the built-in or library
text with the user's parameter values written in -- so it can be edited by hand,
shared, or run by another AMUMSS toolchain.

A few lines are appended, after the script rather than before it: a library
script with no `@desc` takes its description from the comment block it opens
with, and an export imported back must describe itself the same way. They say
where the file came from, what the parameters were set to, and which keys in it
only nmsbonker understands.
*/

// ExportModRequest writes one mod's script to a file.
type ExportModRequest struct {
	Request
	Name string
	// Out is a directory (the file is Name.lua inside it) or a file path.
	// Empty means the current directory.
	Out string
	// Force overwrites an existing file.
	Force bool
}

// ExportModResult says what was written.
type ExportModResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
	// Params are the overrides written into the script, by parameter name.
	Params map[string]float64 `json:"params,omitempty"`
	// NmsbonkerOnly names the keys in the script other toolchains do not know.
	NmsbonkerOnly []string `json:"nmsbonkerOnly,omitempty"`
}

// ErrExportExists reports that the export would overwrite a file.
var ErrExportExists = errors.New("the file already exists")

// ExportMod writes a mod's script, as the build would load it, to a file.
func ExportMod(ctx context.Context, req ExportModRequest) (ExportModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ExportModResult{}, err
	}
	m, err := s.findMod(req.Name)
	if err != nil {
		return ExportModResult{}, err
	}
	if m.Status == ModMissing {
		return ExportModResult{}, fmt.Errorf("%s: the script file is missing (%s)", m.Name, m.Path)
	}
	src, _, err := s.scriptSource(m)
	if err != nil {
		return ExportModResult{}, err
	}

	out := ExportModResult{Name: m.Name, Params: s.cfg.ParamsFor(m.Name)}
	if def, err := modscript.LoadSource(ctx, scriptPath(m), src); err == nil {
		out.NmsbonkerOnly = nmsbonkerOnlyKeys(def)
	}

	out.Path = exportPath(req.Out, m.Name)
	if _, err := os.Stat(out.Path); err == nil && !req.Force {
		return out, fmt.Errorf("%w: %s", ErrExportExists, out.Path)
	}
	text := string(src)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += exportTrailer(m, out, time.Now())
	if err := os.WriteFile(out.Path, []byte(text), 0o644); err != nil { //nolint:gosec // a script the user asked to share
		return out, fmt.Errorf("export %s: %w", m.Name, err)
	}
	out.Bytes = len(text)
	return out, nil
}

// exportPath resolves Out: a directory gets Name.lua inside it.
func exportPath(dest, name string) string {
	if dest == "" {
		dest = "."
	}
	if fi, err := os.Stat(dest); (err == nil && fi.IsDir()) || strings.HasSuffix(dest, string(os.PathSeparator)) {
		return filepath.Join(dest, name+".lua")
	}
	return dest
}

// nmsbonkerOnlyKeys are the keys in a script that AMUMSS does not define:
// CURRENCY_MULT and WRAPPER_MULT come from the builder nmsbonker reimplements
// (spec 002), and CAP is nmsbonker's own ceiling (spec 005).
func nmsbonkerOnlyKeys(def *modscript.Definition) []string {
	seen := map[string]bool{}
	for _, mod := range def.Modifications {
		for _, ch := range mod.Changes {
			for _, b := range ch.Blocks {
				if b.CurrencyMult != nil {
					seen["CURRENCY_MULT"] = true
				}
				if b.WrapperMult != nil {
					seen["WRAPPER_MULT"] = true
				}
				if b.HasCap {
					seen["CAP"] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// exportTrailer is the comment appended to an export.
func exportTrailer(m ModInfo, res ExportModResult, now time.Time) string {
	var b strings.Builder
	kind := "library script"
	if m.Source == SourceBuiltin {
		kind = "built-in tweak"
	}
	fmt.Fprintf(&b, "\n-- Exported by nmsbonker %s on %s from the %s %s.\n",
		buildinfo.Version, now.Format("2006-01-02"), kind, m.Name)
	if len(res.Params) == 0 {
		b.WriteString("-- Parameters: the script's own values.\n")
	} else {
		names := make([]string, 0, len(res.Params))
		for k := range res.Params {
			names = append(names, k)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, k := range names {
			parts = append(parts, fmt.Sprintf("%s = %s", k, modscript.FormatValue(res.Params[k], "")))
		}
		fmt.Fprintf(&b, "-- Parameters as set in nmsbonker: %s.\n", strings.Join(parts, ", "))
	}
	if len(res.NmsbonkerOnly) > 0 {
		fmt.Fprintf(&b, "-- Keys only nmsbonker understands: %s. Another AMUMSS toolchain will\n"+
			"-- ignore or reject them, and the edits they make will not happen there.\n",
			strings.Join(res.NmsbonkerOnly, ", "))
	}
	return b.String()
}

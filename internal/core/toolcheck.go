package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/mbin"
)

// Compatibility verdicts (spec 002 R3.4).
const (
	// CompatOK means every probe file decompiled, recompiled and came back
	// byte-identical outside the header.
	CompatOK = "compatible"
	// CompatMismatch means at least one did not. Building is still allowed;
	// the report and the CLI say so.
	CompatMismatch = "mismatch"
	// CompatNoCompiler means there is nothing installed to check with.
	CompatNoCompiler = "no-compiler"
)

/*
ToolCheckFiles are the two game files the compatibility check round-trips.

One globals file and one table, chosen because they exercise different template
families and because every mod library edits at least one of them. Both are read
out of the paks at check time; neither is committed.
*/
//
//nolint:gochecknoglobals // a fixed list, read-only
var ToolCheckFiles = []string{
	"gcgameplayglobals.global.mbin",
	"metadata/reality/tables/rewardtable.mbin",
}

// ToolCheckRequest asks whether the installed compiler matches this install.
type ToolCheckRequest struct {
	Request
	// Files overrides ToolCheckFiles, for tests and for a user chasing one
	// template that will not build.
	Files []string
}

// ToolCheckResult is the compatibility verdict and its evidence (R3.4).
type ToolCheckResult struct {
	Status string `json:"status"`
	// Detail is a one-line summary suitable for the build report's header.
	Detail          string            `json:"detail,omitempty"`
	Compiler        string            `json:"compiler,omitempty"`
	CompilerVersion string            `json:"compilerVersion,omitempty"`
	Files           []mbin.RoundTrip  `json:"files"`
	Advice          string            `json:"advice,omitempty"`
	Skipped         map[string]string `json:"skipped,omitempty"`
}

/*
ToolCheck round-trips known game files through the installed compiler (R3.4).

Spec 001 tried to establish compatibility from a version string and could not:
the game's MBINs are not MBINCompiler output and carry no libMBIN version. This
asks the question that matters instead -- can this compiler read this install's
files and write them back unchanged -- and answers it by doing exactly that. A
mismatch does not forbid a build; it means the build's outputs are suspect and
the report says which file proved it.
*/
func ToolCheck(ctx context.Context, req ToolCheckRequest) (ToolCheckResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ToolCheckResult{}, err
	}
	return toolCheck(ctx, s, req.Events, req.Files)
}

func toolCheck(ctx context.Context, s *session, ev Events, files []string) (ToolCheckResult, error) {
	out := ToolCheckResult{Status: CompatNoCompiler}
	if len(files) == 0 {
		files = ToolCheckFiles
	}

	compiler, err := mbin.Locate(s.paths.Tools, s.cfg.MBINCompiler.Pin)
	if err != nil {
		out.Detail = "no MBINCompiler is installed"
		out.Advice = "run `nmsbonker tools ensure`"
		return out, nil
	}
	out.Compiler = compiler.Bin
	out.CompilerVersion = compiler.Tag
	if v, err := compiler.Version(ctx); err == nil {
		out.CompilerVersion = v
	}

	if err := s.requireInstall(); err != nil {
		out.Detail = err.Error()
		return out, nil
	}
	idx, _, err := s.index(ctx, ev)
	if err != nil {
		return out, err
	}

	dir, err := os.MkdirTemp("", "nmsbonker-roundtrip-")
	if err != nil {
		return out, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	out.Status = CompatOK
	var failures []string
	for i, name := range files {
		probe, err := extractProbe(idx, name, filepath.Join(dir, fmt.Sprintf("in%d", i)))
		if err != nil {
			if out.Skipped == nil {
				out.Skipped = map[string]string{}
			}
			out.Skipped[name] = err.Error()
			ev.logf(LevelWarn, "compatibility check: %s: %v", name, err)
			continue
		}
		rt, err := compiler.CheckRoundTrip(ctx, probe, filepath.Join(dir, fmt.Sprintf("work%d", i)))
		if err != nil {
			return out, err
		}
		out.Files = append(out.Files, rt)
		if !rt.OK {
			failures = append(failures, rt.Reason)
		}
		ev.logf(LevelDebug, "compatibility check: %s ok=%v %s", name, rt.OK, rt.Reason)
	}

	switch {
	case len(failures) > 0:
		out.Status = CompatMismatch
		out.Detail = strings.Join(failures, "; ")
		out.Advice = "install the newest MBINCompiler (`nmsbonker tools ensure`) " +
			"or pin one that matches this game version (`nmsbonker tools pin <tag>`)"
	case len(out.Files) == 0:
		out.Status = CompatNoCompiler
		out.Detail = "none of the probe files could be read out of the paks"
	default:
		out.Detail = fmt.Sprintf("%d file(s) round-tripped byte-identical outside the header", len(out.Files))
	}
	return out, nil
}

// extractProbe pulls one file out of the paks into dir, named so that
// MBINCompiler will accept it (the `.MBIN` extension is load-bearing).
func extractProbe(idx *hgpak.Index, name, dir string) (string, error) {
	loc, _, ok := idx.Resolve(name)
	if !ok {
		return "", fmt.Errorf("%w: %s", hgpak.ErrNotFound, name)
	}
	pak, err := hgpak.Open(loc.Pak)
	if err != nil {
		return "", err
	}
	defer func() { _ = pak.Close() }()
	data, err := pak.ReadFile(loc.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	base := hgpak.Base(loc.Name)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	path := filepath.Join(dir, base+".MBIN")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

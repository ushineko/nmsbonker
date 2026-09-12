package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
Where a mod's script text comes from, and what the user's overrides did to it
(spec 004 R1.3).

One function, used by the build, by `mods check` and by the Tweaks section, so
that the three of them cannot disagree about what a parameter is currently set
to. Everything downstream -- the Lua loader, the plan, the merge -- sees a
script that already has the user's numbers in it and needs to know nothing about
parameters at all.

The file on disk is never modified. A built-in is embedded and cannot be; a
library script belongs to whoever wrote it, and rewriting someone's download in
place would make "reset to defaults" a lie.
*/

// scriptPath is what a load failure should name. A built-in has no path, so it
// is named by its file as it would be on disk, which is what the user sees in
// the interface.
func scriptPath(m ModInfo) string {
	if m.Path != "" {
		return m.Path
	}
	return filepath.Join("(built-in)", m.Name+".lua")
}

/*
scriptSource returns one mod's script with the configured overrides applied, and
the parameters it declares with their current values filled in.

The parameter list comes from the *original* source rather than the rewritten
one, so an override that has drifted outside a declared range still reports the
range it drifted out of.
*/
func (s *session) scriptSource(m ModInfo) ([]byte, []modscript.Param, error) {
	var src []byte
	if m.Source == SourceBuiltin {
		b, ok := tweaks.Source(m.Name)
		if !ok {
			return nil, nil, fmt.Errorf("no built-in tweak named %q", m.Name)
		}
		src = b
	} else {
		b, err := os.ReadFile(m.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", m.Path, err)
		}
		src = b
	}

	params := modscript.Parameters(src)
	overrides := s.cfg.ParamsFor(m.Name)
	if len(overrides) == 0 {
		return src, params, nil
	}

	kinds := make(map[string]string, len(params))
	for i := range params {
		kinds[params[i].Name] = params[i].Kind
		if v, ok := overrides[params[i].Name]; ok {
			params[i].Current = v
		}
	}
	out, err := modscript.OverrideAll(src, overrides, kinds)
	if err != nil {
		return nil, params, fmt.Errorf("%s: %w", m.Name, err)
	}
	return out, params, nil
}

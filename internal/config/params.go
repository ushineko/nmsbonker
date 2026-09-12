package config

import "sort"

/*
Per-mod parameter overrides (spec 004 R1.3).

The document is `params: {"<mod name>": {"<GLOBAL>": <number>}}` and it is
deliberately sparse: only the values the user has actually changed are stored,
so "reset to defaults" is a deletion rather than a second copy of the script's
own numbers. That also means a built-in whose default changes in a later release
moves for every user who never touched it, which is the behaviour you want from
a default.

The accessors live here rather than in core so that both front ends and the
config file agree about what an absent value means, and so a stray empty map
does not end up written into the file as `"SomeMod": {}`.
*/

// ParamsFor returns the overrides recorded for one mod. The result is a copy:
// callers pass it to the script rewriter, which must not be able to edit the
// settings document by accident.
func (c *Config) ParamsFor(mod string) map[string]float64 {
	src := c.Params[mod]
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// SetParam records one override.
func (c *Config) SetParam(mod, name string, value float64) {
	if c.Params == nil {
		c.Params = map[string]map[string]float64{}
	}
	if c.Params[mod] == nil {
		c.Params[mod] = map[string]float64{}
	}
	c.Params[mod][name] = value
}

// ResetParam drops one override, so the script's own value applies again.
// Reports whether there was one to drop.
func (c *Config) ResetParam(mod, name string) bool {
	if _, ok := c.Params[mod][name]; !ok {
		return false
	}
	delete(c.Params[mod], name)
	if len(c.Params[mod]) == 0 {
		delete(c.Params, mod)
	}
	return true
}

// ResetParams drops every override for one mod and returns the names dropped,
// sorted, so a front end can say what it restored.
func (c *Config) ResetParams(mod string) []string {
	if len(c.Params[mod]) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Params[mod]))
	for name := range c.Params[mod] {
		names = append(names, name)
	}
	sort.Strings(names)
	delete(c.Params, mod)
	return names
}

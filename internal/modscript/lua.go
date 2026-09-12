/*
Package modscript loads AMUMSS-format mod scripts (spec 002 R1).

An AMUMSS mod is a Lua script whose only job is to assign one global table,
NMS_MOD_DEFINITION_CONTAINER, describing which game files to edit and how. The
reference pipeline ran each script through the system `lua` binary with a dumper
that serialised that table to JSON; this package embeds a Lua interpreter
instead, so the tool has no interpreter dependency and third-party scripts run
in a sandbox rather than with the full standard library.

The behaviour reproduced here is the reference dumper's, not AMUMSS's: backslashes
are doubled before the script is compiled, numbers keep the integer/float split
the dumper's JSON established, and an empty table is an empty array. Those are
the inputs the edit engine's golden fixtures were generated from.
*/
package modscript

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ContainerName is the global every AMUMSS script assigns.
const ContainerName = "NMS_MOD_DEFINITION_CONTAINER"

// DefaultTimeout bounds one script's execution (R1.2).
const DefaultTimeout = 5 * time.Second

// maxMemoryKB caps a script's allocations. A script that builds an ADD payload
// with string.rep can ask for an unbounded amount of memory without ever
// running long enough for the timeout to fire.
const maxMemoryKB = 256 * 1024

// maxDepth bounds table nesting while converting the container. A self-
// referential table is legal Lua and would otherwise recurse forever.
const maxDepth = 64

// Load stages, as reported by LoadError.Stage. They mirror the reference dumper's
// exit codes (3, 4, 5), which is what the build report's wording was written
// against.
const (
	StageRead         = "read"
	StageLoad         = "load"
	StageExec         = "exec"
	StageNoContainer  = "no container"
	StageMalformedDef = "malformed definition"
)

// LoadError says which script failed and how far it got (R1.2).
type LoadError struct {
	Path    string
	Stage   string
	Message string
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("%s: %s: %s", filepath.Base(e.Path), e.Stage, e.Message)
}

/*
Load reads, runs and decodes one mod script (R1.4).

ctx bounds the run: a cancelled or expired context stops the interpreter between
instructions rather than at the next I/O call, because a mod script does no I/O.
A context with no deadline gets DefaultTimeout, so a caller that forgets cannot
hang the build on a script with an accidental infinite loop.
*/
func Load(ctx context.Context, path string) (*Definition, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, &LoadError{Path: path, Stage: StageRead, Message: err.Error()}
	}
	return LoadSource(ctx, path, src)
}

// LoadSource is Load with the script text already in hand, for tests and for a
// GUI preview of a file the user has not yet imported.
func LoadSource(ctx context.Context, path string, src []byte) (*Definition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	container, globals, err := run(ctx, path, src)
	if err != nil {
		return nil, err
	}

	def, err := decode(path, container)
	if err != nil {
		return nil, err
	}
	def.Globals = globals
	return def, nil
}

// run executes the script in a sandbox and returns the container table and the
// script's numeric globals.
func run(ctx context.Context, path string, src []byte) (map[string]any, map[string]Value, error) {
	/*
		R1.1: every backslash is doubled before the script is compiled.

		Scripts spell game paths the Windows way ("METADATA\REALITY\X.MBIN"),
		which is not a valid Lua string: \R is an invalid escape and Lua 5.4
		refuses to compile it. The reference dumper worked around this by doubling
		every backslash in the source, and the scripts have been written against
		that ever since -- a script that wanted a real escape would already be
		broken. Doing anything cleverer here (only doubling invalid escapes, say)
		would change what the existing library means.
	*/
	text := strings.ReplaceAll(string(src), `\`, `\\`)

	state := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer state.Close()
	state.SetContext(ctx)
	state.SetMx(maxMemoryKB / 1024)
	sandbox(state)

	fn, err := state.LoadString(text)
	if err != nil {
		return nil, nil, &LoadError{Path: path, Stage: StageLoad, Message: cleanLuaError(err)}
	}
	state.Push(fn)
	if err := state.PCall(0, lua.MultRet, nil); err != nil {
		return nil, nil, &LoadError{Path: path, Stage: StageExec, Message: cleanLuaError(err)}
	}

	raw := state.GetGlobal(ContainerName)
	table, ok := raw.(*lua.LTable)
	if !ok {
		return nil, nil, &LoadError{Path: path, Stage: StageNoContainer,
			Message: ContainerName + " was not assigned a table"}
	}
	tree, ok := convert(table, 0).(map[string]any)
	if !ok {
		return nil, nil, &LoadError{Path: path, Stage: StageNoContainer,
			Message: ContainerName + " is a list, not a table of settings"}
	}
	return tree, numericGlobals(state), nil
}

/*
sandbox opens only the libraries the scripts need (R1.2).

The 27 reference scripts use table.insert, table.concat, string.rep, string
concatenation and arithmetic; nothing else. os and io are simply never opened,
so a script cannot spawn a process or read a file, and the loaders that would
let it fetch more code (load, loadstring, dofile, loadfile, require, module) are
removed from the base library after it registers them. This matters because a
mod script is a third-party download: the user is installing data, and it should
not be able to act like a program.

print survives as a no-op rather than being removed. A script that prints a
banner is common and harmless, and deleting the function would turn it into a
load failure for no gain.
*/
func sandbox(state *lua.LState) {
	for _, lib := range []struct {
		name string
		open lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		state.Push(state.NewFunction(lib.open))
		state.Push(lua.LString(lib.name))
		state.Call(1, 0)
	}
	for _, name := range []string{
		"load", "loadstring", "dofile", "loadfile", "require", "module", "newproxy", "_printregs",
	} {
		state.SetGlobal(name, lua.LNil)
	}
	state.SetGlobal("print", state.NewFunction(func(*lua.LState) int { return 0 }))
}

// cleanLuaError strips the interpreter's Go-side decoration so the message
// reads like the reference dumper's ("LOAD ERROR: ...").
func cleanLuaError(err error) string {
	var apiErr *lua.ApiError
	if errors.As(err, &apiErr) {
		return strings.TrimSpace(apiErr.Object.String())
	}
	return err.Error()
}

/*
numericGlobals collects the tuning constants a script defines at the top level
(R1.3 Globals).

Scripts put their user-editable knobs there -- PULSE_SPEED_DEFINED = 4,
STANDING_MULT = 5 -- with a comment telling the user to change the number.
Spec 004 turns those into GUI fields, which needs to know they exist; nothing in
this phase reads them.
*/
func numericGlobals(state *lua.LState) map[string]Value {
	out := map[string]Value{}
	state.G.Global.ForEach(func(k, v lua.LValue) {
		name, ok := k.(lua.LString)
		if !ok || string(name) == ContainerName {
			return
		}
		if n, ok := v.(lua.LNumber); ok {
			out[string(name)] = numberValue(float64(n))
		}
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

/*
convert turns a Lua value into the tree the reference dumper's JSON decoded to.

The shapes are the dumper's, not Lua's: a table whose keys are all numbers is a
list of t[1]..t[#t], any other table is a string-keyed map, and an empty table
is an empty list because the dumper wrote "[]" for it. Reproducing that here
rather than at the JSON boundary keeps one decoder for both DumpJSON and the
change-table model, so the golden Lua stage and the engine cannot disagree about
what a script said.
*/
func convert(v lua.LValue, depth int) any {
	if depth > maxDepth {
		return nil
	}
	switch t := v.(type) {
	case lua.LString:
		return StringValue(string(t))
	case lua.LNumber:
		return numberValue(float64(t))
	case lua.LBool:
		return BoolValue(bool(t))
	case *lua.LTable:
		return convertTable(t, depth)
	default:
		return Nil
	}
}

func convertTable(t *lua.LTable, depth int) any {
	empty := true
	allNumeric := true
	t.ForEach(func(k, _ lua.LValue) {
		empty = false
		if _, ok := k.(lua.LNumber); !ok {
			allNumeric = false
		}
	})
	if empty {
		return []any{}
	}
	if allNumeric {
		n := t.Len()
		out := make([]any, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, convert(t.RawGetInt(i), depth+1))
		}
		return out
	}
	out := map[string]any{}
	t.ForEach(func(k, v lua.LValue) { out[luaKey(k)] = convert(v, depth+1) })
	return out
}

// luaKey is the dumper's tostring(k) for a table key.
func luaKey(k lua.LValue) string {
	if n, ok := k.(lua.LNumber); ok {
		return numberValue(float64(n)).String()
	}
	return k.String()
}

/*
numberValue applies the reference dumper's integer/float rule (R1.3).

The dumper wrote a number with "%d" when it was integral and below 1e15, and
with tostring() otherwise; Python's json.loads then made the first an int and
the second a float. Everything downstream -- str(val) in the report, str(val)
written into an MXML attribute, the "." test in fmt_num -- keys off that split,
so it is decided here, once, from the same rule.
*/
func numberValue(f float64) Value {
	if !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Floor(f) && math.Abs(f) < 1e15 {
		return IntValue(int64(f))
	}
	// tostring() is "%.14g" in Lua, and Python's json reads a token with no
	// ".", "e" or "E" back as an int. 1e+20 therefore stays a float; a
	// hypothetical 1e14 written this way would not.
	text := strconv.FormatFloat(f, 'g', 14, 64)
	if !strings.ContainsAny(text, ".eEni") {
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return IntValue(i)
		}
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return FloatValue(f)
	}
	return FloatValue(parsed)
}

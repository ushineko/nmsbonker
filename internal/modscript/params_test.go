package modscript_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
)

const headed = `-- @tweak name="Material yield" group="Mining"
-- @desc Multiplies what a rock gives up.
-- @desc Second line.
-- @param MATERIAL_MULTIPLIER label="Mined amount multiplier" min=1 max=100 step=1 default=10
MATERIAL_MULTIPLIER = 10  -- multiply mined amounts by this

NMS_MOD_DEFINITION_CONTAINER = {
["MOD_FILENAME"] = "x.pak",
["MODIFICATIONS"] = {}
}
`

const bare = `-- A library script with no headers at all.
PULSE_SPEED_DEFINED = 4
BOOST = 2.5
local HIDDEN = 9

NMS_MOD_DEFINITION_CONTAINER = {
["MOD_FILENAME"] = "y.pak",
["MODIFICATIONS"] = {}
}
`

// R1.2: a declared parameter carries its bounds, its label and the script's own
// value as the default.
func TestDeclaredParametersCarryTheirBounds(t *testing.T) {
	params := modscript.Parameters([]byte(headed))
	require.Len(t, params, 1)
	p := params[0]
	require.Equal(t, "MATERIAL_MULTIPLIER", p.Name)
	require.Equal(t, "Mined amount multiplier", p.Label)
	require.Equal(t, 1.0, p.Min)
	require.Equal(t, 100.0, p.Max)
	require.Equal(t, 1.0, p.Step)
	require.Equal(t, 10.0, p.Default)
	require.Equal(t, 10.0, p.Current)
	require.Equal(t, modscript.ParamInt, p.Kind)
	require.True(t, p.Bounded)

	h := modscript.ParseHeader([]byte(headed))
	require.Equal(t, "Material yield", h.Name)
	require.Equal(t, "Mining", h.Group)
	require.Equal(t, "Multiplies what a rock gives up. Second line.", h.Desc)
}

/*
R1.2: a script with no header falls back to its top-level assignments.

Unbounded on purpose. The tool has no idea what "PULSE_SPEED_DEFINED = 4" means,
so offering a slider from 1 to 100 would be inventing a range; the front end
shows a numeric field instead. `local` declarations are not parameters: they are
the script's own working variables and nothing outside it can see them.
*/
func TestUndeclaredParametersFallBackToTopLevelAssignments(t *testing.T) {
	params := modscript.Parameters([]byte(bare))
	require.Len(t, params, 2)
	require.Equal(t, "PULSE_SPEED_DEFINED", params[0].Name)
	require.Equal(t, "PULSE_SPEED_DEFINED", params[0].Label, "no header means no friendlier name")
	require.Equal(t, 4.0, params[0].Default)
	require.False(t, params[0].Bounded)
	require.Equal(t, modscript.ParamInt, params[0].Kind)

	require.Equal(t, "BOOST", params[1].Name)
	require.Equal(t, 2.5, params[1].Default)
	require.Equal(t, modscript.ParamFloat, params[1].Kind, "a fractional literal is a float")

	for _, p := range params {
		require.NotEqual(t, "HIDDEN", p.Name, "a local is not a parameter")
	}
}

// R1.2: only the head of the file is scanned. A `Value = 5` inside the change
// tables is a field, not a knob, and offering it would rewrite the definition.
func TestParametersStopAtTheContainer(t *testing.T) {
	src := bare + "\nAFTER_THE_FACT = 7\n"
	for _, p := range modscript.Parameters([]byte(src)) {
		require.NotEqual(t, "AFTER_THE_FACT", p.Name)
	}
}

/*
R1.3: an override rewrites the assignment line and nothing else.

The whole file matters here, not just the number: the comment beside the
assignment is the author's note about what it means, the indentation is theirs,
and the change tables underneath must come through untouched or the override has
edited the mod rather than its parameter.
*/
func TestOverrideRewritesOnlyTheAssignment(t *testing.T) {
	out, err := modscript.Override([]byte(headed), "MATERIAL_MULTIPLIER", 20, modscript.ParamInt)
	require.NoError(t, err)
	text := string(out)
	require.Contains(t, text, "MATERIAL_MULTIPLIER = 20  -- multiply mined amounts by this")
	require.NotContains(t, text, "= 10")
	require.Contains(t, text, `["MOD_FILENAME"] = "x.pak"`)
	require.Equal(t, strings.Count(headed, "\n"), strings.Count(text, "\n"),
		"the substitution is one line for one line")

	// And the input is untouched: the file on disk is never modified, and the
	// embedded copy is shared between builds.
	require.Contains(t, headed, "MATERIAL_MULTIPLIER = 10")
}

// R1.3: a name the script does not assign is an error. The script changed
// shape, and a build that ignores a value the user set is worse than one that
// stops and says which parameter went missing.
func TestOverrideOfAMissingAssignmentFails(t *testing.T) {
	_, err := modscript.Override([]byte(headed), "NO_SUCH_KNOB", 3, modscript.ParamInt)
	require.ErrorIs(t, err, modscript.ErrNoAssignment)
	require.ErrorContains(t, err, "NO_SUCH_KNOB")
}

/*
An integer parameter is written without a decimal point.

This is not cosmetic. The engine's integer/float split decides the shape of the
value written into the merged MXML, so `MULT = 3` and `MULT = 3.0` produce
different bytes, and one of them is a file MBINCompiler rejects. A slider
dragged to a whole number must not change the shape of the output.
*/
func TestAnIntegerParameterIsWrittenWithoutAPoint(t *testing.T) {
	require.Equal(t, "3", modscript.FormatValue(3, modscript.ParamInt))
	require.Equal(t, "3.0", modscript.FormatValue(3, modscript.ParamFloat))
	require.Equal(t, "0.5", modscript.FormatValue(0.5, modscript.ParamFloat))
	require.Equal(t, "2.5", modscript.FormatValue(2.5, modscript.ParamInt),
		"a fractional value keeps its fraction whatever the declared kind")
}

// R1's risk note: a parameter assigned twice would take the second value and
// ignore the override, so `mods check` warns about it.
func TestDuplicateAssignmentsAreReported(t *testing.T) {
	src := "MULT = 2\nMULT = 5\nOTHER = 1\n\nNMS_MOD_DEFINITION_CONTAINER = {}\n"
	require.Equal(t, []string{"MULT"}, modscript.DuplicateAssignments([]byte(src)))
	require.Empty(t, modscript.DuplicateAssignments([]byte(headed)))
}

// OverrideAll applies a whole set, and reports the first failure by name rather
// than leaving the caller to work out which of them was wrong.
func TestOverrideAllAppliesEveryValue(t *testing.T) {
	src := "A = 1\nB = 2\n\nNMS_MOD_DEFINITION_CONTAINER = {}\n"
	out, err := modscript.OverrideAll([]byte(src),
		map[string]float64{"A": 9, "B": 8},
		map[string]string{"A": modscript.ParamInt, "B": modscript.ParamInt})
	require.NoError(t, err)
	require.Contains(t, string(out), "A = 9")
	require.Contains(t, string(out), "B = 8")

	_, err = modscript.OverrideAll([]byte(src),
		map[string]float64{"C": 1}, map[string]string{"C": modscript.ParamInt})
	require.ErrorIs(t, err, modscript.ErrNoAssignment)
}

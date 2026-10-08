//go:build windows

package mbin_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

// The .NET host exits 0x80008096 when the runtime a build needs is missing,
// and says so only on a console nobody sees. The attempt line has to name what
// to install instead (spec 023 R3).
func TestAMissingRuntimeIsNamed(t *testing.T) {
	// cmd's exit takes a signed 32-bit code; this is 0x80008096.
	err := exec.CommandContext(t.Context(), "cmd", "/c", "exit", "-2147450730").Run()
	require.Error(t, err)
	require.Contains(t, mbin.RuntimeHint(err), ".NET 10 runtime")

	err = exec.CommandContext(t.Context(), "cmd", "/c", "exit", "3").Run()
	require.Empty(t, mbin.RuntimeHint(err), "any other failure gets no hint")
}

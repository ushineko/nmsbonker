//go:build !windows

package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
)

// Under Proton the game shows up in /proc as NMS.exe, by comm or by the
// Windows path in its cmdline (spec 007 AC6).
func TestTheProcScanFindsTheGameUnderProton(t *testing.T) {
	proc := t.TempDir()
	t.Cleanup(core.SetProcRoot(proc))
	require.False(t, core.DetectGame(), "an empty /proc has no game in it")

	require.NoError(t, os.MkdirAll(filepath.Join(proc, "self"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(proc, "4242"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "4242", "cmdline"),
		[]byte(`Z:\games\No Man's Sky\Binaries\NMS.exe`+"\x00"), 0o600))
	require.True(t, core.DetectGame(), "found by cmdline")

	require.NoError(t, os.Remove(filepath.Join(proc, "4242", "cmdline")))
	require.NoError(t, os.WriteFile(filepath.Join(proc, "4242", "comm"), []byte("NMS.exe\n"), 0o600))
	require.True(t, core.DetectGame(), "found by comm")
}

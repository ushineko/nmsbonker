//go:build windows

package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/core"
)

// The snapshot scan finds a process that is certainly running -- this test --
// by its image name in any case, and does not find one that is not (spec 020
// R4.3).
func TestTheSnapshotScanFindsARunningProcessByName(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	name := filepath.Base(exe)

	require.True(t, core.ProcessRunning(name))
	require.True(t, core.ProcessRunning(strings.ToUpper(name)), "Windows image names ignore case")
	require.False(t, core.ProcessRunning("no-such-process-"+name))
}

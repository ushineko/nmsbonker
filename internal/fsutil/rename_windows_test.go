//go:build windows

package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/fsutil"
)

// A directory with a file held open cannot be moved on Windows; once the
// holder lets go -- as a virus scan does -- the retry succeeds (spec 023 R5.2).
func TestARenameBlockedByAnOpenFileSucceedsOnceItCloses(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "mod"), filepath.Join(dir, "archived")
	require.NoError(t, os.MkdirAll(src, 0o750))
	held, err := os.Create(filepath.Join(src, "A.MBIN"))
	require.NoError(t, err)

	require.Error(t, os.Rename(src, dst), "the plain rename is refused while the file is open")

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = held.Close()
	}()
	require.NoError(t, fsutil.Rename(src, dst))
	require.FileExists(t, filepath.Join(dst, "A.MBIN"))
}

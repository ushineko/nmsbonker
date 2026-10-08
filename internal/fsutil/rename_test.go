package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/fsutil"
)

// The plain case is a rename, file or directory, replacing a file.
func TestRenameMovesFilesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(from, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(to, []byte("old"), 0o600))
	require.NoError(t, fsutil.Rename(from, to))
	got, err := os.ReadFile(to)
	require.NoError(t, err)
	require.Equal(t, "new", string(got))

	src, dst := filepath.Join(dir, "d1"), filepath.Join(dir, "d2")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "x"), 0o750))
	require.NoError(t, fsutil.Rename(src, dst))
	require.DirExists(t, filepath.Join(dst, "x"))
}

// An error that waiting cannot fix comes back at once, unchanged, so a
// caller's fallback sees the real cause.
func TestAMissingSourceIsNotRetried(t *testing.T) {
	dir := t.TempDir()
	err := fsutil.Rename(filepath.Join(dir, "nope"), filepath.Join(dir, "b"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

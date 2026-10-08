//go:build windows

package fsutil

import (
	"errors"

	"golang.org/x/sys/windows"
)

// transient reports the errors a scanner or indexer holding a file open
// produces, which clear without anyone doing anything.
func transient(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

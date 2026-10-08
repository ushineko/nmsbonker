/*
Package fsutil holds the file-system operations whose behaviour differs by
platform in ways the rest of the program should not have to know about
(spec 020 R5).
*/
package fsutil

import (
	"os"
	"time"
)

// renameBudget is how long Rename keeps retrying a transient refusal.
const renameBudget = 2 * time.Second

/*
Rename is os.Rename, retried while the platform reports the kind of refusal
that goes away on its own (spec 020 R5.2).

On Windows a file that was written a moment ago is often still open in
something else -- Defender scanning it, the search indexer -- and the rename
fails with "access denied" or a sharing violation for a few hundred
milliseconds. Retrying with a short backoff is the standard answer. Any other
error, and anything still failing after the budget, is returned as os.Rename
returned it, so a caller's fallback (copy across volumes, say) still sees the
real cause. On Linux nothing is transient and this is os.Rename.
*/
func Rename(from, to string) error {
	//nolint:wrapcheck // os.Rename's *LinkError already names both paths, and every caller adds its own context
	return retry(func() error { return os.Rename(from, to) })
}

// retry runs op until it succeeds, fails for a reason that is not transient,
// or the budget runs out, and returns its last error.
func retry(op func() error) error {
	err := op()
	if err == nil || !transient(err) {
		return err
	}
	deadline := time.Now().Add(renameBudget)
	for wait := 10 * time.Millisecond; time.Now().Before(deadline); wait = min(wait*2, 250*time.Millisecond) {
		time.Sleep(wait)
		if err = op(); err == nil || !transient(err) {
			return err
		}
	}
	return err
}

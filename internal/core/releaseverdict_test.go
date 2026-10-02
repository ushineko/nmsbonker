package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

/*
#12: "Check for updates" says whether there is one.

The listing used to mark a row "would install" and say nothing else, which
read as "no update" -- with v7.04.1-pre3 out and builds on v7.03.2-pre2.
*/
func TestTheReleaseVerdictSaysWhetherThereIsAnUpdate(t *testing.T) {
	for _, c := range []struct {
		name, pin, inUse, selected, newest string
		update                             bool
		says                               string
	}{
		{"unpinned behind", "", "v7.03.2-pre2", "v7.04.1-pre3", "v7.04.1-pre3", true,
			"Update available: v7.04.1-pre3 (builds use v7.03.2-pre2)"},
		{"unpinned current", "", "v7.04.1-pre3", "v7.04.1-pre3", "v7.04.1-pre3", false,
			"Up to date: builds use v7.04.1-pre3"},
		{"pinned behind", "v7.03.2-pre2", "v7.03.2-pre2", "v7.03.2-pre2", "v7.04.1-pre3", true,
			"Pinned to v7.03.2-pre2; v7.04.1-pre3 is newer. Unpin"},
		{"pinned newest", "v7.04.1-pre3", "v7.04.1-pre3", "v7.04.1-pre3", "v7.04.1-pre3", false,
			"Pinned to v7.04.1-pre3, which is the newest"},
		{"nothing installed", "", "", "v7.04.1-pre3", "v7.04.1-pre3", true,
			"Nothing is installed. Install the newest match fetches v7.04.1-pre3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			says, update := releaseVerdict(c.pin, c.inUse, c.selected, c.newest)
			require.Equal(t, c.update, update)
			require.Contains(t, says, c.says)
		})
	}
}

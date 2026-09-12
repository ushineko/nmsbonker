package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

/*
table prints fixed-width columns without ANSI colour (R8.2).

Widths are measured in runes rather than bytes because pak-internal paths are
ASCII but mod names are not: a mod called "Bigger Inventory ✦" would otherwise
push its whole row a few columns left, which looks like a bug in the table.
*/
type table struct {
	head []string
	rows [][]string
}

func (t *table) header(cols ...string) { t.head = cols }

func (t *table) row(cols ...string) { t.rows = append(t.rows, cols) }

func (t *table) write(w io.Writer) {
	if len(t.rows) == 0 {
		return
	}
	widths := make([]int, 0, len(t.head))
	for _, h := range t.head {
		widths = append(widths, utf8.RuneCountInString(h))
	}
	for _, row := range t.rows {
		for i, cell := range row {
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}

	line := func(cols []string) {
		var b strings.Builder
		for i, cell := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			// The last column is not padded: trailing spaces make copied output
			// awkward and buy nothing.
			if i == len(cols)-1 {
				b.WriteString(cell)
				continue
			}
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)))
		}
		say(w, "%s", strings.TrimRight(b.String(), " "))
	}

	if len(t.head) > 0 {
		line(t.head)
	}
	for _, row := range t.rows {
		line(row)
	}
}

// fact prints one "label: value" line, the CLI's default shape (R8.2).
func fact(w io.Writer, label string, value any) {
	say(w, "%-22s %v", label+":", value)
}

/*
say writes one line to a front end's output.

The write error is dropped, once, here. A command whose stdout has gone away --
a closed pipe, a full disk -- cannot report that fact anywhere the user would
see it, and checking it at forty call sites would replace a page of output
formatting with a page of error plumbing that no caller can act on. The
operations themselves, in core, return real errors; this is presentation.
*/
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

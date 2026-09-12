package mbin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

/*
HeaderSkip is how much of an MBIN the round-trip comparison ignores (spec 002 R3.4).

MBINCompiler stamps its own format id and version into the file header, so a
recompiled MBIN never equals the pristine one byte for byte even when the data
is identical. Measured on 2026-09-11 with v7.02.0-pre1 against this game
install, the only differences are at offsets 0x0A-0x0B and 0x18-0x1C; 0x60 is a
generous margin over that, and the length check below is what stops the skip
from hiding a real difference in the first 96 bytes of data (there is none:
the header runs past 0x60 in every template).
*/
const HeaderSkip = 0x60

// maxDetailLines bounds how much of a compiler failure a result carries. The
// useful part -- the libMBIN exception naming the field it choked on -- is at
// the end, so the tail is kept.
const maxDetailLines = 6

/*
RoundTrip is one file's answer to "can this compiler read and rewrite this
install's data?" (R3.4).

OK is the whole verdict; Reason says why not. FirstDiff is the byte offset of
the first difference outside the header, or -1 when the bodies match.
*/
type RoundTrip struct {
	// Name identifies the file the check ran on.
	Name string
	OK   bool
	// Reason is empty when OK.
	Reason       string
	OriginalSize int
	RebuiltSize  int
	FirstDiff    int
}

/*
CheckRoundTrip decompiles an MBIN, recompiles the result, and compares (R3.4).

This replaces spec 001's version probe as the compatibility test. The game's own
MBINs carry no libMBIN version stamp -- the field holds something else entirely
-- so no version string can answer the question. Whether the compiler can read
and rewrite a file without changing it can be measured directly, and that is the
property a build actually depends on.

A failure at any stage is a result, not an error: the caller reports "mismatch"
and carries on. Only being unable to set up the work directory is an error.
*/
func (c *Compiler) CheckRoundTrip(ctx context.Context, mbinPath, workDir string) (RoundTrip, error) {
	out := RoundTrip{Name: filepath.Base(mbinPath), FirstDiff: -1}

	original, err := os.ReadFile(mbinPath)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", mbinPath, err)
	}
	out.OriginalSize = len(original)

	mxmlDir := filepath.Join(workDir, "mxml")
	mbinDir := filepath.Join(workDir, "mbin")
	for _, dir := range []string{mxmlDir, mbinDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return out, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	mxmlPath, err := c.Decompile(ctx, mbinPath, mxmlDir)
	if err != nil {
		out.Reason = fmt.Sprintf("%s did not decompile: %s", out.Name, tail(err.Error()))
		return out, nil
	}
	rebuiltPath, err := c.Compile(ctx, mxmlPath, mbinDir)
	if err != nil {
		out.Reason = fmt.Sprintf("%s did not recompile: %s", out.Name, tail(err.Error()))
		return out, nil
	}
	rebuilt, err := os.ReadFile(rebuiltPath)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", rebuiltPath, err)
	}
	out.RebuiltSize = len(rebuilt)

	if len(rebuilt) != len(original) {
		out.Reason = fmt.Sprintf("%s round-tripped to a different size (%d bytes, was %d)",
			out.Name, len(rebuilt), len(original))
		return out, nil
	}
	if len(original) <= HeaderSkip {
		// Nothing but header. Equal length is all there is to check.
		out.OK = true
		return out, nil
	}
	if i := bytes.Compare(original[HeaderSkip:], rebuilt[HeaderSkip:]); i != 0 {
		out.FirstDiff = HeaderSkip + firstDiff(original[HeaderSkip:], rebuilt[HeaderSkip:])
		out.Reason = fmt.Sprintf("%s round-tripped to different bytes, first at offset 0x%X", out.Name, out.FirstDiff)
		return out, nil
	}
	out.OK = true
	return out, nil
}

func firstDiff(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			return i
		}
	}
	return len(a)
}

// tail keeps the last few lines of a compiler failure, collapsed onto one line
// so it fits a report bullet.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxDetailLines {
		lines = lines[len(lines)-maxDetailLines:]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, "; ")
}

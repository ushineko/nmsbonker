/*
Command nmsbonker builds No Man's Sky mods from AMUMSS-format Lua scripts.

This binary is built CGO_ENABLED=0: it reads the game's pak archives natively
and shells out to exactly one external program, MBINCompiler (spec 001 R1.2).
*/
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ushineko/nmsbonker/internal/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		// The command tree silences cobra's own reporting so this is the single
		// place a failure is printed; leaving it on prints every failure twice.
		fmt.Fprintln(os.Stderr, "nmsbonker:", err)
		var usage *cli.UsageError
		if errors.As(err, &usage) {
			// R8.2: a misuse of the command line is exit 2, distinct from a
			// command that ran and failed, so scripts can tell them apart.
			os.Exit(2)
		}
		os.Exit(1)
	}
}

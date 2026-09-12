/*
Command nmsbonker-gui is the desktop front end (spec 003).

It is a separate binary from cmd/nmsbonker on purpose. This one needs CGO,
OpenGL and a display server; the CLI needs none of those and must not, because
it is the artifact that has to build and run anywhere the game does (spec 001
R1.2). Neither binary imports the other's front end.

The flags are parsed with the standard library rather than cobra: there are
four, three of them exist so a capture script can deep-link into a section, and
pulling a command framework in for that would put a second command tree in a
project whose whole point is that there is one.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/nmsbonker/internal/buildinfo"
	"github.com/ushineko/nmsbonker/internal/gui"
)

func main() {
	section := flag.String("section", "",
		"open on this section: "+strings.Join(gui.SectionNames(), ", "))
	scheme := flag.String("scheme", "",
		"use this colour scheme for this run without saving it: "+strings.Join(gui.SchemeNames(), ", "))
	// The same flag the CLI takes, spelled the same way, because the two front
	// ends read one settings document and pointing them at different ones is a
	// thing you do deliberately -- a scratch config for a test run -- rather
	// than by accident.
	configPath := flag.String("config", "",
		"settings file (default $XDG_CONFIG_HOME/nmsbonker/config.json)")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *version {
		fmt.Printf("nmsbonker-gui %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		os.Exit(0)
	}

	gui.Run(gui.Options{
		Version:    buildinfo.Version,
		Commit:     buildinfo.Commit,
		ConfigPath: *configPath,
		Section:    *section,
		Scheme:     *scheme,
	})
}

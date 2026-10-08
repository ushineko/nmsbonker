/*
Command fakembin stands in for MBINCompiler in tests (spec 020 R8.1).

It replaces the `#!/bin/sh` scripts the tests used to write, which Windows
cannot execute. Each behaviour is a port of one of those scripts, chosen by a
`kind` file beside the binary; the other knobs are files there too, never the
environment, because the runner deliberately strips the environment (spec 001
R5.4) and an env-driven fake could not tell that apart from a bug.

Files read from the binary's directory:

	kind         runner | build | cache | roundtrip (default runner)
	mode         a per-kind behaviour (default ok)
	version      what a bare `version` prints
	body-offset  where roundtrip's "body" mode flips a byte (default 200)
	fail         cache: basenames whose conversion fails, one per line

Files written there: seen-env (runner: the environment it was given), calls
(cache: one input per line), running/ and concurrency (runner, mode slow).
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// here is the directory the fake was started from; its knobs live there.
func here() string {
	abs, err := filepath.Abs(os.Args[0])
	if err != nil {
		return "."
	}
	return filepath.Dir(abs)
}

func knob(name, def string) string {
	b, err := os.ReadFile(filepath.Join(here(), name))
	if err != nil {
		return def
	}
	if v := strings.TrimSpace(string(b)); v != "" {
		return v
	}
	return def
}

func run(args []string) int {
	kind := knob("kind", "runner")
	mode := knob("mode", "ok")

	if kind == "runner" {
		_ = os.WriteFile(filepath.Join(here(), "seen-env"), []byte(strings.Join(os.Environ(), "\n")+"\n"), 0o600)
		if len(args) > 0 && args[0] == "version" {
			if len(args) > 1 {
				fmt.Println("Compiled with MBINCompiler v7.1.0.1")
			} else {
				fmt.Println(knob("version", "MBINCompiler v7.01.0-pre1"))
			}
			return 0
		}
	}

	outdir, input := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			if i+1 < len(args) {
				outdir = args[i+1]
				i++
			}
		case "-y", "-q", "-Q", "version":
		default:
			input = args[i]
		}
	}
	if input == "" {
		fmt.Println(knob("version", "MBINCompiler v0.0.0-fake"))
		return 0
	}
	base := filepath.Base(input)
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	switch kind {
	case "build":
		return build(input, outdir, stem)
	case "cache":
		return cache(input, outdir, base, stem)
	case "roundtrip":
		return roundTrip(mode, input, outdir, base, stem)
	default:
		return runner(mode, outdir, base, stem)
	}
}

func fail(msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	return 1
}

func write(path string, data []byte) int {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fail(err.Error())
	}
	return 0
}

// runner is the behaviour internal/mbin's own tests drive.
func runner(mode, outdir, base, stem string) int {
	switch mode {
	case "slow":
		dir := filepath.Join(here(), "running")
		_ = os.MkdirAll(dir, 0o750)
		me := filepath.Join(dir, strconv.Itoa(os.Getpid()))
		_ = os.WriteFile(me, nil, 0o600)
		entries, _ := os.ReadDir(dir)
		f, err := os.OpenFile(filepath.Join(here(), "concurrency"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintln(f, len(entries))
			_ = f.Close()
		}
		time.Sleep(400 * time.Millisecond)
		_ = os.Remove(me)
	case "hang":
		time.Sleep(30 * time.Second)
	case "fail":
		return fail("[ERROR]: Invalid file type.")
	case "no-output":
		return 0
	}
	switch strings.ToUpper(filepath.Ext(base)) {
	case ".MXML", ".EXML":
		return write(filepath.Join(outdir, stem+".MBIN"), []byte("compiled"))
	default:
		return write(filepath.Join(outdir, stem+".MXML"), []byte(`<?xml version="1.0"?>`))
	}
}

// build compiles anything to "MBIN" unless the source asks it to fail.
func build(input, outdir, stem string) int {
	src, err := os.ReadFile(input)
	if err != nil {
		return fail(err.Error())
	}
	if strings.Contains(string(src), "BREAK-THE-COMPILER") {
		return fail("[ERROR]: unexpected element")
	}
	return write(filepath.Join(outdir, stem+".MBIN"), []byte("MBIN"))
}

// cache decompiles to a stamp naming the input, and records every call.
func cache(input, outdir, base, stem string) int {
	if f, err := os.OpenFile(filepath.Join(here(), "calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_, _ = fmt.Fprintln(f, input)
		_ = f.Close()
	}
	if failing, err := os.ReadFile(filepath.Join(here(), "fail")); err == nil && strings.Contains(string(failing), base) {
		return fail("[ERROR]: unknown template")
	}
	return write(filepath.Join(outdir, stem+".MXML"), []byte(`<?xml version="1.0"?><!-- `+base+` -->`))
}

// roundTrip gives back what it was given, or a deliberately damaged copy.
func roundTrip(mode, input, outdir, base, stem string) int {
	data, err := os.ReadFile(input)
	if err != nil {
		return fail(err.Error())
	}
	if strings.EqualFold(filepath.Ext(base), ".mbin") {
		if mode == "faildecompile" {
			return fail("[ERROR]: unknown template")
		}
		return write(filepath.Join(outdir, stem+".MXML"), data)
	}
	if mode == "failcompile" {
		return fail("[ERROR]: unexpected element")
	}
	switch mode {
	case "short":
		if len(data) > 200 {
			data = data[:200]
		}
	case "header":
		data = flip(data, 24)
	case "body":
		off, err := strconv.Atoi(knob("body-offset", "200"))
		if err != nil {
			off = 200
		}
		data = flip(data, off)
	}
	return write(filepath.Join(outdir, stem+".MBIN"), data)
}

func flip(data []byte, at int) []byte {
	for len(data) <= at {
		data = append(data, 0)
	}
	data[at] = 'X'
	return data
}

package mbin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrNoOutput reports a run that exited zero but produced no output file (R5.4).
var ErrNoOutput = errors.New("MBINCompiler produced no output file")

// Compiler runs one installed MBINCompiler.
type Compiler struct {
	// Bin is the absolute path of the executable.
	Bin string
	// Tag is the release tag it was installed from, or "" if unknown.
	Tag string
	// Flavor is "dotnet10" or "self-contained".
	Flavor string
}

/*
processes limits how many MBINCompiler processes exist at once (R5.5).

It is process-wide rather than per-Compiler because the resource being rationed
is the machine, not the tool: a build decompiles a hundred files, each one a
.NET process with its own runtime and a copy of libMBIN, on a desktop the user
is still looking at. The GUI (spec 003) and a CLI build share the limit for the
same reason.
*/
var (
	processesMu sync.Mutex
	processes   chan struct{}
)

// SetMaxProcesses sizes the concurrency limit. Called once, from the operation
// that knows the configured value (config.Workers); a later call resizes it,
// which is safe because acquire/release both read the channel under the mutex.
func SetMaxProcesses(n int) {
	if n < 1 {
		n = 1
	}
	processesMu.Lock()
	defer processesMu.Unlock()
	processes = make(chan struct{}, n)
}

func semaphore() chan struct{} {
	processesMu.Lock()
	defer processesMu.Unlock()
	if processes == nil {
		// A caller that never configured a limit still gets one, rather than
		// unbounded process creation.
		processes = make(chan struct{}, 1)
	}
	return processes
}

func acquire(ctx context.Context) (release func(), err error) {
	sem := semaphore()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for an MBINCompiler slot: %w", ctx.Err())
	}
}

/*
minimalEnv is the environment every MBINCompiler process gets (R5.4).

The legacy pipeline ran hgpaktool under Wine and had to strip WINEDEBUG and
CLAUDECODE by hand, and forgetting one produced either a wall of Wine chatter
mixed into the tool's output or a child process that thought it was inside an
agent session. Passing a small allow-list instead of the parent's environment
makes that class of leak impossible rather than remembered: the .NET runtime
needs PATH, HOME and its own DOTNET_* variables, and nothing else here is a
correctness input.
*/
func minimalEnv() []string {
	keep := []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR"}
	out := make([]string, 0, len(keep)+4)
	for _, name := range keep {
		if v, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+v)
		}
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DOTNET_") {
			out = append(out, kv)
		}
	}
	return out
}

// run executes the compiler and returns its combined output.
//
// Cancellation kills the process: a build the user stopped must not leave a
// .NET runtime chewing a core until it finishes a file nobody wants.
func (c *Compiler) run(ctx context.Context, args ...string) (string, error) {
	release, err := acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	cmd := exec.CommandContext(ctx, c.Bin, args...) //nolint:gosec // the binary this program installed
	cmd.Env = minimalEnv()
	setProcessGroup(cmd)
	// A backstop for a process that ignores the kill or leaves a pipe open:
	// Wait gives up on the I/O rather than blocking the build forever.
	cmd.WaitDelay = 2 * time.Second
	// The compiler writes its log next to the binary, so run it from there:
	// a relative log path must not land in the user's current directory.
	cmd.Dir = filepath.Dir(c.Bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	out := strings.TrimSpace(stdout.String() + stderr.String())
	if runErr != nil {
		return out, fmt.Errorf("%s %s: %w%s", filepath.Base(c.Bin), strings.Join(args, " "),
			runErr, detail(out, c.logTail()))
	}
	return out, nil
}

// detail appends whatever the failure said, including the compiler's own log.
func detail(output, logTail string) string {
	var b strings.Builder
	if output != "" {
		b.WriteString("\n" + output)
	}
	if logTail != "" {
		b.WriteString("\n--- " + filepath.Base(logTail) + " ---")
		b.WriteString("\n" + logTail)
	}
	return b.String()
}

// logTail returns the last lines of the compiler's log file (R5.4).
//
// The interesting part of a conversion failure -- the libMBIN stack trace
// naming the field it choked on -- goes to the log, not to stderr, so an error
// without it says only "exit status 1".
func (c *Compiler) logTail() string {
	const lines = 40
	b, err := os.ReadFile(c.Bin + ".log")
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// Version reports the compiler binary's own version (R5.4).
func (c *Compiler) Version(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// FileVersion reports the libMBIN version a .MBIN was produced with (R5.4).
//
// The `version <file>` form only accepts MBIN; handed an MXML the compiler
// prints "Invalid file type" and exits non-zero, which surfaces as an error
// rather than being papered over.
func (c *Compiler) FileVersion(ctx context.Context, path string) (string, error) {
	out, err := c.run(ctx, "version", path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Decompile converts an MBIN to MXML in outDir (R5.4).
func (c *Compiler) Decompile(ctx context.Context, mbinPath, outDir string) (string, error) {
	return c.convert(ctx, mbinPath, outDir, ".MXML", ".EXML")
}

// Compile converts an MXML back to MBIN in outDir (R5.4).
func (c *Compiler) Compile(ctx context.Context, mxmlPath, outDir string) (string, error) {
	return c.convert(ctx, mxmlPath, outDir, ".MBIN")
}

// convert runs one conversion and finds what it produced.
//
// The compiler always writes the output extension in upper case regardless of
// the input's, and older releases wrote .EXML where current ones write .MXML,
// so the candidates are matched case-insensitively against the directory rather
// than assumed. Exit code zero with no output file is a real outcome -- a
// filtered-out input, a silently skipped file -- and it has its own error so a
// build reports "produced nothing" instead of failing later on a missing file.
func (c *Compiler) convert(ctx context.Context, input, outDir string, extensions ...string) (string, error) {
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", outDir, err)
	}
	if _, err := c.run(ctx, "-y", "-q", "-d", outDir, input); err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", outDir, err)
	}
	for _, ext := range extensions {
		want := strings.ToLower(base + ext)
		for _, e := range entries {
			if !e.IsDir() && strings.ToLower(e.Name()) == want {
				return filepath.Join(outDir, e.Name()), nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s produced no %s in %s",
		ErrNoOutput, filepath.Base(input), strings.Join(extensions, " or "), outDir)
}

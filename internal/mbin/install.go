package mbin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Flavors are the two Linux asset pairs MBINCompiler publishes (R2.2, R5.3).
const (
	FlavorAuto          = "auto"
	FlavorDotnet10      = "dotnet10"
	FlavorSelfContained = "self-contained"
)

// assetNames returns the binary and library asset names for a flavor.
//
// Both are required: the framework-dependent binary loads libMBIN from beside
// itself, and a directory holding only the executable produces a runtime error
// that looks nothing like "the download was incomplete".
func assetNames(flavor string) (bin, lib string, err error) {
	switch flavor {
	case FlavorDotnet10:
		return "MBINCompiler-linux-dotnet10", "libMBIN-linux-dotnet10.so", nil
	case FlavorSelfContained:
		return "MBINCompiler-linux", "libMBIN-linux.so", nil
	default:
		return "", "", fmt.Errorf("unknown MBINCompiler flavor %q", flavor)
	}
}

// HasDotnet10 reports whether a .NET 10 runtime is installed (R5.3).
//
// `dotnet --list-runtimes` rather than `dotnet --version`: the latter reports
// the SDK, which a machine with only a runtime does not have, and this project
// needs the runtime.
func HasDotnet10(ctx context.Context) bool {
	dotnet, err := exec.LookPath("dotnet")
	if err != nil {
		return false
	}
	//nolint:gosec // the path comes from exec.LookPath, and the argument is a constant
	cmd := exec.CommandContext(ctx, dotnet, "--list-runtimes")
	cmd.Env = minimalEnv()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Microsoft.NETCore.App 10.") {
			return true
		}
	}
	return false
}

// flavorOrder resolves the configured flavor into the order to try (R5.3).
//
// "auto" leads with the framework-dependent build when a .NET 10 runtime is
// present because it is a quarter of the size, and falls back to the
// self-contained one. An explicit choice is still followed by the other: the
// self-contained asset's actual runtime requirements are undocumented, so
// "fails to execute" is treated as a normal outcome to report and route around
// rather than a reason to stop.
func flavorOrder(ctx context.Context, configured string) []string {
	switch configured {
	case FlavorDotnet10:
		return []string{FlavorDotnet10, FlavorSelfContained}
	case FlavorSelfContained:
		return []string{FlavorSelfContained, FlavorDotnet10}
	default:
		if HasDotnet10(ctx) {
			return []string{FlavorDotnet10, FlavorSelfContained}
		}
		return []string{FlavorSelfContained, FlavorDotnet10}
	}
}

// InstallResult describes what Install did, for `tools ensure` output (R7.2).
type InstallResult struct {
	Compiler *Compiler
	Tag      string
	Flavor   string
	// AlreadyPresent is true when a verified install was found and nothing was
	// downloaded.
	AlreadyPresent bool
	// Attempts records each flavor tried and how it went, so a fallback is
	// visible rather than silent.
	Attempts []string
}

// Dir is where a release is installed under the tools directory.
func Dir(toolsDir, tag string) string {
	return filepath.Join(toolsDir, "mbincompiler", tag)
}

/*
Install downloads and verifies a release (R5.3).

Verification is running the thing: a binary that downloaded cleanly but cannot
start -- wrong runtime, missing glibc symbol, a truncated asset that still has
the right size -- is indistinguishable from a good one until it is executed,
and the first execution otherwise happens in the middle of a build. A failed
verification removes the directory so a retry is a clean download rather than a
half-populated directory that looks installed.
*/
func Install(ctx context.Context, toolsDir string, release Release, configuredFlavor string, client *http.Client) (*InstallResult, error) {
	dir := Dir(toolsDir, release.Tag)
	result := &InstallResult{Tag: release.Tag}

	if c, err := verifyDir(ctx, dir, release); err == nil {
		result.Compiler = c
		result.Flavor = c.Flavor
		result.AlreadyPresent = true
		return result, nil
	}

	var lastErr error
	for _, flavor := range flavorOrder(ctx, configuredFlavor) {
		binName, libName, err := assetNames(flavor)
		if err != nil {
			return nil, err
		}
		binAsset, okBin := release.Asset(binName)
		libAsset, okLib := release.Asset(libName)
		if !okBin || !okLib {
			result.Attempts = append(result.Attempts,
				fmt.Sprintf("%s: release %s does not publish %s and %s", flavor, release.Tag, binName, libName))
			continue
		}

		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
		binPath := filepath.Join(dir, binName)
		if err := download(ctx, client, binAsset.URL, binPath, 0o750); err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, flavor+": "+err.Error())
			_ = os.RemoveAll(dir)
			continue
		}
		if err := download(ctx, client, libAsset.URL, filepath.Join(dir, libName), 0o640); err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, flavor+": "+err.Error())
			_ = os.RemoveAll(dir)
			continue
		}

		compiler := &Compiler{Bin: binPath, Tag: release.Tag, Flavor: flavor}
		out, err := compiler.Version(ctx)
		if err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, fmt.Sprintf("%s: does not run: %v", flavor, err))
			_ = os.RemoveAll(dir)
			continue
		}
		if !reportsVersion(out, release.Version) {
			lastErr = fmt.Errorf("%s reported %q, which is not %s", binName, out, release.Version.Numeric())
			result.Attempts = append(result.Attempts, flavor+": "+lastErr.Error())
			_ = os.RemoveAll(dir)
			continue
		}

		result.Attempts = append(result.Attempts, fmt.Sprintf("%s: installed and verified (%s)", flavor, out))
		result.Compiler = compiler
		result.Flavor = flavor
		return result, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("release %s publishes no Linux assets this build knows how to install", release.Tag)
	}
	return result, fmt.Errorf("install MBINCompiler %s: %w", release.Tag, lastErr)
}

/*
reportsVersion checks a `version` line against the release it should be.

R5.3 asks that the output "contains the tag's numeric version". A literal
substring test fails on the real data: tag v7.01.0-pre1 prints "MBINCompiler
v7.01.0-pre1", and "7.1.0" is not a substring of that because the tag pads the
minor component. Parsing both sides and comparing the numbers is the same check
without the padding trap.
*/
func reportsVersion(output string, want Version) bool {
	got, ok := findVersion(output)
	if !ok {
		return false
	}
	return got.Major == want.Major && got.Minor == want.Minor && got.Patch == want.Patch
}

// verifyDir reports whether a directory already holds a working install.
func verifyDir(ctx context.Context, dir string, release Release) (*Compiler, error) {
	for _, flavor := range []string{FlavorDotnet10, FlavorSelfContained} {
		binName, libName, err := assetNames(flavor)
		if err != nil {
			continue
		}
		binPath := filepath.Join(dir, binName)
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, libName)); err != nil {
			continue
		}
		c := &Compiler{Bin: binPath, Tag: filepath.Base(dir), Flavor: flavor}
		out, err := c.Version(ctx)
		if err != nil {
			continue
		}
		if release.Tag != "" && !reportsVersion(out, release.Version) {
			continue
		}
		return c, nil
	}
	return nil, fmt.Errorf("no verified MBINCompiler in %s", dir)
}

// download fetches one asset to a path.
func download(ctx context.Context, client *http.Client, url, dest string, mode os.FileMode) error {
	ctx, cancel := context.WithTimeout(ctx, 10*httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "nmsbonker")
	if client == nil {
		client = &http.Client{Timeout: 10 * httpTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	// Write to a temporary name and rename, so an interrupted download never
	// leaves a truncated binary that the next run would treat as installed.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", filepath.Dir(dest), err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("place %s: %w", dest, err)
	}
	return nil
}

// ErrNoCompiler reports that no MBINCompiler is installed.
var ErrNoCompiler = errors.New("no MBINCompiler is installed")

// Installed lists the release tags present in the tools directory, highest
// first.
func Installed(toolsDir string) []string {
	entries, err := os.ReadDir(filepath.Join(toolsDir, "mbincompiler"))
	if err != nil {
		return nil
	}
	var tags []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := ParseVersion(e.Name()); !ok {
			continue
		}
		tags = append(tags, e.Name())
	}
	sort.Slice(tags, func(i, j int) bool {
		a, _ := ParseVersion(tags[i])
		b, _ := ParseVersion(tags[j])
		return a.Compare(b) > 0
	})
	return tags
}

// Locate finds an installed compiler without touching the network (R5.6).
//
// A pin that is not installed is an error rather than a silent fall-through to
// a different version: the pin exists because the user decided which compiler
// this game version needs, and quietly using another would produce MBINs they
// did not ask for.
func Locate(toolsDir, pin string) (*Compiler, error) {
	tags := Installed(toolsDir)
	if pin != "" {
		for _, tag := range tags {
			if tag == pin {
				return locateIn(Dir(toolsDir, tag), tag)
			}
		}
		return nil, fmt.Errorf("%w: pinned release %s is not in %s", ErrNoCompiler, pin, toolsDir)
	}
	for _, tag := range tags {
		if c, err := locateIn(Dir(toolsDir, tag), tag); err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w in %s", ErrNoCompiler, toolsDir)
}

// locateIn finds the executable in one installed release directory.
func locateIn(dir, tag string) (*Compiler, error) {
	for _, flavor := range []string{FlavorDotnet10, FlavorSelfContained} {
		binName, libName, err := assetNames(flavor)
		if err != nil {
			continue
		}
		bin := filepath.Join(dir, binName)
		fi, err := os.Stat(bin)
		if err != nil || fi.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, libName)); err != nil {
			continue
		}
		return &Compiler{Bin: bin, Tag: tag, Flavor: flavor}, nil
	}
	return nil, fmt.Errorf("%w in %s", ErrNoCompiler, dir)
}

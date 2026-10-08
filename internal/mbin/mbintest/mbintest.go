/*
Package mbintest installs a fake MBINCompiler for tests (spec 023 R8.1).

The fake is the Go program in ./fakembin, built once per test process with the
toolchain running the tests, so it runs wherever the tests do -- the shell
scripts it replaces did not run on Windows. Behaviour is set with files beside
the installed binary; see fakembin's documentation for the list.
*/
package mbintest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/ushineko/nmsbonker/internal/mbin"
)

// Kinds of fake, one per behaviour the tests need.
const (
	KindRunner    = "runner"
	KindBuild     = "build"
	KindCache     = "cache"
	KindRoundTrip = "roundtrip"
)

//nolint:gochecknoglobals // built once per process
var (
	// startEnv is the environment as the process started, before any test
	// redirected HOME, the XDG directories or AppData: the go command's build
	// cache has to be the developer's, or every package rebuilds the standard
	// library into a scratch directory.
	startEnv = os.Environ()
	once     sync.Once
	built    string
	buildErr error
)

/*
Binary returns the path of the built fake.

The build lands in the system temp directory under a name derived from the
fake's source and the Go version, so concurrent test packages share one build
and a changed fake is rebuilt. It is written under a temporary name and
renamed, so two packages building at once cannot see half a file.
*/
func Binary(t testing.TB) string {
	t.Helper()
	once.Do(func() { built, buildErr = buildFake() })
	if buildErr != nil {
		t.Fatalf("build the fake MBINCompiler: %v", buildErr)
	}
	return built
}

func buildFake() (string, error) {
	_, self, _, _ := runtime.Caller(0)
	pkg := filepath.Join(filepath.Dir(self), "fakembin")
	src, err := os.ReadFile(filepath.Join(pkg, "main.go"))
	if err != nil {
		return "", fmt.Errorf("read the fake's source: %w", err)
	}
	sum := sha256.Sum256(append(src, runtime.Version()...))
	name := "fakembin-" + hex.EncodeToString(sum[:8])
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := filepath.Join(os.TempDir(), "nmsbonker-test")
	out := filepath.Join(dir, name)
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	_ = tmp.Close()
	// `go test` puts the toolchain running the tests first on PATH.
	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("find the go command: %w", err)
	}
	//nolint:noctx // the toolchain running these tests, building this repository's own fake
	cmd := exec.Command(goBin, "build", "-o", tmp.Name(), ".")
	cmd.Dir = pkg
	cmd.Env = append(append([]string{}, startEnv...), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", &buildError{err: err, out: string(b)}
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		_ = os.Remove(tmp.Name())
		if _, statErr := os.Stat(out); statErr == nil {
			return out, nil // another package won the race
		}
		return "", fmt.Errorf("install the fake: %w", err)
	}
	return out, nil
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// Bytes returns the fake's contents, for a test that serves it over HTTP as a
// release asset.
func Bytes(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile(Binary(t))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

/*
Install puts the fake into dir under this platform's asset names for flavor,
library included, and writes the knob files (name to contents) beside it. It
returns the binary's path.
*/
func Install(t testing.TB, dir, flavor, kind string, knobs map[string]string) string {
	t.Helper()
	binName, libName, err := mbin.AssetNames(flavor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, binName)
	if err := copyFile(Binary(t), bin); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"kind": kind, libName: "stub"}
	for k, v := range knobs {
		files[k] = v
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

// copyFile hard-links when it can, which is instant, and copies when it
// cannot (another volume, a file system without links).
func copyFile(from, to string) error {
	if err := os.Link(from, to); err == nil {
		return nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	if err := os.WriteFile(to, b, 0o700); err != nil {
		return fmt.Errorf("write %s: %w", to, err)
	}
	return nil
}

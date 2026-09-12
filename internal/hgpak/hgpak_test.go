package hgpak_test

import (
	"bytes"
	"crypto/md5" //nolint:gosec // asserting the game's own hash choice
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/hgpak"
	"github.com/ushineko/nmsbonker/internal/hgpak/hgpaktest"
)

// fixtureEntries returns files that span chunk boundaries in both directions:
// one comfortably inside the first chunk, one that starts in chunk 0 and ends
// in chunk 2, and one that begins mid-chunk after it. A reader that gets the
// start/end chunk arithmetic wrong passes on small files and fails on these.
func fixtureEntries() []hgpaktest.Entry {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test data, not a security use
	big := make([]byte, 3*hgpak.ChunkSize+1234)
	_, _ = rng.Read(big)
	tail := make([]byte, hgpak.ChunkSize+7)
	_, _ = rng.Read(tail)
	return []hgpaktest.Entry{
		{Name: `METADATA\REALITY\TABLES\REWARDTABLE.MBIN`, Data: []byte("first file, small")},
		{Name: "big.spanning.file", Data: big},
		{Name: "GCGAMEPLAYGLOBALS.GLOBAL.MBIN", Data: []byte("globals live at the pak root")},
		{Name: "tail.file", Data: tail},
	}
}

func writeFixture(t *testing.T, compressed bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.pak")
	require.NoError(t, hgpaktest.WriteFile(path, fixtureEntries(), compressed))
	return path
}

// The chunked layout is the whole reason this package exists: the reader has to
// stitch a file back together out of 64 KiB chunks, and an off-by-one in the
// start/end chunk arithmetic corrupts exactly the files that cross a boundary.
// This is the regression test for that, in both pak layouts (R4.1, R4.4).
func TestAPakRoundTripsEveryFileInBothLayouts(t *testing.T) {
	for _, compressed := range []bool{true, false} {
		name := "uncompressed"
		if compressed {
			name = "compressed"
		}
		t.Run(name, func(t *testing.T) {
			pak, err := hgpak.Open(writeFixture(t, compressed))
			require.NoError(t, err)
			defer func() { require.NoError(t, pak.Close()) }()

			require.Equal(t, compressed, pak.Compressed())
			require.Len(t, pak.Names(), len(fixtureEntries()),
				"the manifest pairs one name with each index entry after entry 0")

			for _, want := range fixtureEntries() {
				got, err := pak.ReadFile(want.Name)
				require.NoError(t, err, want.Name)
				require.Equal(t, want.Data, got, want.Name)
			}
		})
	}
}

// Scripts, the legacy builder and the pak itself all spell paths differently
// (backslashes, upper case, a "GLOBALS\" prefix the extractor invented). If
// lookup were literal, a mod would silently stop applying because its author
// typed a backslash. R4.2.
func TestLookupIgnoresCaseAndSlashDirection(t *testing.T) {
	pak, err := hgpak.Open(writeFixture(t, true))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	for _, spelling := range []string{
		`METADATA\REALITY\TABLES\REWARDTABLE.MBIN`,
		"metadata/reality/tables/rewardtable.mbin",
		"Metadata/Reality/Tables/RewardTable.MBIN",
		`metadata\reality\tables\rewardtable.mbin`,
	} {
		require.True(t, pak.Contains(spelling), spelling)
		got, err := pak.ReadFile(spelling)
		require.NoError(t, err, spelling)
		require.Equal(t, []byte("first file, small"), got)
	}

	require.False(t, pak.Contains("nothing/here.mbin"))
	_, err = pak.ReadFile("nothing/here.mbin")
	require.ErrorIs(t, err, hgpak.ErrNotFound)
}

// The index stores an md5 of the path as the game computes it. If HashPath and
// the pak disagree, extract_specific-style lookups by hash break and, worse, so
// does any future writer: the game would not find the file it just shipped.
// R4.2 asks for exactly this assertion.
func TestHashPathMatchesTheHashStoredInTheIndex(t *testing.T) {
	pak, err := hgpak.Open(writeFixture(t, true))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	for _, name := range pak.Names() {
		entry, ok := pak.Stat(name)
		require.True(t, ok, name)
		require.Equal(t, hgpak.HashPath(name), entry.Hash, name)
	}

	// The vector, spelled out: md5 of the lower-cased forward-slash path.
	want := md5.Sum([]byte("metadata/reality/tables/rewardtable.mbin")) //nolint:gosec // filename hash
	require.Equal(t, want, hgpak.HashPath(`METADATA\REALITY\TABLES\REWARDTABLE.MBIN`))
}

// A build reads a hundred small files out of one table pak. Without the chunk
// cache each read decompresses the same chunk again; without a bound on the
// cache, opening 97 paks would hold the whole install in memory. R4.3.
func TestReadingOneSmallFileDoesNotDecompressTheWholePak(t *testing.T) {
	pak, err := hgpak.Open(writeFixture(t, true))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	before := hgpak.CachedChunks(pak)
	_, err = pak.ReadFile("GCGAMEPLAYGLOBALS.GLOBAL.MBIN")
	require.NoError(t, err)
	after := hgpak.CachedChunks(pak)

	require.LessOrEqual(t, after-before, 2,
		"a 28-byte file touches at most the chunk it is in and the one it may straddle")
	require.Less(t, after, hgpak.TotalChunks(pak), "the whole pak was decompressed")
}

// The streaming reader exists so a caller does not have to hold a
// several-hundred-megabyte mesh entry in memory. It has its own chunk-boundary
// arithmetic, so it gets its own test rather than trusting ReadFile's. R4.2.
func TestStreamingAFileAgreesWithReadingItWhole(t *testing.T) {
	pak, err := hgpak.Open(writeFixture(t, true))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	whole, err := pak.ReadFile("big.spanning.file")
	require.NoError(t, err)

	rc, err := pak.Open("big.spanning.file")
	require.NoError(t, err)
	defer func() { require.NoError(t, rc.Close()) }()

	// A deliberately awkward buffer size: not a divisor of the chunk size, so
	// every read but the first starts mid-chunk.
	streamed := &bytes.Buffer{}
	_, err = io.CopyBuffer(streamed, rc, make([]byte, 7919))
	require.NoError(t, err)
	require.Equal(t, whole, streamed.Bytes())
}

// The GUI (spec 003) reads from several goroutines while a build runs. A shared
// file handle plus a shared chunk cache is exactly the shape that corrupts
// under concurrency, so the invariant is asserted under -race. R4.6.
func TestConcurrentReadsReturnTheSameBytes(t *testing.T) {
	pak, err := hgpak.Open(writeFixture(t, true))
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()

	want := map[string][]byte{}
	for _, e := range fixtureEntries() {
		b, err := pak.ReadFile(e.Name)
		require.NoError(t, err)
		want[e.Name] = b
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 16 {
		for _, e := range fixtureEntries() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := pak.ReadFile(e.Name)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got, want[e.Name]) {
					errs <- io.ErrUnexpectedEOF
				}
			}()
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// PCBANKS holds files that are not paks (BankSignatures.bin, filenames.json)
// and a game update could change the format. Reporting "not an archive"
// separately from "version 3" is what lets the index skip the first and the
// user be told about the second. R4.1.
func TestABadMagicAndABadVersionAreDistinctErrors(t *testing.T) {
	dir := t.TempDir()

	notAPak := filepath.Join(dir, "BankSignatures.bin")
	require.NoError(t, os.WriteFile(notAPak, []byte("this is not a pak at all, really"), 0o600))
	_, err := hgpak.Open(notAPak)
	require.ErrorIs(t, err, hgpak.ErrNotHGPAK)

	tooShort := filepath.Join(dir, "short.pak")
	require.NoError(t, os.WriteFile(tooShort, []byte("HGPAK"), 0o600))
	_, err = hgpak.Open(tooShort)
	require.ErrorIs(t, err, hgpak.ErrNotHGPAK)

	good, err := hgpaktest.Build(fixtureEntries(), true)
	require.NoError(t, err)
	future := append([]byte(nil), good...)
	future[0x08] = 3
	futurePath := filepath.Join(dir, "future.pak")
	require.NoError(t, os.WriteFile(futurePath, future, 0o600))
	_, err = hgpak.Open(futurePath)
	require.ErrorIs(t, err, hgpak.ErrUnsupportedVersion)
}

// A pak with no files at all still has a manifest entry, and the reader used to
// index past the end of the entry table building the (empty) name list.
func TestAnEmptyPakOpensWithNoNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.pak")
	require.NoError(t, hgpaktest.WriteFile(path, nil, true))

	pak, err := hgpak.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, pak.Close()) }()
	require.Empty(t, pak.Names())
}

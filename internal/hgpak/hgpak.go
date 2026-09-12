/*
Package hgpak reads No Man's Sky HGPAK version 2 archives natively (spec 001 R4).

It replaces running AMUMSS's hgpaktool.exe under Wine. The read path is a port
of github.com/monkeyman192/HGPAKtool (hgpaktool/api.py, MIT), which is the
reference for the format; the layout notes below record what that code does so
the Go here can be checked against it without a Python interpreter.

Layout, all little-endian:

	0x00  "HGPAK" + 3 pad bytes
	0x08  version        u64   (2)
	0x10  file_count     u64
	0x18  chunk_count    u64
	0x20  is_compressed  u8 + 7 pad
	0x28  data_offset    u64
	0x30  file_count x { md5 [16]byte, start_offset u64, decompressed_size u64 }
	      if compressed: chunk_count x u64 compressed chunk sizes

Each chunk begins at data_offset plus the sum of the previous chunk sizes, each
rounded up to a 0x10 boundary, and decompresses to exactly 0x10000 bytes. File
entry 0 is the filename manifest: CRLF-separated relative paths, one per
remaining entry, so files[i] pairs with fileInfo[i+1]. Entry offsets are
absolute in an uncompressed pak and relative to the start of the decompressed
stream (offset - data_offset) in a compressed one.

Out of scope, deliberately: writing paks, and the mac (lz4, 0x20000 chunks) and
Switch (Oodle) variants. This tool reads the PC install it is running on.
*/
package hgpak

import (
	"crypto/md5" //nolint:gosec // the game's filename hash is md5; this is not a security use
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	// magic is the first five bytes of every HGPAK.
	magic = "HGPAK"
	// formatVersion is the only version this reads (R4.1).
	formatVersion = 2
	// ChunkSize is the decompressed size of every chunk in a PC pak.
	ChunkSize = 0x10000
	headerLen = 0x30
	entryLen  = 0x20
	// defaultCacheChunks is the LRU size (R4.3): 64 x 64 KiB = 4 MiB per open
	// pak. Large enough that reading a table's worth of small files touches
	// each chunk once, small enough that the index build can hold 97 of these
	// open without the cache being the reason it needs a gigabyte.
	defaultCacheChunks = 64
)

// Typed errors so a caller can tell "this is not a pak" from "this is a pak I
// cannot read" (R4.1) -- the first is a stray file in PCBANKS, the second is a
// game update that needs code.
var (
	ErrNotHGPAK           = errors.New("not an HGPAK archive")
	ErrUnsupportedVersion = errors.New("unsupported HGPAK version")
	ErrNotFound           = errors.New("file not found in pak")
	// ErrCorruptChunk reports a chunk that neither decompressed nor matched the
	// stored-raw fallback.
	ErrCorruptChunk = errors.New("chunk did not decompress")
)

// Entry is one file in a pak (R4.2).
type Entry struct {
	// Name is the manifest's path, normalised (forward slashes, lower case).
	Name string
	// Offset is where the file starts: absolute in the pak for an uncompressed
	// archive, relative to the decompressed stream for a compressed one.
	Offset uint64
	Size   uint64
	Hash   [16]byte
}

// Normalise puts a path into the form the manifest and the hash use: forward
// slashes, lower case.
//
// Scripts and the legacy tooling write these paths every possible way --
// "METADATA\REALITY\X.MBIN", "metadata/reality/x.mbin", sometimes with a
// "GLOBALS\" prefix invented by hgpaktool's extractor -- and the pak itself
// stores one canonical spelling. Every lookup goes through here so that the
// spelling a caller happens to have is never the reason a file is "missing".
func Normalise(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.ToLower(p)
}

// HashPath reproduces the game's filename hash: md5 of the normalised path
// (R4.2). A test asserts this equals the hash stored in the index, which is the
// only proof that Normalise agrees with the game.
func HashPath(p string) [16]byte {
	return md5.Sum([]byte(Normalise(p))) //nolint:gosec // filename hash, not a security primitive
}

// header is the fixed part of the archive.
type header struct {
	version      uint64
	fileCount    uint64
	chunkCount   uint64
	isCompressed bool
	dataOffset   uint64
}

// File is an open pak.
//
// Safe for concurrent use (R4.6): the mutex covers the file handle, the chunk
// cache and the zstd decoder together, because a chunk read touches all three.
type File struct {
	path string
	f    *os.File
	hdr  header

	names  []string
	byName map[string]Entry

	chunkSizes   []uint64
	chunkOffsets []int64

	mu    sync.Mutex
	cache *chunkCache
}

// sharedDecoder is one zstd decoder for the whole process.
//
// Per-File decoders were the first shape and cost measurably more than they
// saved: building the pak index opens 97 archives, and each zstd.NewReader
// starts its own goroutines. DecodeAll is documented as safe for concurrent
// use, so one decoder serves every open pak.
var sharedDecoder = sync.OnceValues(func() (*zstd.Decoder, error) {
	d, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("start zstd decoder: %w", err)
	}
	return d, nil
})

// Open parses a pak's header and index (R4.1).
//
// Only the manifest is decompressed here; file data is read on demand. Opening
// all 97 paks to build an index must not mean decompressing 30 GB.
func Open(name string) (*File, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	pf, err := newFile(name, f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return pf, nil
}

func newFile(name string, f *os.File) (*File, error) {
	raw := make([]byte, headerLen)
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, fmt.Errorf("%s: read header: %w", name, errNotPak(err))
	}
	if string(raw[:5]) != magic {
		return nil, fmt.Errorf("%s: %w", name, ErrNotHGPAK)
	}
	hdr := header{
		version:      binary.LittleEndian.Uint64(raw[0x08:]),
		fileCount:    binary.LittleEndian.Uint64(raw[0x10:]),
		chunkCount:   binary.LittleEndian.Uint64(raw[0x18:]),
		isCompressed: raw[0x20] != 0,
		dataOffset:   binary.LittleEndian.Uint64(raw[0x28:]),
	}
	if hdr.version != formatVersion {
		return nil, fmt.Errorf("%s: %w: %d", name, ErrUnsupportedVersion, hdr.version)
	}
	if hdr.fileCount == 0 {
		return nil, fmt.Errorf("%s: %w: no file index", name, ErrNotHGPAK)
	}
	// A file count large enough to overflow the index read is a corrupt or
	// hostile header, not a pak; refuse before allocating from it.
	if hdr.fileCount > 1<<24 {
		return nil, fmt.Errorf("%s: %w: implausible file count %d", name, ErrNotHGPAK, hdr.fileCount)
	}

	index := make([]byte, hdr.fileCount*entryLen)
	if _, err := io.ReadFull(f, index); err != nil {
		return nil, fmt.Errorf("%s: read file index: %w", name, err)
	}

	pf := &File{path: name, f: f, hdr: hdr, cache: newChunkCache(defaultCacheChunks)}

	if hdr.isCompressed {
		if hdr.chunkCount > 1<<26 {
			return nil, fmt.Errorf("%s: %w: implausible chunk count %d", name, ErrNotHGPAK, hdr.chunkCount)
		}
		sizes := make([]byte, hdr.chunkCount*8)
		if _, err := io.ReadFull(f, sizes); err != nil {
			return nil, fmt.Errorf("%s: read chunk table: %w", name, err)
		}
		pf.chunkSizes = make([]uint64, hdr.chunkCount)
		pf.chunkOffsets = make([]int64, hdr.chunkCount)
		pos := int64(hdr.dataOffset) //nolint:gosec // a file offset read from the header
		for i := range pf.chunkSizes {
			size := binary.LittleEndian.Uint64(sizes[i*8:])
			pf.chunkSizes[i] = size
			pf.chunkOffsets[i] = pos
			pos += int64(roundUp16(size)) //nolint:gosec // bounded by the pak's own size
		}
	}

	if err := pf.readManifest(index); err != nil {
		return nil, err
	}
	return pf, nil
}

// errNotPak turns a short read of the header into ErrNotHGPAK. A 12-byte file
// in PCBANKS is not a truncated archive worth a stack trace, it is not an
// archive.
func errNotPak(err error) error {
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return ErrNotHGPAK
	}
	return err
}

// readManifest decompresses entry 0 and pairs its lines with the index.
func (p *File) readManifest(index []byte) error {
	entryAt := func(i uint64) (offset, size uint64, hash [16]byte) {
		b := index[i*entryLen:]
		copy(hash[:], b[:16])
		return binary.LittleEndian.Uint64(b[16:]), binary.LittleEndian.Uint64(b[24:]), hash
	}

	manifestOffset, manifestSize, _ := entryAt(0)
	manifest, err := p.readAbsolute(p.logicalOffset(manifestOffset), manifestSize)
	if err != nil {
		return fmt.Errorf("%s: read filename manifest: %w", p.path, err)
	}

	// The manifest is CRLF-separated and ends with one, so trim before
	// splitting: a trailing separator would otherwise produce an empty name
	// that pairs with a real index entry and shifts every later pairing.
	text := strings.TrimRight(string(manifest), "\r\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\r\n")
	}

	p.names = make([]string, 0, len(lines))
	p.byName = make(map[string]Entry, len(lines))
	for i, line := range lines {
		name := Normalise(strings.TrimSpace(line))
		if name == "" {
			continue
		}
		idx := uint64(i) + 1
		if idx >= p.hdr.fileCount {
			// More manifest lines than index entries: the pairing is broken and
			// every name past here would read another file's bytes.
			return fmt.Errorf("%s: %w: manifest lists %d files but the index holds %d",
				p.path, ErrNotHGPAK, len(lines), p.hdr.fileCount-1)
		}
		offset, size, hash := entryAt(idx)
		offset = p.logicalOffset(offset)
		p.names = append(p.names, name)
		p.byName[name] = Entry{Name: name, Offset: offset, Size: size, Hash: hash}
	}
	return nil
}

// logicalOffset converts an index entry's stored offset into an offset in the
// archive's logical address space.
//
// A compressed pak stores offsets into the whole file, so the data section's
// own start has to come off; an uncompressed pak's offsets already address the
// file directly. The clamp guards a header whose data_offset is larger than the
// entry offsets, which would otherwise wrap to an enormous unsigned value and
// read from beyond the end of the archive.
func (p *File) logicalOffset(raw uint64) uint64 {
	if !p.hdr.isCompressed {
		return raw
	}
	if raw < p.hdr.dataOffset {
		return 0
	}
	return raw - p.hdr.dataOffset
}

// Path is the archive this was opened from.
func (p *File) Path() string { return p.path }

// Compressed reports whether the archive stores zstd chunks (R4.4).
func (p *File) Compressed() bool { return p.hdr.isCompressed }

// Names lists the archive's files in manifest order (R4.2).
func (p *File) Names() []string { return p.names }

// Contains reports whether the archive holds a path, matched case-insensitively
// with backslashes normalised (R4.2).
func (p *File) Contains(name string) bool {
	_, ok := p.byName[Normalise(name)]
	return ok
}

// Stat returns the index entry for a path (R4.2).
func (p *File) Stat(name string) (Entry, bool) {
	e, ok := p.byName[Normalise(name)]
	return e, ok
}

// Close releases the file handle.
func (p *File) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		return nil
	}
	err := p.f.Close()
	p.f = nil
	p.cache.reset()
	if err != nil {
		return fmt.Errorf("close %s: %w", p.path, err)
	}
	return nil
}

// ReadFile returns a file's whole contents (R4.2).
func (p *File) ReadFile(name string) ([]byte, error) {
	e, ok := p.Stat(name)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %s", p.path, ErrNotFound, name)
	}
	b, err := p.readAbsolute(e.Offset, e.Size)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", p.path, name, err)
	}
	return b, nil
}

// Open streams a file's contents (R4.2).
//
// Streaming matters for the large archives: a mesh pak entry can be hundreds of
// megabytes, and ReadFile on one of those is a copy the caller may not need.
func (p *File) Open(name string) (io.ReadCloser, error) {
	e, ok := p.Stat(name)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %s", p.path, ErrNotFound, name)
	}
	return &entryReader{pak: p, entry: e}, nil
}

// readAbsolute reads size bytes starting at an offset in the archive's logical
// (decompressed) address space.
func (p *File) readAbsolute(offset, size uint64) ([]byte, error) {
	if size == 0 {
		return []byte{}, nil
	}
	out := make([]byte, size)
	n, err := p.readAt(out, offset)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

// readAt fills dst from the archive's logical address space.
func (p *File) readAt(dst []byte, offset uint64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		return 0, fmt.Errorf("%s: archive is closed", p.path)
	}
	if !p.hdr.isCompressed {
		// R4.4: offsets in an uncompressed pak address the file directly.
		n, err := p.f.ReadAt(dst, int64(offset)) //nolint:gosec // an offset from the pak's own index
		if err != nil && !errors.Is(err, io.EOF) {
			return n, fmt.Errorf("read at %#x: %w", offset, err)
		}
		return n, nil
	}

	written := 0
	for written < len(dst) {
		pos := offset + uint64(written) //nolint:gosec // written is a non-negative byte count
		chunkIndex := pos / ChunkSize
		if chunkIndex >= uint64(len(p.chunkSizes)) {
			break
		}
		idx := int(chunkIndex) //nolint:gosec // bounded by the check above
		within := int(pos % ChunkSize)
		chunk, err := p.chunk(idx)
		if err != nil {
			return written, err
		}
		if within >= len(chunk) {
			break
		}
		written += copy(dst[written:], chunk[within:])
	}
	return written, nil
}

// chunk returns a decompressed chunk, from the LRU when possible (R4.3).
// The caller holds p.mu.
func (p *File) chunk(idx int) ([]byte, error) {
	if idx < 0 || idx >= len(p.chunkSizes) {
		return nil, fmt.Errorf("%s: chunk %d is outside the archive's %d chunks", p.path, idx, len(p.chunkSizes))
	}
	if b, ok := p.cache.get(idx); ok {
		return b, nil
	}
	raw := make([]byte, p.chunkSizes[idx])
	if _, err := p.f.ReadAt(raw, p.chunkOffsets[idx]); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: read chunk %d: %w", p.path, idx, err)
	}
	dec, err := sharedDecoder()
	if err != nil {
		return nil, err
	}
	out, err := dec.DecodeAll(raw, make([]byte, 0, ChunkSize))
	if err != nil {
		// A chunk the packer could not compress is stored raw, and is
		// recognised only by being exactly one decompressed chunk long. This
		// is the reference implementation's rule; guessing more liberally
		// would turn a genuinely corrupt chunk into silent garbage.
		if len(raw) == ChunkSize {
			out = raw
		} else {
			return nil, fmt.Errorf("%s: chunk %d: %w", p.path, idx, errors.Join(ErrCorruptChunk, err))
		}
	}
	p.cache.put(idx, out)
	return out, nil
}

// entryReader streams one file across chunk boundaries.
type entryReader struct {
	pak   *File
	entry Entry
	pos   uint64
}

func (r *entryReader) Read(dst []byte) (int, error) {
	if r.pos >= r.entry.Size {
		return 0, io.EOF
	}
	if remaining := r.entry.Size - r.pos; uint64(len(dst)) > remaining {
		dst = dst[:remaining]
	}
	n, err := r.pak.readAt(dst, r.entry.Offset+r.pos)
	r.pos += uint64(n) //nolint:gosec // io.Reader contracts a non-negative count
	if err != nil {
		return n, err
	}
	if n == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	return n, nil
}

func (r *entryReader) Close() error { return nil }

// Base is the last path element of a normalised internal path, used by the
// index's basename fallback.
func Base(p string) string { return path.Base(Normalise(p)) }

// roundUp16 rounds a chunk's stored size up to the 0x10 boundary the next chunk
// starts on.
func roundUp16(x uint64) uint64 { return (x + 0xF) &^ 0xF }

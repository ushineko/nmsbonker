/*
Package hgpaktest builds synthetic HGPAK v2 archives for tests (spec 001 R9.1).

It exists because the repository may not contain game data (project rule: no
assets from the game's paks, not even as fixtures), so the reader's unit tests
need archives they can construct from nothing. The writer is deliberately the
mirror image of the reader's understanding of the format rather than a port of
HGPAKtool's packer: if the two agreed only because they shared code, a
round-trip test would prove nothing about whether either matches the game.
Byte-for-byte agreement with a real pak is proven separately, by the
integration test that reads the install (R9.2).
*/
package hgpaktest

import (
	"bytes"
	"crypto/md5" //nolint:gosec // the game's filename hash is md5
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// chunkSize is the decompressed chunk size of a PC pak.
const chunkSize = 0x10000

// Entry is one file to pack.
type Entry struct {
	// Name is the internal path, written into the manifest as given.
	Name string
	Data []byte
}

// Build returns the bytes of a v2 pak holding the entries.
//
// compressed selects between the two layouts the reader has to handle (R4.4):
// zstd chunks with offsets relative to the decompressed stream, or raw bytes
// with absolute offsets.
func Build(entries []Entry, compressed bool) ([]byte, error) {
	// Entry 0 is the filename manifest: CRLF-separated, with a trailing CRLF,
	// which is how the game's own paks are written.
	var manifest strings.Builder
	for _, e := range entries {
		manifest.WriteString(e.Name)
		manifest.WriteString("\r\n")
	}

	payloads := make([][]byte, 0, len(entries)+1)
	payloads = append(payloads, []byte(manifest.String()))
	names := []string{""}
	for _, e := range entries {
		payloads = append(payloads, e.Data)
		names = append(names, e.Name)
	}

	// Lay the payloads out back to back in the decompressed address space.
	// Nothing pads or aligns them: that is what the reader assumes, and a file
	// that straddles a chunk boundary is the case worth testing.
	offsets := make([]uint64, len(payloads))
	var total uint64
	for i, p := range payloads {
		offsets[i] = total
		total += uint64(len(p))
	}
	stream := make([]byte, 0, total)
	for _, p := range payloads {
		stream = append(stream, p...)
	}

	fileCount := uint64(len(payloads))
	var chunkCount uint64
	if compressed {
		chunkCount = (total + chunkSize - 1) / chunkSize
	}
	dataOffset := 0x30 + fileCount*0x20
	if compressed {
		dataOffset += chunkCount * 8
	}

	var buf bytes.Buffer
	buf.WriteString("HGPAK")
	buf.Write(make([]byte, 3))
	write64 := func(v uint64) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	write64(2)
	write64(fileCount)
	write64(chunkCount)
	if compressed {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	buf.Write(make([]byte, 7))
	write64(dataOffset)

	for i, p := range payloads {
		hash := md5.Sum([]byte(normalise(names[i]))) //nolint:gosec // filename hash
		if i == 0 {
			// The manifest entry's hash is not a path hash in the real format
			// either; zero is as good as anything and no reader looks at it.
			hash = [16]byte{}
		}
		buf.Write(hash[:])
		// Both layouts store dataOffset + the offset in the payload stream: an
		// uncompressed pak's offsets are then absolute file offsets, and a
		// compressed pak's are what the reader subtracts dataOffset from.
		write64(dataOffset + offsets[i])
		write64(uint64(len(p)))
	}

	if !compressed {
		buf.Write(stream)
		return buf.Bytes(), nil
	}

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, fmt.Errorf("start zstd encoder: %w", err)
	}
	defer func() { _ = enc.Close() }()

	chunks := make([][]byte, 0, chunkCount)
	sizes := make([]uint64, 0, chunkCount)
	for start := uint64(0); start < total; start += chunkSize {
		end := min(start+chunkSize, total)
		// Every chunk decompresses to exactly chunkSize bytes; the last one is
		// zero-padded up to it, exactly as the packer does.
		plain := make([]byte, chunkSize)
		copy(plain, stream[start:end])
		packed := enc.EncodeAll(plain, nil)
		chunks = append(chunks, packed)
		sizes = append(sizes, uint64(len(packed)))
	}
	for _, s := range sizes {
		write64(s)
	}
	for _, c := range chunks {
		buf.Write(c)
		// Chunks start on 0x10 boundaries.
		if pad := (0x10 - (len(c) & 0xF)) & 0xF; pad > 0 {
			buf.Write(make([]byte, pad))
		}
	}
	return buf.Bytes(), nil
}

// WriteFile builds a pak and writes it to disk.
func WriteFile(path string, entries []Entry, compressed bool) error {
	b, err := Build(entries, compressed)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil { //nolint:gosec // a test fixture, not a secret
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func normalise(p string) string { return strings.ToLower(strings.ReplaceAll(p, "\\", "/")) }

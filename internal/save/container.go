/*
Package save reads and writes No Man's Sky save files natively (spec 007).

Three layers, each its own file, none of which knows where the game keeps its
saves: container.go is the chunked LZ4 wrapper around the payload; meta.go is
the encrypted manifest that sits beside every save; document.go is a JSON tree
that gives back exactly the bytes it was given for everything it did not change.
mapping.go names the obfuscated keys and edits.go is the handful of typed edits
the editor offers. internal/core owns the paths, the backup and the write.

Nothing here writes a file. That is deliberate: the one code path that writes
into the game's save directory lives in core, behind the backup and the
game-not-running check, and a package that cannot write cannot skip either.
*/
package save

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/pierrec/lz4/v4"
)

// chunkMagic opens every chunk of a compressed save (R1.1).
const chunkMagic = 0xFEEDA1E5

// ChunkMax is the decompressed size of every chunk but the last (R1.2).
const ChunkMax = 0x80000

// chunkHeader is the four little-endian uint32s in front of each chunk.
const chunkHeader = 16

// ErrEmpty reports a save file with nothing in it.
var ErrEmpty = errors.New("save file is empty")

/*
Decode unwraps a save file into its JSON payload (R1.1).

A payload that begins with `{` is already plain (the account data file, and
saves written before Frontiers 3.60) and comes back as is. Either way the
trailing NUL the game writes is stripped, so the result is JSON and nothing
else. Every chunk is checked against its own header: a header that promises a
decompressed size the block does not produce is how a truncated download or a
half-written file shows itself, and it is reported with the chunk index rather
than as a JSON parse failure ten steps later.
*/
func Decode(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, ErrEmpty
	}
	if raw[0] == '{' {
		return trimNUL(raw), nil
	}
	var out []byte
	for off, chunk := 0, 0; off < len(raw); chunk++ {
		if len(raw)-off < chunkHeader {
			return nil, fmt.Errorf("chunk %d: %d byte(s) left, header needs %d", chunk, len(raw)-off, chunkHeader)
		}
		magic := binary.LittleEndian.Uint32(raw[off:])
		csize := int(binary.LittleEndian.Uint32(raw[off+4:]))
		dsize := int(binary.LittleEndian.Uint32(raw[off+8:]))
		if magic != chunkMagic {
			return nil, fmt.Errorf("chunk %d: magic 0x%08X is not 0x%08X", chunk, magic, chunkMagic)
		}
		start := off + chunkHeader
		if csize <= 0 || start+csize > len(raw) {
			return nil, fmt.Errorf("chunk %d: compressed size %d runs past the end of the file", chunk, csize)
		}
		if dsize <= 0 || dsize > ChunkMax {
			return nil, fmt.Errorf("chunk %d: decompressed size %d is outside 1..%d", chunk, dsize, ChunkMax)
		}
		buf := make([]byte, dsize)
		n, err := lz4.UncompressBlock(raw[start:start+csize], buf)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", chunk, err)
		}
		if n != dsize {
			return nil, fmt.Errorf("chunk %d: header says %d bytes, block held %d", chunk, dsize, n)
		}
		out = append(out, buf...)
		off = start + csize
	}
	return trimNUL(out), nil
}

/*
Encode wraps a JSON payload the way the game does (R1.2).

The NUL goes on first, then the whole thing is cut into ChunkMax pieces and each
is compressed on its own. The game's own compressor and this one need not agree
byte for byte -- and do not -- but they agree on what the chunks decompress to,
which is the only thing the reader checks.
*/
func Encode(payload []byte) ([]byte, error) {
	data := make([]byte, 0, len(payload)+1)
	data = append(data, payload...)
	data = append(data, 0)

	var c lz4.Compressor
	out := make([]byte, 0, len(data)/4+chunkHeader)
	for start := 0; start < len(data); start += ChunkMax {
		end := min(start+ChunkMax, len(data))
		piece := data[start:end]
		buf := make([]byte, lz4.CompressBlockBound(len(piece)))
		n, err := c.CompressBlock(piece, buf)
		if err != nil {
			return nil, fmt.Errorf("compress chunk at %d: %w", start, err)
		}
		if n == 0 {
			// The block API reports "incompressible" as zero bytes written. A
			// tiny last chunk can be exactly that, and the LZ4 block format
			// still has a spelling for it: one sequence of literals and no
			// match, which every decoder accepts.
			buf = literalBlock(piece)
			n = len(buf)
		}
		var header [chunkHeader]byte
		binary.LittleEndian.PutUint32(header[0:], chunkMagic)
		binary.LittleEndian.PutUint32(header[4:], uint32(n))          //nolint:gosec // n <= CompressBlockBound(ChunkMax)
		binary.LittleEndian.PutUint32(header[8:], uint32(len(piece))) //nolint:gosec // <= ChunkMax
		out = append(out, header[:]...)
		out = append(out, buf[:n]...)
	}
	return out, nil
}

// literalBlock spells a piece as a single LZ4 literal sequence.
func literalBlock(piece []byte) []byte {
	out := make([]byte, 0, len(piece)+len(piece)/255+2)
	if len(piece) < 15 {
		out = append(out, byte(len(piece)<<4)) //nolint:gosec // len(piece) < 15
	} else {
		out = append(out, 0xF0)
		for rest := len(piece) - 15; ; rest -= 255 {
			if rest < 255 {
				out = append(out, byte(rest)) //nolint:gosec // rest < 255
				break
			}
			out = append(out, 255)
		}
	}
	return append(out, piece...)
}

// trimNUL drops the terminator and anything after it. The game writes exactly
// one NUL; the account data file has been seen with padding behind it.
func trimNUL(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

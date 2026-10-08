/*
Command winicon renders packaging/nmsbonker.svg into a Windows .ico (spec 023
R9.2).

	go run ./tools/winicon packaging/nmsbonker.svg packaging/windows/nmsbonker.ico

Each size is rasterised from the SVG rather than scaled from one bitmap, so the
16 and 24 pixel entries -- the taskbar and the title bar -- are as sharp as the
artwork allows. Entries are stored as PNG, which every Windows since Vista
reads. `make winres` runs this and then windres to produce the .syso the GUI
build links.
*/
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"regexp"
	"strconv"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// sizes are the entries Windows looks for, small to large.
var sizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256} //nolint:gochecknoglobals // a fixed table

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: winicon <in.svg> <out.ico>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "winicon:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	src, err := os.ReadFile(in) //nolint:gosec // the SVG named on the command line
	if err != nil {
		return fmt.Errorf("read %s: %w", in, err)
	}
	images := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		b, err := render(src, size)
		if err != nil {
			return fmt.Errorf("render %dpx: %w", size, err)
		}
		images = append(images, b)
	}
	if err := os.WriteFile(out, ico(images), 0o600); err != nil { //nolint:gosec // the .ico named on the command line
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}

// strokeWidth matches a stroke-width attribute's value.
var strokeWidth = regexp.MustCompile(`stroke-width="([0-9.]+)"`) //nolint:gochecknoglobals // compiled once

/*
render rasterises the SVG at size x size and returns it as PNG.

SetTarget scales the geometry to the target but leaves stroke widths in output
pixels, which draws a 7-unit handle 7 pixels wide at every size: far too thin
at 256, far too thick at 16. The widths are scaled in the source to match.
*/
func render(src []byte, size int) ([]byte, error) {
	probe, err := oksvg.ReadIconStream(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse the SVG: %w", err)
	}
	scale := float64(size) / probe.ViewBox.W
	scaled := strokeWidth.ReplaceAllFunc(src, func(m []byte) []byte {
		w, err := strconv.ParseFloat(string(strokeWidth.FindSubmatch(m)[1]), 64)
		if err != nil {
			return m
		}
		return []byte(fmt.Sprintf(`stroke-width="%g"`, w*scale))
	})
	icon, err := oksvg.ReadIconStream(bytes.NewReader(scaled))
	if err != nil {
		return nil, fmt.Errorf("parse the scaled SVG: %w", err)
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(size, size, scanner), 1)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// ico packs PNG images into an ICO file: a 6-byte header, a 16-byte directory
// entry per image, then the images.
func ico(images [][]byte) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(images))}) //nolint:gosec // nine entries
	offset := 6 + 16*len(images)
	for i, img := range images {
		dim := byte(sizes[i]) //nolint:gosec // deliberate: the format writes 256 as 0
		_ = binary.Write(&buf, le, struct {
			W, H, Colors, Reserved byte
			Planes, BitCount       uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(img)), uint32(offset)}) //nolint:gosec // small files
		offset += len(img)
	}
	for _, img := range images {
		buf.Write(img)
	}
	return buf.Bytes()
}

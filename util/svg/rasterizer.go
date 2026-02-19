//go:build rasterizesvg

package svg

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/anyproto/anytype-heart/core/files"
)

const pngMedia = "image/png"

func ProcessSvg(ctx context.Context, file files.File) (io.ReadSeeker, string, error) {
	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, "", err
	}
	icon, err := oksvg.ReadIconStream(reader)
	if err != nil {
		return nil, "", err
	}
	w, h := icon.ViewBox.W, icon.ViewBox.H
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	icon.Draw(rasterx.NewDasher(int(w), int(h), rasterx.NewScannerGV(int(w), int(h), img, img.Bounds())), 1)
	return writePNGToReader(img)
}

func writePNGToReader(img image.Image) (io.ReadSeeker, string, error) {
	pngBuffer := &bytes.Buffer{}
	err := png.Encode(pngBuffer, img)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(pngBuffer.Bytes()), pngMedia, nil
}

package svg

import (
	"bytes"
	"image"
	"image/png"
	"io"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func Rasterize(file io.ReadSeeker) (io.ReadSeeker, error) {
	icon, err := oksvg.ReadIconStream(file)
	if err != nil {
		return nil, err
	}
	w, h := icon.ViewBox.W, icon.ViewBox.H
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	icon.Draw(rasterx.NewDasher(int(w), int(h), rasterx.NewScannerGV(int(w), int(h), img, img.Bounds())), 1)
	return writePNGToReader(img)
}

func writePNGToReader(img image.Image) (io.ReadSeeker, error) {
	pngBuffer := &bytes.Buffer{}
	err := png.Encode(pngBuffer, img)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(pngBuffer.Bytes()), nil
}

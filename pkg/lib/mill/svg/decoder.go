package svg

import (
	"image"
	"io"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func Decode(r io.Reader) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(r)
	if err != nil {
		return nil, err
	}
	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)
	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())), 1)
	return rgba, nil
}

func DecodeConfig(r io.Reader) (image.Config, error) {
	icon, err := oksvg.ReadIconStream(r)
	if err != nil {
		return image.Config{}, err
	}
	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)
	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())), 1)
	return image.Config{
		ColorModel: rgba.ColorModel(),
		Width:      rgba.Bounds().Size().X,
		Height:     rgba.Bounds().Size().Y,
	}, nil
}

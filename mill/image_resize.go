package mill

import (
	"bytes"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strconv"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"

	"github.com/anytypeio/go-anytype-library/mill/ico"
)

// Format enumerates the type of images currently supported
type Format string

func init() {
	image.RegisterFormat("ico", string([]byte{0x00, 0x00, 0x01, 0x00}), ico.Decode, ico.DecodeConfig)
}

const (
	JPEG Format = "jpeg"
	PNG  Format = "png"
	GIF  Format = "gif"
	ICO  Format = "ico"
)

type ImageSize struct {
	Width  int
	Height int
}

type ImageResizeOpts struct {
	Width   string `json:"width"`
	Quality string `json:"quality"`
}

type ImageResize struct {
	Opts ImageResizeOpts
}

func (m *ImageResize) ID() string {
	return "/image/resize"
}

func (m *ImageResize) Encrypt() bool {
	return true
}

func (m *ImageResize) Pin() bool {
	return false
}

func (m *ImageResize) AcceptMedia(media string) error {
	return accepts([]string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/x-icon",
	}, media)
}

func (m *ImageResize) Options(add map[string]interface{}) (string, error) {
	return hashOpts(m.Opts, add)
}

func (m *ImageResize) Mill(input []byte, name string) (*Result, error) {
	img, formatStr, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	format := Format(formatStr)

	clean, err := removeExif(bytes.NewReader(input), img, format)
	if err != nil {
		return nil, err
	}

	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: " + m.Opts.Width)
	}
	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: " + m.Opts.Quality)
	}

	buff, rect, err := encodeImage(clean, format, width, quality)
	if err != nil {
		return nil, err
	}

	return &Result{
		File: buff.Bytes(),
		Meta: map[string]interface{}{
			"width":  rect.Dx(),
			"height": rect.Dy(),
		},
	}, nil
}

// removeExif strips exif data from an image
func removeExif(reader io.Reader, img image.Image, format Format) (io.Reader, error) {
	if format == GIF || format == ICO {
		return reader, nil
	}

	exf, _ := exif.Decode(reader)
	var err error
	img, err = correctOrientation(img, exf)
	if err != nil {
		return nil, err
	}

	// re-encoding will remove any exif
	return encodeSingleImage(img, format)
}

// encodeImage creates a jpeg|gif from reader (quality applies to jpeg only)
// NOTE: format is the reader image format, destination format is chosen accordingly.
func encodeImage(reader io.Reader, format Format, width int, quality int) (*bytes.Buffer, *image.Rectangle, error) {
	buff := new(bytes.Buffer)
	var size image.Rectangle

	if format != GIF {
		// encode to png or jpeg
		img, _, err := image.Decode(reader)
		if err != nil {
			return nil, nil, err
		}

		if img.Bounds().Size().X < width {
			width = img.Bounds().Size().X
		}

		resized := imaging.Resize(img, width, 0, imaging.Lanczos)

		if format == PNG || format == ICO {
			if err = png.Encode(buff, resized); err != nil {
				return nil, nil, err
			}
		} else {
			if err = jpeg.Encode(buff, resized, &jpeg.Options{Quality: quality}); err != nil {
				return nil, nil, err
			}
		}
		size = resized.Rect

	} else {
		// encode to gif
		img, err := gif.DecodeAll(reader)
		if err != nil {
			return nil, nil, err
		}
		if len(img.Image) == 0 {
			return nil, nil, fmt.Errorf("gif does not have any frames")
		}

		firstFrame := img.Image[0].Bounds()
		if firstFrame.Dx() < width {
			width = firstFrame.Dx()
		}
		rect := image.Rect(0, 0, firstFrame.Dx(), firstFrame.Dy())
		rgba := image.NewRGBA(rect)
		for index, frame := range img.Image {
			bounds := frame.Bounds()
			draw.Draw(rgba, bounds, frame, bounds.Min, draw.Over)
			img.Image[index] = imageToPaletted(imaging.Resize(rgba, width, 0, imaging.Lanczos))
		}

		img.Config.Width = img.Image[0].Bounds().Dx()
		img.Config.Height = img.Image[0].Bounds().Dy()

		if err = gif.EncodeAll(buff, img); err != nil {
			return nil, nil, err
		}

		size = img.Image[0].Bounds()
	}

	return buff, &size, nil
}

// correctOrientation returns a copy of an image (jpg|png|gif) with exif removed
func correctOrientation(img image.Image, exf *exif.Exif) (image.Image, error) {
	if exf == nil {
		return img, nil
	}

	orient, err := exf.Get(exif.Orientation)
	if err != nil && err != exif.TagNotPresentError(exif.Orientation) {
		return nil, err
	}
	if orient != nil {
		img = reverseOrientation(img, orient.String())
	} else {
		img = reverseOrientation(img, "1")
	}

	return img, nil
}

// encodeSingleImage creates a reader from an image
func encodeSingleImage(img image.Image, format Format) (*bytes.Reader, error) {
	writer := &bytes.Buffer{}
	var err error

	switch format {
	case JPEG:
		err = jpeg.Encode(writer, img, &jpeg.Options{Quality: 100})
	case PNG:
		// NOTE: while PNGs don't technically have exif data,
		// they can contain meta data with sensitive info
		err = png.Encode(writer, img)
	default:
		err = fmt.Errorf("unrecognized image format")
	}
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(writer.Bytes()), nil
}

// reverseOrientation transforms the given orientation to 1
func reverseOrientation(img image.Image, orientation string) *image.NRGBA {
	switch orientation {
	case "1":
		return imaging.Clone(img)
	case "2":
		return imaging.FlipV(img)
	case "3":
		return imaging.Rotate180(img)
	case "4":
		return imaging.Rotate180(imaging.FlipV(img))
	case "5":
		return imaging.Rotate270(imaging.FlipV(img))
	case "6":
		return imaging.Rotate270(img)
	case "7":
		return imaging.Rotate90(imaging.FlipV(img))
	case "8":
		return imaging.Rotate90(img)
	}

	log.Warningf("unknown orientation %s, expected 1-8", orientation)
	return imaging.Clone(img)
}

// imageToPaletted convert Image to Paletted for GIF handling
func imageToPaletted(img image.Image) *image.Paletted {
	b := img.Bounds()
	pm := image.NewPaletted(b, palette.Plan9)
	draw.FloydSteinberg.Draw(pm, b, img, image.ZP)
	return pm
}

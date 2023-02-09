//go:build cgo && !noheic

package mill

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"strconv"

	"github.com/adrium/goheif"
	"github.com/disintegration/imaging"
)

func (m *ImageResize) resizeHEIC(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	goheif.SafeEncoding = true
	img, err := goheif.Decode(r)

	var height int
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: " + m.Opts.Width)
	}

	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: " + m.Opts.Quality)
	}

	buff := &bytes.Buffer{}

	if err = jpeg.Encode(buff, resized, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}

	return &Result{
		File: buff,
		Meta: map[string]interface{}{
			"width":  width,
			"height": height,
		},
	}, nil
}

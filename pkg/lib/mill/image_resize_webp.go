//go:build cgo && !nowebp

package mill

import (
	"fmt"
	"image"
	"io"
	"strconv"

	"github.com/chai2010/webp"
	"github.com/kovidgoyal/imaging"
)

func (m *ImageResize) resizeWEBP(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	var height int
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: %s", m.Opts.Width)
	}

	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: %s", m.Opts.Quality)
	}

	if imgConfig.Width <= width || width == 0 {
		// we will not do the upscale
		width, height = imgConfig.Width, imgConfig.Height
	}

	if width == imgConfig.Width {
		// here is an optimization
		// lets return the original picture in case it has not been resized or normalized
		return &Result{
			File: noopCloser(r),
			Meta: map[string]interface{}{
				"width":  imgConfig.Width,
				"height": imgConfig.Height,
			},
		}, nil
	}

	img, err := webp.Decode(r)
	if err != nil {
		return nil, err
	}

	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	buf := pool.Get()
	defer func() {
		_ = buf.Close()
	}()
	if webp.Encode(buf, resized, &webp.Options{Quality: float32(quality)}) != nil {
		return nil, err
	}

	readSeekCloser, err := buf.GetReadSeekCloser()
	if err != nil {
		return nil, err
	}

	return &Result{
		File: readSeekCloser,
		Meta: map[string]interface{}{
			"width":  width,
			"height": height,
		},
	}, nil
}

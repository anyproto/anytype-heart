package mill

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"strconv"

	"github.com/kovidgoyal/imaging"
	"github.com/oov/psd"
)

func (m *ImageResize) resizePSD(imgConfig *image.Config, r io.ReadSeeker) (res *Result, err error) {
	img, _, err := psd.Decode(r, &psd.DecodeOptions{SkipLayerImage: true})
	if err != nil {
		return nil, err
	}

	var height int
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: %s", m.Opts.Width)
	}

	resized := imaging.Resize(img.Picker, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: %s", m.Opts.Quality)
	}

	buf := pool.Get()
	defer func() {
		_ = buf.Close()
	}()

	// encode to jpeg encoding to increase compatibility on mobile devices
	if err = jpeg.Encode(buf, resized, &jpeg.Options{Quality: quality}); err != nil {
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

//go:build cgo && !noheic

package mill

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"strconv"

	"github.com/adrium/goheif"
	"github.com/kovidgoyal/imaging"
)

func (m *ImageResize) resizeHEIC(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	goheif.SafeEncoding = true
	img, err := goheif.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode heic: %w", err)
	}

	var height int
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: %s", m.Opts.Width)
	}

	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: %s", m.Opts.Quality)
	}

	buf := pool.Get()
	defer func() {
		_ = buf.Close()
	}()

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

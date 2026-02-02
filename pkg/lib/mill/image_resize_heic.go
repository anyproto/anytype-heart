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
	"github.com/kovidgoyal/imaging"
)

func (m *ImageResize) resizeHEIC(r io.ReadSeeker) (*Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read heic: %w", err)
	}

	// Get configuration (dimensions, rotation) from metadata
	cfg, err := getHEICConfig(data)
	if err != nil {
		return nil, err
	}

	// Map irot rotation to EXIF orientation
	var orientation int
	switch cfg.Rotations {
	case 0:
		orientation = 1
	case 1:
		orientation = 8
	case 2:
		orientation = 3
	case 3:
		orientation = 6
	}

	// Decode the image - try standard decoding first, then reordered
	goheif.SafeEncoding = true
	var img image.Image
	img, err = goheif.Decode(bytes.NewReader(data))
	if err != nil {
		// Try with reordered boxes for files where mdat comes before meta
		reordered, reorderErr := reorderHEICFull(data)
		if reorderErr != nil {
			return nil, fmt.Errorf("decode heic: %w (reorder failed: %v)", err, reorderErr)
		}
		img, err = goheif.Decode(bytes.NewReader(reordered))
		if err != nil {
			return nil, fmt.Errorf("decode heic after reorder: %w", err)
		}
	}

	if orientation > 1 {
		img = reverseOrientation(img, orientation)
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
		Meta: map[string]any{
			"width":  width,
			"height": height,
		},
	}, nil
}

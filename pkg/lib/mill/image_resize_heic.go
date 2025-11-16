//go:build cgo && !noheic

package mill

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"strconv"

	"github.com/adrium/goheif"
	"github.com/adrium/goheif/heif"
	"github.com/kovidgoyal/imaging"
)

func (m *ImageResize) resizeHEIC(r io.ReadSeeker) (*Result, error) {
	orientation, err := getHEICOrientation(r)
	if err != nil {
		return nil, err
	}

	goheif.SafeEncoding = true
	img, err := goheif.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode heic: %w", err)
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
		Meta: map[string]interface{}{
			"width":  width,
			"height": height,
		},
	}, nil
}

func getHEICOrientation(r io.ReadSeeker) (int, error) {
	var rotations int
	ra, ok := r.(io.ReaderAt)
	if !ok {
		data, err := io.ReadAll(r)
		if err != nil {
			return 0, fmt.Errorf("read heic: %w", err)
		}
		ra = bytes.NewReader(data)
		r = bytes.NewReader(data)
	}

	hf := heif.Open(ra)
	it, err := hf.PrimaryItem()
	if err != nil {
		return 0, fmt.Errorf("get primary item: %w", err)
	}
	rotations = it.Rotations()

	// Seek back to start for decoding
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek: %w", err)
	}

	// Map irot values (0-3) to EXIF orientation values
	// irot: 0=no rotation, 1=90°CCW, 2=180°, 3=270°CCW (90°CW)
	// EXIF: 1=normal, 8=90°CCW, 3=180°, 6=90°CW
	var orientation int
	switch rotations {
	case 0:
		orientation = 1 // normal
	case 1:
		orientation = 8 // 90° counter-clockwise
	case 2:
		orientation = 3 // 180°
	case 3:
		orientation = 6 // 90° clockwise (270° CCW)
	}

	return orientation, nil
}

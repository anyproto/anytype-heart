//go:build !webpresize

package mill

import (
	"fmt"
	"image"
	"io"
)

func (m *ImageResize) Mill(r io.ReadSeeker, name string) (*Result, error) {
	imgConfig, formatStr, err := image.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	format := Format(formatStr)

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	switch format {
	case JPEG:
		return m.resizeJPEG(&imgConfig, r)
	case ICO, PNG:
		return m.resizePNG(&imgConfig, r)
	case GIF:
		return m.resizeGIF(&imgConfig, r)
	}

	return nil, fmt.Errorf("unknown format")
}

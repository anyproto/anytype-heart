//go:build !webpresize

package mill

import (
	"image"
	"io"
)

func (m *ImageResize) resizeWEBP(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	return nil, ErrWEBPNotSupported
}

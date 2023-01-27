//go:build !cgo || noheic

package mill

import (
	"image"
	"io"
)

func (m *ImageResize) resizeHEIC(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	return nil, ErrFormatSupportNotEnabled
}

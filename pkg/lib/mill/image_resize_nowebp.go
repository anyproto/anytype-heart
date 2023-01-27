//go:build !cgo || nowebp

package mill

import (
	"image"
	"io"
)

func (m *ImageResize) resizeWEBP(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	return nil, ErrFormatSupportNotEnabled
}

//go:build !cgo || nowebp

package mill

import (
	"image"
	"io"
)

func (m *ImageResize) resizeWEBP(_ *image.Config, _ io.ReadSeeker) (*Result, error) {
	return nil, ErrFormatSupportNotEnabled
}

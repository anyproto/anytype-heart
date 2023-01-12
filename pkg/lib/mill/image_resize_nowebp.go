//go:build !webpresize

package mill

import (
	"fmt"
	"image"
	"io"
)

func (m *ImageResize) resizeWEBP(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	return nil, fmt.Errorf("webp image format is not supported")
}

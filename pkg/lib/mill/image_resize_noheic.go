//go:build !cgo || noheic

package mill

import (
	"image"
	"io"
)

func init() {
	image.RegisterFormat("heic", "????ftyp", noHEICDecode, noHEICDecodeConfig)
}

func noHEICDecode(io.Reader) (image.Image, error) {
	return nil, ErrFormatSupportNotEnabled
}

func noHEICDecodeConfig(io.Reader) (image.Config, error) {
	return image.Config{}, ErrFormatSupportNotEnabled
}

func (m *ImageResize) resizeHEIC(_ io.ReadSeeker) (*Result, error) {
	return nil, ErrFormatSupportNotEnabled
}

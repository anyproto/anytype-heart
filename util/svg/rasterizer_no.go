//go:build !rasterizesvg

package svg

import (
	"context"
	"io"

	"github.com/anyproto/anytype-heart/core/files"
)

const svgMedia = "image/svg+xml"

func ProcessSvg(ctx context.Context, file files.File) (io.ReadSeeker, error) {
	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, err
	}
	file.Info().Media = svgMedia
	return reader, nil
}

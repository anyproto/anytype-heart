package schema

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

var log = logging.Logger("anytype-core-mill")

func GetMill(id string, opts map[string]string) (mill.Mill, error) {
	switch id {
	case "/blob":
		return &mill.Blob{}, nil
	case "/image/resize":
		width := opts["width"]
		if width == "" {
			return nil, fmt.Errorf("missing width")
		}
		quality := opts["quality"]
		if quality == "" {
			quality = "75"
		}
		return &mill.ImageResize{
			Opts: mill.ImageResizeOpts{
				Width:   width,
				Quality: quality,
			},
		}, nil
	case "/image/exif":
		return &mill.ImageExif{}, nil

	default:
		return nil, nil
	}
}

var ImageResizeSchema = &storage.ImageResizeSchema{
	Name: "image",
	Links: []*storage.Link{
		{
			Name: "original",
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "0",
				"quality": "100",
			},
		},
		{
			Name: "large",
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "1920",
				"quality": "85",
			},
		},
		{
			Name: "small",
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "320",
				"quality": "80",
			},
		},
		{
			Name: "thumbnail",
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "100",
				"quality": "80",
			},
		},
		{
			Name: "exif",
			Mill: "/image/exif",
		},
	},
}

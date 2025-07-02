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

const (
	// We have legacy nodes structure that allowed us to add directories and "0" means the first directory
	// Now we have only one directory in which we have either single file or image variants
	LinkFile = "0"

	LinkImageOriginal  = "original"
	LinkImageLarge     = "large"
	LinkImageSmall     = "small"
	LinkImageThumbnail = "thumbnail"
	LinkImageExif      = "exif"
)

var ImageResizeSchema = &storage.ImageResizeSchema{
	Name: "image",
	Links: []*storage.Link{
		{
			Name: LinkImageOriginal,
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "0",
				"quality": "100",
			},
		},
		{
			Name: LinkImageLarge,
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "1920",
				"quality": "85",
			},
		},
		{
			Name: LinkImageSmall,
			Mill: "/image/resize",
			Opts: map[string]string{
				"width":   "320",
				"quality": "80",
			},
		},
		{
			Name: LinkImageExif,
			Mill: "/image/exif",
		},
	},
}

package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) ImageByHash(ctx context.Context, hash string) (Image, error) {
	files, err := a.fileStore.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	// check the image files count explicitly because we have a bug when the info can be cached not fully(only for some files)
	if len(files) < 4 || files[0].MetaHash == "" {
		// index image files info from ipfs
		files, err = a.files.FileIndexInfo(ctx, hash, true)
		if err != nil {
			log.Errorf("ImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrImageNotFound
		}
	}

	var variantsByWidth = make(map[int]*storage.FileInfo, len(files))
	for _, f := range files {
		if f.Mill != "/image/resize" {
			continue
		}

		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}

	return &image{
		hash:            files[0].Targets[0],
		variantsByWidth: variantsByWidth,
		service:         a.files,
	}, nil
}

// TODO: Touch the file to fire indexing
func (a *Anytype) ImageAdd(ctx context.Context, options ...files.AddOption) (Image, error) {
	opts := files.AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := a.files.NormalizeOptions(ctx, &opts)
	if err != nil {
		return nil, err
	}

	hash, variants, err := a.files.ImageAdd(ctx, opts)
	if err != nil {
		return nil, err
	}

	img := &image{
		hash:            hash,
		variantsByWidth: variants,
		service:         a.files,
	}
	return img, nil
}

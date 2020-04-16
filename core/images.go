package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/storage"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) ImageByHash(ctx context.Context, hash string) (Image, error) {
	files, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		files, err = a.getFileIndexes(ctx, hash)
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
		node:            a,
	}, nil
}

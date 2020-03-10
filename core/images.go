package core

import (
	"fmt"

	tpb "github.com/textileio/go-textile/pb"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) getFileIndexes(hash string) ([]tpb.FileIndex, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *Anytype) ImageByHash(hash string) (Image, error) {
	files, err := a.getFileIndexByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		files, err = a.getFileIndexes(hash)
		if err != nil {
			log.Errorf("fImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrImageNotFound
		}
	}

	var variantsByWidth = make(map[int]tpb.FileIndex, len(files))
	for _, f := range files {
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

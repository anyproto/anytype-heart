package core

import (
	"fmt"

	tpb "github.com/textileio/go-textile/pb"
)

func (a *Anytype) ImageByHash(hash string) (*image, error) {
	files, err := a.getFileIndexByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		// todo: extract from IPFS
		return nil, fmt.Errorf("image not found")
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

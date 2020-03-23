package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-library/ipfs"
	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) getFileIndexes(ctx context.Context, hash string) ([]lsmodel.FileInfo, error) {
	links, err := ipfs.LinksAtPath(ctx, a.ts.GetIpfsLite(), hash)
	if err != nil {
		return nil, err
	}

	filesKeysCacheMutex.RLock()
	defer filesKeysCacheMutex.RUnlock()

	filesKeys, filesKeysExists := filesKeysCache[hash]

	var files []lsmodel.FileInfo
	for _, index := range links {
		node, err := ipfs.NodeAtLink(ctx, a.ipfs(), index)
		if err != nil {
			return nil, err
		}

		if looksLikeFileNode(node) {
			var key string
			if filesKeysExists {
				key = filesKeys["/"+index.Name+"/"]
			}

			fileIndex, err := a.addFileIndexFromPath(hash, hash+"/"+index.Name, key)
			if err != nil {
				return nil, fmt.Errorf("addFileIndexFromPath error: %s", err.Error())
			}
			files = append(files, *fileIndex)
		} else {
			for _, link := range node.Links() {
				var key string
				if filesKeysExists {
					key = filesKeys["/"+index.Name+"/"+link.Name+"/"]
				}

				fileIndex, err := a.addFileIndexFromPath(hash, hash+"/"+index.Name+"/"+link.Name, key)
				if err != nil {
					return nil, fmt.Errorf("addFileIndexFromPath error: %s", err.Error())
				}
				files = append(files, *fileIndex)
			}
		}
	}

	return files, nil
}

func (a *Anytype) ImageByHash(ctx context.Context, hash string) (Image, error) {
	files, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	/*if len(files) == 0 {
		files, err = a.getFileIndexes(ctx, hash)
		if err != nil {
			log.Errorf("fImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrImageNotFound
		}
	}*/

	var variantsByWidth = make(map[int]*lsmodel.FileInfo, len(files))
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

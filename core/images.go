package core

import (
	"fmt"

	"github.com/textileio/go-textile/ipfs"
	tpb "github.com/textileio/go-textile/pb"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) getFileIndexes(hash string) ([]tpb.FileIndex, error) {
	links, err := ipfs.LinksAtPath(a.ipfs(), hash)
	if err != nil {
		return nil, err
	}

	filesKeysCacheMutex.RLock()
	defer filesKeysCacheMutex.RUnlock()

	filesKeys, filesKeysExists := filesKeysCache[hash]

	var files []tpb.FileIndex

	for _, index := range links {
		node, err := ipfs.NodeAtLink(a.ipfs(), index)
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

func (a *Anytype) ImageByHash(hash string) (*image, error) {
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

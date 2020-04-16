package files

import (
	"context"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/schema/anytype"
)

func (s *Service) ImageAdd(ctx context.Context, opts AddOptions) (string, map[int]*storage.FileInfo, error) {
	b, err := ioutil.ReadAll(opts.Reader)
	if err != nil {
		return "", nil, err
	}

	dir, err := s.fileBuildDirectory(ctx, b, opts.Name, anytype.ImageNode())
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromDirs(ctx, &storage.DirectoryList{Items: []*storage.Directory{dir}})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()

	s.KeysCacheMutex.Lock()
	defer s.KeysCacheMutex.Unlock()
	s.KeysCache[nodeHash] = keys.KeysByPath

	err = s.fileIndexData(ctx, node, nodeHash)
	if err != nil {
		return "", nil, err
	}

	var variantsByWidth = make(map[int]*storage.FileInfo, len(dir.Files))
	for _, f := range dir.Files {
		if f.Mill != "/image/resize" {
			continue
		}
		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}

	return nodeHash, variantsByWidth, nil
}

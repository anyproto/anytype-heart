package files

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill/schema/anytype"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

func (s *Service) ImageAdd(ctx context.Context, opts AddOptions) (string, map[int]*storage.FileInfo, error) {
	dir, err := s.fileBuildDirectory(ctx, opts.Reader, opts.Name, opts.Plaintext, anytype.ImageNode())
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromDirs(ctx, &storage.DirectoryList{Items: []*storage.Directory{dir}})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()

	err = s.store.AddFileKeys(filestore.FileKeys{
		Hash: nodeHash,
		Keys: keys.KeysByPath,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to save file keys: %w", err)
	}

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

	go func() {
		if err := s.pins.FilePin(nodeHash); err != nil {
			log.Warnf("cannot pin image %s: %v", nodeHash, err)
		} else {
			log.Debugf("pinning image %s started on the cafe", nodeHash)
		}
	}()

	return nodeHash, variantsByWidth, nil
}

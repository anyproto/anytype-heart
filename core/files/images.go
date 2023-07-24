package files

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema/anytype"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (s *service) ImageByHash(ctx context.Context, id domain.FullID) (Image, error) {
	ok, err := s.isDeleted(id.ObjectID)
	if err != nil {
		return nil, fmt.Errorf("check if file is deleted: %w", err)
	}
	if ok {
		return nil, ErrFileNotFound
	}

	files, err := s.fileStore.ListByTarget(id.ObjectID)
	if err != nil {
		return nil, err
	}

	// check the image files count explicitly because we have a bug when the info can be cached not fully(only for some files)
	if len(files) < 4 || files[0].MetaHash == "" {
		// index image files info from ipfs
		files, err = s.fileIndexInfo(ctx, id, true)
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
		spaceID:         id.SpaceID,
		hash:            files[0].Targets[0],
		variantsByWidth: variantsByWidth,
		service:         s,
	}, nil
}

// TODO: Touch the file to fire indexing
func (s *service) ImageAdd(ctx context.Context, spaceID string, options ...AddOption) (Image, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(ctx, spaceID, &opts)
	if err != nil {
		return nil, err
	}

	hash, variants, err := s.imageAdd(ctx, spaceID, opts)
	if err != nil {
		return nil, err
	}

	img := &image{
		spaceID:         spaceID,
		hash:            hash,
		variantsByWidth: variants,
		service:         s,
	}
	return img, nil
}

func (s *service) imageAdd(ctx context.Context, spaceID string, opts AddOptions) (string, map[int]*storage.FileInfo, error) {
	dir, err := s.fileBuildDirectory(ctx, spaceID, opts.Reader, opts.Name, opts.Plaintext, anytype.ImageNode())
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromDirs(ctx, spaceID, &storage.DirectoryList{Items: []*storage.Directory{dir}})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()
	err = s.fileStore.AddFileKeys(filestore.FileKeys{
		Hash: nodeHash,
		Keys: keys.KeysByPath,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	err = s.fileIndexData(ctx, node, domain.FullID{SpaceID: spaceID, ObjectID: nodeHash})
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

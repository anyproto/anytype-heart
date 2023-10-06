package files

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func (s *service) ImageByHash(ctx context.Context, id domain.FullID) (Image, error) {
	ok, err := s.isDeleted(id.ObjectID)
	if err != nil {
		return nil, fmt.Errorf("check if file is deleted: %w", err)
	}
	if ok {
		return nil, domain.ErrFileNotFound
	}

	files, err := s.fileStore.ListByTarget(id.ObjectID)
	if err != nil {
		return nil, err
	}

	// TODO Can we use FileByHash here? FileByHash contains important syncing logic. Yes, we use FileByHash before ImageByHash
	// 	but it doesn't seem to be clear why we repeat file indexing process here

	// check the image files count explicitly because we have a bug when the info can be cached not fully(only for some files)
	if len(files) < 4 || files[0].MetaHash == "" {
		// index image files info from ipfs
		files, err = s.fileIndexInfo(ctx, id, true)
		if err != nil {
			log.Errorf("ImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, domain.ErrFileNotFound
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
	origin := s.getFileOrigin(id.ObjectID)
	return &image{
		spaceID:         id.SpaceID,
		hash:            id.ObjectID,
		variantsByWidth: variantsByWidth,
		service:         s,
		origin:          origin,
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
	dir, err := s.fileBuildDirectory(ctx, spaceID, opts.Reader, opts.Name, opts.Plaintext, schema.ImageNode())
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

	id := domain.FullID{SpaceID: spaceID, ObjectID: nodeHash}
	err = s.fileIndexData(ctx, node, id, s.isImported(opts.Origin))
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

	err = s.storeFileSize(spaceID, nodeHash)
	if err != nil {
		return "", nil, fmt.Errorf("store file size: %w", err)
	}

	err = s.fileStore.SetFileOrigin(nodeHash, opts.Origin)
	if err != nil {
		log.Errorf("failed to set file origin %s: %s", nodeHash, err)
	}

	return nodeHash, variantsByWidth, nil
}

func (s *service) isImported(origin *model.ObjectOrigin) bool {
	if origin == nil {
		return false
	}
	return *origin == model.ObjectOrigin_import
}

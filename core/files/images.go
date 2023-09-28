package files

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema/anytype"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func (s *service) ImageByHash(ctx context.Context, hash string) (Image, error) {
	ok, err := s.isDeleted(hash)
	if err != nil {
		return nil, fmt.Errorf("check if file is deleted: %w", err)
	}
	if ok {
		return nil, domain.ErrFileNotFound
	}

	files, err := s.fileStore.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	// check the image files count explicitly because we have a bug when the info can be cached not fully(only for some files)
	if len(files) < 4 || files[0].MetaHash == "" {
		// index image files info from ipfs
		files, err = s.fileIndexInfo(ctx, hash, true)
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

	return &image{
		hash:            hash,
		variantsByWidth: variantsByWidth,
		service:         s,
	}, nil
}

// TODO: Touch the file to fire indexing
func (s *service) ImageAdd(ctx context.Context, options ...AddOption) (Image, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(ctx, &opts)
	if err != nil {
		return nil, err
	}

	hash, variants, err := s.imageAdd(ctx, opts)
	if err != nil {
		return nil, err
	}

	img := &image{
		hash:            hash,
		variantsByWidth: variants,
		service:         s,
	}
	return img, nil
}

func (s *service) imageAdd(ctx context.Context, opts AddOptions) (string, map[int]*storage.FileInfo, error) {
	dir, err := s.fileBuildDirectory(ctx, opts.Reader, opts.Name, opts.Plaintext, anytype.ImageNode())
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromDirs(ctx, &storage.DirectoryList{Items: []*storage.Directory{dir}})
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

	err = s.fileIndexData(ctx, node, nodeHash, opts.Imported)
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

	err = s.storeFileSize(nodeHash, node)
	if err != nil {
		return "", nil, fmt.Errorf("store file size: %w", err)
	}

	return nodeHash, variantsByWidth, nil
}

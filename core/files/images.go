package files

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func (s *service) ImageByHash(ctx context.Context, id domain.FullFileId) (Image, error) {
	files, err := s.fileStore.ListChildrenByFileId(id.FileId)
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
			log.Errorf("ImageByHash: failed to retrieve from IPFS: %s", err)
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
	origin := s.getFileOrigin(id.FileId)
	return &image{
		spaceID:         id.SpaceId,
		fileId:          id.FileId,
		variantsByWidth: variantsByWidth,
		service:         s,
		origin:          origin,
	}, nil
}

type ImageAddResult struct {
	FileId         domain.FileId
	Image          Image
	EncryptionKeys *domain.FileKeys
	IsExisting     bool
}

func (s *service) ImageAdd(ctx context.Context, spaceId string, options ...AddOption) (*ImageAddResult, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(ctx, spaceId, &opts)
	if err != nil {
		return nil, err
	}

	dir, err := s.buildImageVariants(ctx, spaceId, opts.Reader, opts.Name, opts.Plaintext)
	if errors.Is(err, errFileExists) {
		return s.newExisingImageResult(spaceId, dir)
	}
	if err != nil {
		return nil, err
	}

	node, keys, err := s.fileAddNodeFromDir(ctx, spaceId, dir)
	if err != nil {
		return nil, err
	}

	nodeHash := node.Cid().String()

	fileId := domain.FileId(nodeHash)
	fileKeys := domain.FileKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	id := domain.FullFileId{SpaceId: spaceId, FileId: fileId}
	err = s.fileIndexData(ctx, node, id, s.isImported(opts.Origin))
	if err != nil {
		return nil, err
	}

	err = s.storeFileSize(spaceId, fileId)
	if err != nil {
		return nil, fmt.Errorf("store file size: %w", err)
	}

	err = s.fileStore.SetFileOrigin(fileId, opts.Origin)
	if err != nil {
		log.Errorf("failed to set file origin %s: %s", fileId.String(), err)
	}

	return &ImageAddResult{
		FileId:         fileId,
		Image:          s.newImage(spaceId, fileId, dir),
		EncryptionKeys: &fileKeys,
	}, nil
}

func (s *service) isImported(origin model.ObjectOrigin) bool {
	return origin == model.ObjectOrigin_import
}

func (s *service) newExisingImageResult(spaceId string, dir *storage.Directory) (*ImageAddResult, error) {
	for _, fileInfo := range dir.Files {
		fileId, keys, err := s.getFileIdAndEncryptionKeysFromInfo(fileInfo)
		if err != nil {
			return nil, err
		}
		return &ImageAddResult{
			IsExisting:     true,
			FileId:         fileId,
			Image:          s.newImage(spaceId, fileId, dir),
			EncryptionKeys: keys,
		}, nil
	}
	return nil, errors.New("image directory is empty")
}

func newVariantsByWidth(dir *storage.Directory) map[int]*storage.FileInfo {
	variantsByWidth := make(map[int]*storage.FileInfo, len(dir.Files))
	for _, f := range dir.Files {
		if f.Mill != "/image/resize" {
			continue
		}
		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}
	return variantsByWidth
}

func (s *service) newImage(spaceId string, fileId domain.FileId, dir *storage.Directory) Image {
	variantsByWidth := newVariantsByWidth(dir)
	return &image{
		spaceID:         spaceId,
		fileId:          fileId,
		variantsByWidth: variantsByWidth,
		service:         s,
	}
}

package files

import (
	"context"
	"errors"
	"fmt"

	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func (s *service) ImageByHash(ctx context.Context, id domain.FullFileId) (Image, error) {
	files, err := s.fileStore.ListFileVariants(id.FileId)
	if err != nil {
		return nil, err
	}

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
	return &image{
		spaceID:         id.SpaceId,
		fileId:          id.FileId,
		variantsByWidth: variantsByWidth,
		service:         s,
	}, nil
}

func (s *service) ImageAdd(ctx context.Context, spaceId string, options ...AddOption) (*AddResult, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(&opts)
	if err != nil {
		return nil, err
	}
	addLock := s.lockAddOperation(opts.checksum)

	dirEntries, err := s.addImageNodes(ctx, spaceId, opts)
	var errExists *errFileExists
	if errors.As(err, &errExists) {
		res, err := s.newExistingFileResult(addLock, errExists.fileId)
		if err != nil {
			addLock.Unlock()
			return nil, err
		}
		return res, nil
	}
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	if len(dirEntries) == 0 {
		addLock.Unlock()
		return nil, errors.New("no image variants")
	}

	rootNode, keys, err := s.addImageRootNode(ctx, spaceId, dirEntries)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	fileId := domain.FileId(rootNode.Cid().String())
	fileKeys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	id := domain.FullFileId{SpaceId: spaceId, FileId: fileId}
	var successfullyAdded []domain.FileContentId
	for _, variant := range dirEntries {
		variant.fileInfo.Targets = []string{id.FileId.String()}
		err = s.fileStore.AddFileVariant(variant.fileInfo)
		if err != nil && !errors.Is(err, localstore.ErrDuplicateKey) {
			// Cleanup
			deleteErr := s.fileStore.DeleteFileVariants(successfullyAdded)
			if deleteErr != nil {
				log.Errorf("cleanup: failed to delete file variants %s", deleteErr)
			}
			addLock.Unlock()
			return nil, fmt.Errorf("failed to store file variant: %w", err)
		}
		successfullyAdded = append(successfullyAdded, domain.FileContentId(variant.fileInfo.Hash))
	}

	err = s.storeFileSize(spaceId, fileId)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("store file size: %w", err)
	}

	entry := dirEntries[0]
	return &AddResult{
		FileId:         fileId,
		MIME:           entry.fileInfo.Media,
		Size:           entry.fileInfo.Size_,
		EncryptionKeys: &fileKeys,
		lock:           addLock,
	}, nil
}

func (s *service) addImageNodes(ctx context.Context, spaceID string, addOpts AddOptions) ([]dirEntry, error) {
	sch := schema.ImageResizeSchema
	if len(sch.Links) == 0 {
		return nil, schema.ErrEmptySchema
	}

	dirEntries := make([]dirEntry, 0, len(sch.Links))
	for _, link := range sch.Links {
		stepMill, err := schema.GetMill(link.Mill, link.Opts)
		if err != nil {
			return nil, err
		}
		opts := &AddOptions{
			Reader:               addOpts.Reader,
			Media:                "",
			Name:                 addOpts.Name,
			CustomEncryptionKeys: addOpts.CustomEncryptionKeys,
		}
		err = s.normalizeOptions(opts)
		if err != nil {
			return nil, err
		}
		added, fileNode, err := s.addFileNode(ctx, spaceID, stepMill, *opts, link.Name)
		if err != nil {
			return nil, err
		}
		dirEntries = append(dirEntries, dirEntry{
			name:     link.Name,
			fileInfo: added,
			fileNode: fileNode,
		})
		addOpts.Reader.Seek(0, 0)
	}
	return dirEntries, nil
}

// addImageRootNode has structure:
/*
- dir (outer)
	- dir (0)
		- dir (original)
			- meta
			- content
		- dir (large)
			- meta
			- content
	...
*/
func (s *service) addImageRootNode(ctx context.Context, spaceID string, dirEntries []dirEntry) (ipld.Node, *storage.FileKeys, error) {
	dagService := s.dagServiceForSpace(spaceID)
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}

	outer := uio.NewDirectory(dagService)
	outer.SetCidBuilder(cidBuilder)

	inner := uio.NewDirectory(dagService)
	inner.SetCidBuilder(cidBuilder)

	for _, entry := range dirEntries {

		err := helpers.AddLinkToDirectory(ctx, dagService, inner, entry.name, entry.fileNode.Cid().String())
		if err != nil {
			return nil, nil, err
		}
		keys.KeysByPath[encryptionKeyPath(entry.name)] = entry.fileInfo.Key
	}

	node, err := inner.GetNode()
	if err != nil {
		return nil, nil, err
	}
	err = dagService.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}

	id := node.Cid().String()
	err = helpers.AddLinkToDirectory(ctx, dagService, outer, schema.LinkFile, id)
	if err != nil {
		return nil, nil, err
	}

	outerNode, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}
	err = dagService.Add(ctx, outerNode)
	if err != nil {
		return nil, nil, err
	}
	return outerNode, keys, nil
}

func newVariantsByWidth(dirEntries []dirEntry) map[int]*storage.FileInfo {
	variantsByWidth := make(map[int]*storage.FileInfo, len(dirEntries))
	for _, entry := range dirEntries {
		if entry.fileInfo.Mill != "/image/resize" {
			continue
		}
		if v, exists := entry.fileInfo.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = entry.fileInfo
		}
	}
	return variantsByWidth
}

func (s *service) newImage(spaceId string, fileId domain.FileId, dirEntries []dirEntry) Image {
	variantsByWidth := newVariantsByWidth(dirEntries)
	return &image{
		spaceID:         spaceId,
		fileId:          fileId,
		variantsByWidth: variantsByWidth,
		service:         s,
	}
}

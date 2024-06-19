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
			return nil, err
		}
	}

	return &image{
		spaceID:            id.SpaceId,
		fileId:             id.FileId,
		onlyResizeVariants: selectAndSortResizeVariants(files),
		service:            s,
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

	addNodesResult, err := s.addImageNodes(ctx, spaceId, opts)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	if addNodesResult.isExisting {
		res, err := s.newExistingFileResult(addLock, addNodesResult.fileId)
		if err != nil {
			addLock.Unlock()
			return nil, err
		}
		return res, nil
	}
	if len(addNodesResult.dirEntries) == 0 {
		addLock.Unlock()
		return nil, errors.New("no image variants")
	}

	rootNode, keys, err := s.addImageRootNode(ctx, spaceId, addNodesResult.dirEntries)
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

	dirEntries := addNodesResult.dirEntries
	id := domain.FullFileId{SpaceId: spaceId, FileId: fileId}
	successfullyAdded := make([]domain.FileContentId, 0, len(dirEntries))
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

	entry := dirEntries[0]
	return &AddResult{
		FileId:         fileId,
		MIME:           entry.fileInfo.Media,
		Size:           entry.fileInfo.Size_,
		EncryptionKeys: &fileKeys,
		lock:           addLock,
	}, nil
}

type addImageNodesResult struct {
	isExisting bool
	fileId     domain.FileId
	dirEntries []dirEntry
}

func newExistingImageResult(fileId domain.FileId) *addImageNodesResult {
	return &addImageNodesResult{
		isExisting: true,
		fileId:     fileId,
	}
}

func newImageNodesResult(dirEntries []dirEntry) *addImageNodesResult {
	return &addImageNodesResult{
		dirEntries: dirEntries,
	}
}

func (s *service) addImageNodes(ctx context.Context, spaceID string, addOpts AddOptions) (*addImageNodesResult, error) {
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
		addNodeResult, err := s.addFileNode(ctx, spaceID, stepMill, *opts, link.Name)
		if err != nil {
			return nil, err
		}
		if addNodeResult.isExisting {
			return newExistingImageResult(addNodeResult.fileId), nil
		}
		dirEntries = append(dirEntries, dirEntry{
			name:     link.Name,
			fileInfo: addNodeResult.variant,
			fileNode: addNodeResult.filePairNode,
		})
		addOpts.Reader.Seek(0, 0)
	}
	return newImageNodesResult(dirEntries), nil
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

package files

import (
	"context"
	"errors"
	"fmt"
	"io"

	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func (s *service) ImageByHash(ctx context.Context, id domain.FullFileId) (Image, error) {
	files, err := s.fileStore.ListFileVariants(id.FileId)
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
	EncryptionKeys *domain.FileEncryptionKeys
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

	dirEntries, err := s.addImageNodes(ctx, spaceId, opts.Reader, opts.Name)
	if errors.Is(err, errFileExists) {
		return s.newExisingImageResult(spaceId, dirEntries)
	}
	if err != nil {
		return nil, err
	}

	rootNode, keys, err := s.addImageRootNode(ctx, spaceId, dirEntries)
	if err != nil {
		return nil, err
	}
	fileId := domain.FileId(rootNode.Cid().String())
	fileKeys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	id := domain.FullFileId{SpaceId: spaceId, FileId: fileId}
	err = s.fileIndexData(ctx, rootNode, id, s.isImported(opts.Origin))
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
		Image:          s.newImage(spaceId, fileId, dirEntries),
		EncryptionKeys: &fileKeys,
	}, nil
}

func (s *service) addImageNodes(ctx context.Context, spaceID string, reader io.ReadSeeker, filename string) ([]dirEntry, error) {
	sch := schema.ImageResizeSchema
	if len(sch.Links) == 0 {
		return nil, schema.ErrEmptySchema
	}

	var isExisting bool
	dirEntries := make([]dirEntry, 0, len(sch.Links))
	for _, link := range sch.Links {
		stepMill, err := schema.GetMill(link.Mill, link.Opts)
		if err != nil {
			return nil, err
		}
		opts := &AddOptions{
			Reader: reader,
			Use:    "",
			Media:  "",
			Name:   filename,
		}
		err = s.normalizeOptions(ctx, spaceID, opts)
		if err != nil {
			return nil, err
		}
		added, fileNode, err := s.addFileNode(ctx, spaceID, stepMill, *opts)
		if errors.Is(err, errFileExists) {
			// If we found out that original variant is already exists, so we are trying to add the same file
			if link.Name == "original" {
				isExisting = true
			} else {
				// If we have multiple variants with the same hash, for example "original" and "large",
				// we need to find the previously added file node
				for _, entry := range dirEntries {
					if entry.fileInfo.Hash == added.Hash {
						fileNode = entry.fileNode
						break
					}
				}
			}
		} else if err != nil {
			return nil, err
		}
		dirEntries = append(dirEntries, dirEntry{
			name:     link.Name,
			fileInfo: added,
			fileNode: fileNode,
		})
		reader.Seek(0, 0)
	}

	if isExisting {
		return dirEntries, errFileExists
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
	err = helpers.AddLinkToDirectory(ctx, dagService, outer, fileLinkName, id)
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

func (s *service) isImported(origin model.ObjectOrigin) bool {
	return origin == model.ObjectOrigin_import
}

func (s *service) newExisingImageResult(spaceId string, dirEntries []dirEntry) (*ImageAddResult, error) {
	for _, entry := range dirEntries {
		fileId, keys, err := s.getFileIdAndEncryptionKeysFromInfo(entry.fileInfo)
		if err != nil {
			return nil, err
		}
		return &ImageAddResult{
			IsExisting:     true,
			FileId:         fileId,
			Image:          s.newImage(spaceId, fileId, dirEntries),
			EncryptionKeys: keys,
		}, nil
	}
	return nil, errors.New("image directory is empty")
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

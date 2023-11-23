package fileobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "fileobject"

type Service interface {
	app.Component

	Create(ctx context.Context, spaceId string, req CreateRequest) (id string, object *types.Struct, err error)
	GetFileIdFromObject(ctx context.Context, objectId string) (domain.FullFileId, error)
}

type service struct {
	spaceService  space.Service
	resolver      idresolver.Resolver
	objectCreator objectcreator.Service
	fileService   files.Service
	fileSync      filesync.FileSync
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.spaceService = app.MustComponent[space.Service](a)
	s.resolver = app.MustComponent[idresolver.Resolver](a)
	s.objectCreator = app.MustComponent[objectcreator.Service](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	return nil
}

type CreateRequest struct {
	FileId         domain.FileId
	EncryptionKeys map[string]string
	IsImported     bool
}

func (s *service) Create(ctx context.Context, spaceId string, req CreateRequest) (id string, object *types.Struct, err error) {
	if req.FileId == "" {
		return "", nil, fmt.Errorf("file hash is empty")
	}

	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}

	details, typeKey, err := s.getDetailsForFileOrImage(ctx, domain.FullFileId{
		SpaceId: space.Id(),
		FileId:  req.FileId,
	})
	if err != nil {
		return "", nil, fmt.Errorf("get details for file or image: %w", err)
	}
	details.Fields[bundle.RelationKeyFileId.String()] = pbtypes.String(req.FileId.String())

	createState := state.NewDoc("", nil).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		FileId:         req.FileId,
		EncryptionKeys: req.EncryptionKeys,
	})

	id, object, err = s.objectCreator.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{typeKey}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create object: %w", err)
	}

	err = s.addToSyncQueue(domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}, true, req.IsImported)
	if err != nil {
		return "", nil, fmt.Errorf("add to sync queue: %w", err)
	}
	return id, object, nil
}

func (s *service) getDetailsForFileOrImage(ctx context.Context, id domain.FullFileId) (*types.Struct, domain.TypeKey, error) {
	file, err := s.fileService.FileByHash(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if mill.IsImage(file.Info().Media) {
		image, err := s.fileService.ImageByHash(ctx, id)
		if err != nil {
			return nil, "", err
		}
		details, err := image.Details(ctx)
		if err != nil {
			return nil, "", err
		}
		return details, bundle.TypeKeyImage, nil
	}

	d, typeKey, err := file.Details(ctx)
	if err != nil {
		return nil, "", err
	}
	return d, typeKey, nil
}

func (s *service) addToSyncQueue(id domain.FullFileId, uploadedByUser bool, imported bool) error {
	if err := s.fileSync.AddFile(id.SpaceId, id.FileId, uploadedByUser, imported); err != nil {
		return fmt.Errorf("add file to sync queue: %w", err)
	}
	// TODO Maybe we need a watcher here?
	return nil
}

func (s *service) GetFileIdFromObject(ctx context.Context, objectId string) (domain.FullFileId, error) {
	spaceId, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("resolve spaceId: %w", err)
	}

	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get space: %w", err)
	}

	return s.getFileIdFromObjectInSpace(ctx, space, objectId)
}

func (s *service) getFileIdFromObjectInSpace(ctx context.Context, space smartblock.Space, objectId string) (domain.FullFileId, error) {
	var fileId string
	err := space.Do(objectId, func(sb smartblock.SmartBlock) error {
		fileId = pbtypes.GetString(sb.Details(), bundle.RelationKeyFileId.String())
		if fileId == "" {
			return fmt.Errorf("empty file hash")
		}
		return nil
	})
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get file object: %w", err)
	}

	return domain.FullFileId{
		SpaceId: space.Id(),
		FileId:  domain.FileId(fileId),
	}, nil
}

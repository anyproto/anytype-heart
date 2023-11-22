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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "fileobject"

type Service interface {
	app.Component

	GetFileHashFromObject(ctx context.Context, objectId string) (domain.FullID, error)
	GetFileHashFromObjectInSpace(ctx context.Context, space smartblock.Space, objectId string) (domain.FullID, error)
}

type service struct {
	spaceService  space.Service
	resolver      idresolver.Resolver
	objectCreator objectcreator.Service
	fileService   files.Service
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
	return nil
}

func (s *service) Create(ctx context.Context, space space.Space, fileHash string, encryptionKeys map[string]string) (id string, object *types.Struct, err error) {
	if fileHash == "" {
		return "", nil, fmt.Errorf("file hash is empty")
	}
	details, typeKey, err := s.getDetailsForFileOrImage(ctx, domain.FullID{
		SpaceID:  space.Id(),
		ObjectID: fileHash,
	})
	if err != nil {
		return "", nil, fmt.Errorf("get details for file or image: %w", err)
	}
	details.Fields[bundle.RelationKeyFileHash.String()] = pbtypes.String(fileHash)

	createState := state.NewDoc("", nil).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		Hash:           fileHash,
		EncryptionKeys: encryptionKeys,
	})

	return s.objectCreator.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{typeKey}, createState)
}

func (s *service) getDetailsForFileOrImage(ctx context.Context, id domain.FullID) (*types.Struct, domain.TypeKey, error) {
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

func (s *service) GetFileHashFromObject(ctx context.Context, objectId string) (domain.FullID, error) {
	spaceId, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return domain.FullID{}, fmt.Errorf("resolve spaceId: %w", err)
	}

	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return domain.FullID{}, fmt.Errorf("get space: %w", err)
	}

	return s.GetFileHashFromObjectInSpace(ctx, space, objectId)
}

func (s *service) GetFileHashFromObjectInSpace(ctx context.Context, space smartblock.Space, objectId string) (domain.FullID, error) {
	var fileHash string
	err := space.Do(objectId, func(sb smartblock.SmartBlock) error {
		fileHash = pbtypes.GetString(sb.Details(), bundle.RelationKeyFileHash.String())
		if fileHash == "" {
			return fmt.Errorf("empty file hash")
		}
		return nil
	})
	if err != nil {
		return domain.FullID{}, fmt.Errorf("get file object: %w", err)
	}

	return domain.FullID{
		SpaceID:  space.Id(),
		ObjectID: fileHash,
	}, nil
}

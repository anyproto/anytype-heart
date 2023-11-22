package fileobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	spaceService space.Service
	resolver     idresolver.Resolver
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
	return nil
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

package system_object

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "relation"

var (
	log = logging.Logger("anytype-relations")
)

func New() Service {
	return new(service)
}

type Service interface {
	GetTypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (id string, err error)
	GetObjectIdByUniqueKey(ctx context.Context, spaceId string, key domain.UniqueKey) (id string, err error)

	app.Component
}

type service struct {
	objectStore  objectstore.ObjectStore
	spaceService space.Service
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.spaceService = app.MustComponent[space.Service](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetTypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return addr.BundledObjectTypeURLPrefix + key.String(), nil
	}

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}
	return spc.DeriveObjectID(ctx, uk)
}

func (s *service) GetRelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return addr.BundledRelationURLPrefix + key.String(), nil
	}
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}
	return spc.DeriveObjectID(ctx, uk)
}

// GetObjectIdByUniqueKey returns object id by uniqueKey and spaceId
// context is used in case of space cache miss(shouldn't be a case for a valid spaceId)
// cheap to use in terms of performance (about 500ms per 10000 derivations)
func (s *service) GetObjectIdByUniqueKey(ctx context.Context, spaceId string, key domain.UniqueKey) (id string, err error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space: %w", err)
	}
	return spc.DeriveObjectID(ctx, key)
}

package system_object

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "relation"

var (
	ErrNotFound = errors.New("relation not found")
	log         = logging.Logger("anytype-relations")
)

func New() Service {
	return new(service)
}

type Service interface {
	FetchRelationByKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error)
	FetchRelationByKey(spaceId string, key string) (relation *relationutils.Relation, err error)
	ListAllRelations(spaceId string) (relations relationutils.Relations, err error)
	GetRelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (id string, err error)

	FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error)

	GetObjectType(url string) (*model.ObjectType, error)
	HasObjectType(id string) (bool, error)
	GetObjectTypes(urls []string) (ots []*model.ObjectType, err error)

	GetRelationByID(id string) (relation *model.Relation, err error)
	GetRelationByKey(key string) (relation *model.Relation, err error)

	GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*model.ObjectDetails, error)

	app.Component
}

type deriver interface {
	DeriveObjectID(ctx context.Context, spaceID string, uniqueKey domain.UniqueKey) (id string, err error)
}

type service struct {
	objectStore objectstore.ObjectStore
	deriver     deriver
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.deriver = app.MustComponent[deriver](a)
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

	return s.deriver.DeriveObjectID(ctx, spaceId, uk)
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

	return s.deriver.DeriveObjectID(ctx, spaceId, uk)
}

func (s *service) FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	keys := make([]string, 0, len(links))
	for _, l := range links {
		keys = append(keys, l.Key)
	}
	return s.FetchRelationByKeys(spaceId, keys...)
}

func (s *service) FetchRelationByKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error) {
	uks := make([]string, 0, len(keys))

	for _, key := range keys {
		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
		if err != nil {
			return nil, err
		}
		uks = append(uks, uk.Marshal())
	}
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(uks),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return
	}

	for _, rec := range records {
		relations = append(relations, relationutils.RelationFromStruct(rec.Details))
	}
	return
}

func (s *service) ListAllRelations(spaceId string) (relations relationutils.Relations, err error) {
	filters := []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyLayout.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Float64(float64(model.ObjectType_relation)),
		},
	}
	filters = append(filters, &model.BlockContentDataviewFilter{
		RelationKey: bundle.RelationKeySpaceId.String(),
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(spaceId),
	})

	relations2, _, err := s.objectStore.Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return
	}

	for _, rec := range relations2 {
		relations = append(relations, relationutils.RelationFromStruct(rec.Details))
	}
	return
}

type fetchOptions struct {
}

type FetchOption func(options *fetchOptions)

func (s *service) FetchRelationByKey(spaceID string, key string) (relation *relationutils.Relation, err error) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		return nil, err
	}
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(uk.Marshal()),
			},
		},
	}
	q.Filters = append(q.Filters, &model.BlockContentDataviewFilter{
		Condition:   model.BlockContentDataviewFilter_Equal,
		RelationKey: bundle.RelationKeySpaceId.String(),
		Value:       pbtypes.String(spaceID),
	})

	records, _, err := s.objectStore.Query(q)
	if err != nil {
		return
	}
	for _, rec := range records {
		return relationutils.RelationFromStruct(rec.Details), nil
	}
	return nil, ErrNotFound
}

func (s *service) GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*model.ObjectDetails, error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(uniqueKey.Marshal()),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceId),
			},
		},
		Limit: 2,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, objectstore.ErrObjectNotFound
	}

	if len(records) > 1 {
		// should never happen
		return nil, fmt.Errorf("multiple objects with unique key %s", uniqueKey)
	}

	return &model.ObjectDetails{Details: records[0].Details}, nil
}

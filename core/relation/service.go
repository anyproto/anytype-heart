package relation

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
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
	GetRelationIdByKey(ctx context.Context, spaceId string, key bundle.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, spaceId string, key bundle.TypeKey) (id string, err error)

	FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	app.Component
}

type service struct {
	objectStore objectstore.ObjectStore
	core        core.Service
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.core = app.MustComponent[core.Service](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetTypeIdByKey(ctx context.Context, spaceId string, key bundle.TypeKey) (id string, err error) {
	uk, err := uniquekey.New(model.SmartBlockType_STType, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return addr.BundledObjectTypeURLPrefix + key.String(), nil
	}

	return s.core.DeriveObjectId(ctx, spaceId, uk)
}

func (s *service) GetRelationIdByKey(ctx context.Context, spaceId string, key bundle.RelationKey) (id string, err error) {
	uk, err := uniquekey.New(model.SmartBlockType_STRelation, key.String())
	if err != nil {
		return "", err
	}

	// todo: it should be done via a virtual space
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return addr.BundledRelationURLPrefix + key.String(), nil
	}

	return s.core.DeriveObjectId(ctx, spaceId, uk)
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
		uk, err := uniquekey.New(model.SmartBlockType_STRelation, key)
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
		RelationKey: bundle.RelationKeyWorkspaceId.String(),
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
	uk, err := uniquekey.New(model.SmartBlockType_STRelation, key)
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

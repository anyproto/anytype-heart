package export

import (
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/converter/md"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// lazyObjectResolver implements ObjectResolver using objectStore
type lazyObjectResolver struct {
	objectStore    objectstore.ObjectStore
	spaceId        string
	relationsCache map[string]*domain.Details
	typesCache     map[string]*domain.Details
}

func newLazyObjectResolver(objectStore objectstore.ObjectStore, spaceId string) md.ObjectResolver {
	return &lazyObjectResolver{
		objectStore:    objectStore,
		spaceId:        spaceId,
		relationsCache: make(map[string]*domain.Details),
		typesCache:     make(map[string]*domain.Details),
	}
}

func (r *lazyObjectResolver) ResolveRelation(relationId string) (*domain.Details, error) {
	// Check cache first
	if details, exists := r.relationsCache[relationId]; exists {
		return details, nil
	}

	// Query from objectStore
	records, err := r.objectStore.SpaceIndex(r.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationId),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(records) > 0 {
		details := records[0].Details
		r.relationsCache[relationId] = details
		return details, nil
	}

	return nil, nil
}

func (r *lazyObjectResolver) GetRelationByKey(relationKey string) (*domain.Details, error) {
	// Check cache first
	for _, details := range r.relationsCache {
		if details.GetString(bundle.RelationKeyRelationKey) == relationKey {
			return details, nil
		}
	}

	// Query from objectStore
	records, err := r.objectStore.SpaceIndex(r.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(records) > 0 {
		details := records[0].Details
		relationId := details.GetString(bundle.RelationKeyId)
		r.relationsCache[relationId] = details
		return details, nil
	}

	return nil, nil
}

func (r *lazyObjectResolver) ResolveType(typeId string) (*domain.Details, error) {
	// Check cache first
	if details, exists := r.typesCache[typeId]; exists {
		return details, nil
	}

	// Query from objectStore
	records, err := r.objectStore.SpaceIndex(r.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(typeId),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(records) > 0 {
		details := records[0].Details
		r.typesCache[typeId] = details
		return details, nil
	}

	return nil, nil
}

func (r *lazyObjectResolver) ResolveRelationOptions(relationKey string) ([]*domain.Details, error) {
	// Query from objectStore
	records, err := r.objectStore.SpaceIndex(r.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relationOption)),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	options := make([]*domain.Details, 0, len(records))
	for _, record := range records {
		options = append(options, record.Details)
	}

	return options, nil
}

func (r *lazyObjectResolver) ResolveObject(objectId string) (*domain.Details, bool) {
	records, err := r.objectStore.SpaceIndex(r.spaceId).QueryByIds([]string{objectId})
	if err != nil {
		log.Error("failed to resolve object", zap.String("objectId", objectId), zap.Error(err))
		return nil, false
	}
	if len(records) == 0 {
		log.Warn("export lazyObjectResolver: object not found", zap.String("objectId", objectId))
		return nil, false
	}
	details := records[0].Details
	return details, true
}

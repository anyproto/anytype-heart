package objectstore

import (
	"fmt"

	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) GetRelationLink(spaceID string, key string) (*model.RelationLink, error) {
	bundledRel, err := bundle.GetRelation(domain.RelationKey(key))
	if err == nil {
		return &model.RelationLink{
			Key:    bundledRel.Key,
			Format: bundledRel.Format,
		}, nil
	}

	rel, err := s.FetchRelationByKey(spaceID, key)
	if err != nil {
		return nil, fmt.Errorf("get relation: %w", err)
	}
	return rel.RelationLink(), nil
}

func (s *dsObjectStore) FetchRelationByKey(spaceID string, key string) (relation *relationutils.Relation, err error) {
	bundledRel, err := bundle.GetRelation(domain.RelationKey(key))
	if err == nil {
		return &relationutils.Relation{Relation: bundledRel}, nil
	}

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

	records, _, err := s.Query(q)
	if err != nil {
		return
	}
	for _, rec := range records {
		return relationutils.RelationFromStruct(rec.Details), nil
	}
	return nil, ErrObjectNotFound
}

func (s *dsObjectStore) FetchRelationByKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error) {
	uks := make([]string, 0, len(keys))

	for _, key := range keys {
		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
		if err != nil {
			return nil, err
		}
		uks = append(uks, uk.Marshal())
	}
	records, _, err := s.Query(database.Query{
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

func (s *dsObjectStore) FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	keys := make([]string, 0, len(links))
	for _, l := range links {
		keys = append(keys, l.Key)
	}
	return s.FetchRelationByKeys(spaceId, keys...)
}

func (s *dsObjectStore) GetRelationByID(id string) (*model.Relation, error) {
	det, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if pbtypes.GetString(det.GetDetails(), bundle.RelationKeyRelationKey.String()) == "" {
		return nil, fmt.Errorf("object %s is not a relation", id)
	}

	rel := relationutils.RelationFromStruct(det.GetDetails())
	return rel.Relation, nil
}

func (s *dsObjectStore) ListAllRelations(spaceId string) (relations relationutils.Relations, err error) {
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

	relations2, _, err := s.Query(database.Query{
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

func (s *dsObjectStore) GetRelationByKey(key string) (*model.Relation, error) {
	// todo: should pass workspace
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout.String(),
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
	}

	records, _, err := s.Query(q)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ds.ErrNotFound
	}

	rel := relationutils.RelationFromStruct(records[0].Details)

	return rel.Relation, nil
}

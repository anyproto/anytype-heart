package spaceindex

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

func (s *dsObjectStore) GetRelationLink(key string) (*model.RelationLink, error) {
	bundledRel, err := bundle.GetRelation(domain.RelationKey(key))
	if err == nil {
		return &model.RelationLink{
			Key:    bundledRel.Key,
			Format: bundledRel.Format,
		}, nil
	}

	rel, err := s.FetchRelationByKey(key)
	if err != nil {
		return nil, fmt.Errorf("get relation: %w", err)
	}
	return rel.RelationLink(), nil
}

func (s *dsObjectStore) FetchRelationByKey(key string) (relation *relationutils.Relation, err error) {
	bundledRel, err := bundle.GetRelation(domain.RelationKey(key))
	if err == nil {
		return &relationutils.Relation{Relation: bundledRel}, nil
	}

	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		return nil, err
	}
	records, err := s.QueryRaw(&database.Filters{FilterObj: database.FilterEq{
		Key:   bundle.RelationKeyUniqueKey,
		Cond:  model.BlockContentDataviewFilter_Equal,
		Value: domain.String(uk.Marshal()),
	}}, 1, 0)
	if err != nil {
		return
	}
	for _, rec := range records {
		return relationutils.RelationFromDetails(rec.Details), nil
	}
	return nil, ErrObjectNotFound
}

func (s *dsObjectStore) FetchRelationByKeys(keys ...domain.RelationKey) (relations relationutils.Relations, err error) {
	uks := make([]string, 0, len(keys))
	for _, key := range keys {
		// we should be able to get system relations even when not indexed
		bundledRel, err := bundle.GetRelation(key)
		if err == nil {
			relations = append(relations, &relationutils.Relation{Relation: bundledRel})
			continue
		}

		uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, string(key))
		if err != nil {
			return nil, err
		}
		uks = append(uks, uk.Marshal())
	}
	if len(uks) == 0 {
		return
	}
	records, err := s.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyUniqueKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(uks),
			},
		},
	})
	if err != nil {
		return
	}

	for _, rec := range records {
		relations = append(relations, relationutils.RelationFromDetails(rec.Details))
	}
	return
}

func (s *dsObjectStore) FetchRelationByLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	keys := make([]domain.RelationKey, 0, len(links))
	for _, l := range links {
		keys = append(keys, domain.RelationKey(l.Key))
	}
	return s.FetchRelationByKeys(keys...)
}

func (s *dsObjectStore) GetRelationById(id string) (*model.Relation, error) {
	det, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if _, ok := det.TryString(bundle.RelationKeyRelationKey); !ok {
		return nil, fmt.Errorf("object %s is not a relation", id)
	}

	rel := relationutils.RelationFromDetails(det)
	return rel.Relation, nil
}

func (s *dsObjectStore) ListAllRelations() (relations relationutils.Relations, err error) {
	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(model.ObjectType_relation),
		},
	}

	records, err := s.Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return
	}

	allKeys := make(map[domain.RelationKey]struct{}, len(records))
	for _, rec := range records {
		relationModel := relationutils.RelationFromDetails(rec.Details)
		relations = append(relations, relationModel)
		allKeys[domain.RelationKey(relationModel.Key)] = struct{}{}
	}

	for _, key := range bundle.SystemRelations {
		if _, found := allKeys[key]; found {
			continue
		}
		// we should include system relations if they were not indexed
		relations = append(relations, &relationutils.Relation{Relation: bundle.MustGetRelation(key)})
	}
	return
}

func (s *dsObjectStore) GetRelationByKey(key string) (*model.Relation, error) {
	q := database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(key),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
	}

	records, err := s.Query(q)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ds.ErrNotFound
	}

	rel := relationutils.RelationFromDetails(records[0].Details)

	return rel.Relation, nil
}

func (s *dsObjectStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	format, err := bundle.GetRelationFormat(key)
	if err == nil {
		return format, nil
	}
	q := database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(key.String()),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
	}

	records, err := s.Query(q)
	if err != nil {
		return 0, err
	}

	if len(records) == 0 {
		return 0, ds.ErrNotFound
	}

	details := records[0].Details
	return model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)), nil
}

// ListRelationOptions returns options for specific relation
func (s *dsObjectStore) ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error) {
	filters := []database.FilterRequest{
		{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeyRelationKey,
			Value:       domain.String(relationKey),
		},
		{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeyResolvedLayout,
			Value:       domain.Int64(model.ObjectType_relationOption),
		},
	}
	records, err := s.Query(database.Query{
		Filters: filters,
	})

	for _, rec := range records {
		options = append(options, relationutils.OptionFromDetails(rec.Details).RelationOption)
	}
	return
}

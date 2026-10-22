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
	format, err := bundle.GetRelationFormat(domain.RelationKey(key))
	if err == nil {
		return &model.RelationLink{
			Key:    key,
			Format: format,
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

// GetRelationByKey resolves a relation by its key. It goes through QueryRaw,
// not Query, so that a property the user removed is still returned: Query
// injects `isDeleted != true` and `isArchived != true` for any caller that does
// not mention those keys (database.addDefaultFilters), while the by-id arm
// (GetRelationById) reads details directly and never applies them. That
// asymmetry made one relation resolvable by id and unresolvable by key, so an
// export could name a property in a type document and fail to define it in the
// dictionary. Every caller here — the AnyBlock and markdown exporters, and the
// sub-object link migration — wants the definition of a property whose values
// still sit on objects.
//
// A LIVE row still wins. Dropping the default filters means a removed row and a
// live one can both match, and returning the removed one would be a regression,
// so the scan prefers a row marked neither deleted nor archived and falls back
// to a removed one only when that is all there is. This is also why the query
// takes no limit: the live row is not necessarily first.
//
// A tombstoned relation is out of reach either way: DeleteObject strips the row
// to id, isDeleted, preservedOnDelete and a deletedSnapshot, and neither
// relationKey nor relationFormat survives that (see SnapshotOnDelete), so
// nothing is left to match a key against or to build a definition from.
func (s *dsObjectStore) GetRelationByKey(key string) (*model.Relation, error) {
	records, err := s.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(key),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(int64(model.ObjectType_relation)),
		},
	}}, 0, 0)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ds.ErrNotFound
	}

	chosen := records[0].Details
	for _, rec := range records {
		if !rec.Details.GetBool(bundle.RelationKeyIsDeleted) && !rec.Details.GetBool(bundle.RelationKeyIsArchived) {
			chosen = rec.Details
			break
		}
	}

	rel := relationutils.RelationFromDetails(chosen)

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

// ListRelationOptions returns options for specific relation.
//
// The explicit isUninstalled exclusion pins the corpse policy for options
// the way livePropertyFilters does for relations (API v2 §8.41): a UI-deleted
// option persists as {isUninstalled, isDeleted} plus full details, and until
// this filter the injected `isDeleted != true` default was the SOLE thing
// hiding it — any snapshot-shaped row carrying only isUninstalled (exports;
// isDeleted is a local relation re-derived on load), or any future caller
// suppressing the defaults, would have served deleted options as live.
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
		{
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			RelationKey: bundle.RelationKeyIsUninstalled,
			Value:       domain.Bool(true),
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

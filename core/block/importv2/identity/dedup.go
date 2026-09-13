package identity

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// matchExisting applies the minted-class dedup order: oldAnytypeID (also
// checked against uniqueKey — old ids of derived objects equal their unique
// key), then sourceFilePath when sourcePathAlways or update-existing is on.
// Empty keys are skipped (v1 queried with empty strings; that was a latent
// mismatch source).
//
// sourcePathAlways carries v1's rule that derived objects correlate on their
// source id whatever the flag says (v1 hardcoded true for them in
// objectid/derivedobject.go): the flag decides whether a re-import overwrites
// user-facing PAGES, while a type or relation that is demonstrably the same
// one must converge either way, or every re-import mints a parallel set.
func (s *Service) matchExisting(c importv2.IdentityClaim, sourcePathAlways bool) (string, error) {
	if c.OldAnytypeID != "" {
		id, err := s.queryFirstId(database.Query{Filters: []database.FilterRequest{
			eq(bundle.RelationKeyOldAnytypeID, domain.String(c.OldAnytypeID)),
		}})
		if err != nil {
			return "", fmt.Errorf("query by old anytype id: %w", err)
		}
		if id == "" {
			id, err = s.queryFirstId(database.Query{Filters: []database.FilterRequest{
				eq(bundle.RelationKeyUniqueKey, domain.String(c.OldAnytypeID)),
			}})
			if err != nil {
				return "", fmt.Errorf("query by unique key: %w", err)
			}
		}
		if id != "" {
			return id, nil
		}
	}
	if (sourcePathAlways || s.updateExisting) && c.SourceFilePath != "" {
		id, err := s.queryFirstId(database.Query{Filters: []database.FilterRequest{
			eq(bundle.RelationKeySourceFilePath, domain.String(c.SourceFilePath)),
		}})
		if err != nil {
			return "", fmt.Errorf("query by source file path: %w", err)
		}
		if id != "" {
			return id, nil
		}
	}
	return "", nil
}

// matchExistingDerived applies the derivable-class dedup order: the shared
// oldAnytypeID/sourceFilePath match (sourceFilePath always enabled for
// derived objects, as in v1), then the type-specific fallbacks.
func (s *Service) matchExistingDerived(o *importv2.Object) (string, error) {
	details := o.Payload.Details
	id, err := s.matchExisting(importv2.IdentityClaim{
		SourceKey:      o.SourceKey,
		SbType:         o.SbType,
		OldAnytypeID:   details.GetString(bundle.RelationKeyOldAnytypeID),
		SourceFilePath: details.GetString(bundle.RelationKeySourceFilePath),
	}, true)
	if err != nil || id != "" {
		return id, err
	}
	switch o.SbType {
	case coresb.SmartBlockTypeRelationOption:
		return s.matchRelationOption(o)
	case coresb.SmartBlockTypeRelation:
		return s.matchRelation(o)
	case coresb.SmartBlockTypeObjectType:
		return s.matchObjectType(o)
	}
	return "", nil
}

func (s *Service) matchRelationOption(o *importv2.Object) (string, error) {
	name := o.Payload.Details.GetString(bundle.RelationKeyName)
	key := o.Payload.Details.GetString(bundle.RelationKeyRelationKey)
	if name == "" || key == "" {
		return "", nil
	}
	id, err := s.queryFirstId(database.Query{Filters: []database.FilterRequest{
		eq(bundle.RelationKeyName, domain.String(name)),
		eq(bundle.RelationKeyRelationKey, domain.String(key)),
		eq(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_relationOption))),
	}})
	if err != nil {
		return "", fmt.Errorf("query relation option: %w", err)
	}
	return id, nil
}

// matchRelation matches on the relation key first and only then on the name.
// The key is an identity — for Notion it is derived from the property id, so
// it names one property of one database — while the name is a label many
// properties share. Querying both at once as an OR let a same-named relation
// from another database win the single returned row, which would merge two
// databases' vocabularies back into one option pool on every re-import, and
// non-deterministically at that.
func (s *Service) matchRelation(o *importv2.Object) (string, error) {
	format := o.Payload.Details.Get(bundle.RelationKeyRelationFormat)
	for _, identifying := range []domain.RelationKey{bundle.RelationKeyRelationKey, bundle.RelationKeyName} {
		value := o.Payload.Details.Get(identifying)
		if !value.Ok() {
			continue
		}
		records, err := s.store.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
			database.FilterEq{
				Key:   bundle.RelationKeyRelationFormat,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: format,
			},
			database.FilterEq{
				Key:   bundle.RelationKeyResolvedLayout,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Int64(int64(model.ObjectType_relation)),
			},
			database.FilterEq{
				Key:   identifying,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: value,
			},
		}}, 1, 0)
		if err != nil {
			return "", fmt.Errorf("query relation by %s: %w", identifying, err)
		}
		if len(records) > 0 {
			return records[0].Details.GetString(bundle.RelationKeyId), nil
		}
	}
	return "", nil
}

// matchObjectType matches on the unique key first, then on the name — but a
// name match only claims a type a previous IMPORT created.
//
// Names carry no identity, and under the always-mint plan they come from an
// untrusted model. Claiming a same-named type the user authored would hand
// their data model to the importer: persist already refuses to rewrite such a
// type (persist.go:247-256), but the incoming objects would still be typed
// onto it, and a database whose collection was replaced by its type would lose
// its membership entirely. Reusing a type an import made is the re-import
// case and stays.
func (s *Service) matchObjectType(o *importv2.Object) (string, error) {
	if uniqueKey := o.Payload.Details.Get(bundle.RelationKeyUniqueKey); uniqueKey.Ok() {
		records, err := s.store.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
			typeLayoutFilter(),
			database.FilterEq{
				Key:   bundle.RelationKeyUniqueKey,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: uniqueKey,
			},
		}}, 1, 0)
		if err != nil {
			return "", fmt.Errorf("query object type by unique key: %w", err)
		}
		if len(records) > 0 {
			return records[0].Details.GetString(bundle.RelationKeyId), nil
		}
	}
	if o.Payload.Details.GetString(bundle.RelationKeyName) == "" {
		return "", nil
	}
	records, err := s.store.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		typeLayoutFilter(),
		database.FilterEq{
			Key:   bundle.RelationKeyName,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: o.Payload.Details.Get(bundle.RelationKeyName),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyOrigin,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(int64(model.ObjectOrigin_import)),
		},
	}}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("query object type by name: %w", err)
	}
	if len(records) > 0 {
		return records[0].Details.GetString(bundle.RelationKeyId), nil
	}
	return "", nil
}

func typeLayoutFilter() database.FilterEq {
	return database.FilterEq{
		Key:   bundle.RelationKeyResolvedLayout,
		Cond:  model.BlockContentDataviewFilter_Equal,
		Value: domain.Int64(int64(model.ObjectType_objectType)),
	}
}

func (s *Service) isDeleted(uniqueKey string) bool {
	id, err := s.queryFirstId(database.Query{Filters: []database.FilterRequest{
		eq(bundle.RelationKeyUniqueKey, domain.String(uniqueKey)),
		eq(bundle.RelationKeyIsDeleted, domain.Bool(true)),
	}})
	return err == nil && id != ""
}

func (s *Service) internalKeyOf(objectId string) (string, error) {
	records, err := s.store.Query(database.Query{Filters: []database.FilterRequest{
		eq(bundle.RelationKeyId, domain.String(objectId)),
	}})
	if err != nil {
		return "", fmt.Errorf("query object %q: %w", objectId, err)
	}
	if len(records) == 0 {
		return "", nil
	}
	uniqueKey, err := domain.UnmarshalUniqueKey(records[0].Details.GetString(bundle.RelationKeyUniqueKey))
	if err != nil {
		return "", nil // no unique key on the matched object — nothing to adopt
	}
	return uniqueKey.InternalKey(), nil
}

func (s *Service) queryFirstId(q database.Query) (string, error) {
	ids, _, err := s.store.QueryObjectIds(q)
	if err != nil {
		return "", err
	}
	if len(ids) > 0 {
		return ids[0], nil
	}
	return "", nil
}

func eq(key domain.RelationKey, value domain.Value) database.FilterRequest {
	return database.FilterRequest{
		Condition:   model.BlockContentDataviewFilter_Equal,
		RelationKey: key,
		Value:       value,
	}
}

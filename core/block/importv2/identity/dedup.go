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
// key), then sourceFilePath when update-existing is on. Empty keys are
// skipped (v1 queried with empty strings; that was a latent mismatch source).
func (s *Service) matchExisting(c importv2.IdentityClaim) (string, error) {
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
	if s.updateExisting && c.SourceFilePath != "" {
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
	})
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

func (s *Service) matchRelation(o *importv2.Object) (string, error) {
	records, err := s.store.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationFormat,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: o.Payload.Details.Get(bundle.RelationKeyRelationFormat),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(int64(model.ObjectType_relation)),
		},
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyName,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: o.Payload.Details.Get(bundle.RelationKeyName),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyRelationKey,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: o.Payload.Details.Get(bundle.RelationKeyRelationKey),
			},
		},
	}}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("query relation: %w", err)
	}
	if len(records) > 0 {
		return records[0].Details.GetString(bundle.RelationKeyId), nil
	}
	return "", nil
}

func (s *Service) matchObjectType(o *importv2.Object) (string, error) {
	if o.Payload.Details.GetString(bundle.RelationKeyName) == "" {
		return "", nil
	}
	records, err := s.store.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(int64(model.ObjectType_objectType)),
		},
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyName,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: o.Payload.Details.Get(bundle.RelationKeyName),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyUniqueKey,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: o.Payload.Details.Get(bundle.RelationKeyUniqueKey),
			},
		},
	}}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("query object type: %w", err)
	}
	if len(records) > 0 {
		return records[0].Details.GetString(bundle.RelationKeyId), nil
	}
	return "", nil
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

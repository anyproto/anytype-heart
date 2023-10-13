package source

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	dataview2 "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type systemObjectService interface {
	GetTypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (id string, err error)
	GetRelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (id string, err error)
	GetObjectIdByUniqueKey(ctx context.Context, spaceId string, key domain.UniqueKey) (id string, err error)
}

// Migrate old relation (rel-name, etc.) and object type (ot-page, etc.) IDs to new ones (just ordinary object IDs)
// Those old ids are ids of sub-objects, legacy system for storing types and relations inside workspace object
type subObjectsLinksMigration struct {
	profileID           string
	identityObjectID    string
	space       Space
	objectStore objectstore.ObjectStore
}

func newSubObjectsLinksMigration(space Space, identityObjectID string, objectStore objectstore.ObjectStore) *subObjectsLinksMigration {
	return &subObjectsLinksMigration{
		space:       space,
		identityObjectID:    identityObjectID,
		objectStore: objectStore,
	}
}

func (m *subObjectsAndProfileLinksMigration) replaceLinksInDetails(s *state.State) {
	for _, rel := range s.GetRelationLinks() {
		if m.canRelationContainObjectValues(rel.Format) {
			ids := pbtypes.GetStringList(s.Details(), rel.Key)
			changed := false
			for i, oldId := range ids {
				newId := m.migrateId(oldId)
				if oldId != newId {
					ids[i] = newId
					changed = true
				}
			}
			if changed {
				s.SetDetail(rel.Key, pbtypes.StringList(ids))
			}
		}
	}
}

func (m *subObjectsAndProfileLinksMigration) migrate(s *state.State) {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeProfilePage, "")
	if err != nil {
		log.Errorf("migration: failed to create unique key for profile: %s", err)
	} else {
		// this way we will get incorrect profileID for non-personal spaces, but we are not migrating them
		id, err := m.systemObjectService.GetObjectIdByUniqueKey(context.Background(), m.spaceID, uk)
		if err != nil {
			log.Errorf("migration: failed to derive id for profile: %s", err)
		} else {
			m.profileID = id
		}
	}

	m.replaceLinksInDetails(s)

	s.Iterate(func(block simple.Block) bool {
		if block.Model().GetDataview() != nil {
			// Mark block as mutable
			dv := s.Get(block.Model().Id).(dataview2.Block)
			m.migrateFilters(dv)
		}

		if _, ok := block.(simple.ObjectLinkReplacer); ok {
			// Mark block as mutable
			b := s.Get(block.Model().Id)
			replacer := b.(simple.ObjectLinkReplacer)
			replacer.ReplaceLinkIds(m.migrateId)
		}

		return true
	})
}

func (m *subObjectsAndProfileLinksMigration) migrateId(oldId string) (newId string) {
	if m.profileID != "" && m.identityObjectID != "" && oldId == m.profileID {
		return m.identityObjectID
	}
	uniqueKey, valid := subObjectIdToUniqueKey(oldId)
	if !valid {
		return oldId
	}

	newId, err := m.space.DeriveObjectID(context.Background(), uniqueKey)
	if err != nil {
		log.With("uniqueKey", uniqueKey.Marshal()).Errorf("failed to derive id: %s", err)
		return oldId
	}
	return newId
}

// subObjectIdToUniqueKey converts legacy sub-object id to uniqueKey
// if id is not supported subObjectId, it will return nil, false
// suppose to be used only for migration and almost free to use
func subObjectIdToUniqueKey(id string) (uniqueKey domain.UniqueKey, valid bool) {
	// historically, we don't have the prefix for the options,
	// so we need to handled it this ugly way
	if bson.IsObjectIdHex(id) {
		return domain.MustUniqueKey(smartblock.SmartBlockTypeRelationOption, id), true
	}
	uniqueKey, err := domain.UnmarshalUniqueKey(id)
	if err != nil {
		return nil, false
	}
	return uniqueKey, true
}

func (m *subObjectsAndProfileLinksMigration) migrateFilters(dv dataview2.Block) {
	for _, view := range dv.Model().GetDataview().GetViews() {
		for _, filter := range view.GetFilters() {
			err := m.migrateFilter(filter)
			if err != nil {
				log.Errorf("failed to migrate filter %s: %s", filter.Id, err)
			}
		}
	}
}

func (m *subObjectsAndProfileLinksMigration) migrateFilter(filter *model.BlockContentDataviewFilter) error {
	relation, err := m.objectStore.GetRelationByKey(filter.RelationKey)
	if err != nil {
		log.Warnf("migration: failed to get relation by key %s: %s", filter.RelationKey, err)
	}

	// TODO: check this logic
	// here we use objectstore to get relation, but it may be not yet available
	// In case it is missing, lets try to migrate any string/stringlist: it should ignore invalid strings
	if relation == nil || m.canRelationContainObjectValues(relation.Format) {
		switch v := filter.Value.Kind.(type) {
		case *types.Value_StringValue:
			filter.Value = pbtypes.String(m.migrateId(v.StringValue))
		case *types.Value_ListValue:
			newIDs := make([]string, 0, len(v.ListValue.Values))

			for _, oldID := range v.ListValue.Values {
				if id, ok := oldID.Kind.(*types.Value_StringValue); ok {
					newIDs = append(newIDs, m.migrateId(id.StringValue))
				} else {
					return fmt.Errorf("migration: failed to migrate filter: invalid list item value kind %t", oldID.Kind)
				}
			}

			filter.Value = pbtypes.StringList(newIDs)
		}
	}
	return nil
}

// migrateID always returns ID, even if migration failed
func (m *subObjectsAndProfileLinksMigration) migrateID(id string) (string, error) {
	if m.profileID != "" && m.identityObjectID != "" && id == m.profileID {
		return m.identityObjectID, nil
	}

	typeKey, err := bundle.TypeKeyFromUrl(id)
	if err == nil {
		typeID, err := m.space.GetTypeIdByKey(context.Background(), typeKey)
		if err != nil {
			return id, fmt.Errorf("migrate object type id %s: %w", id, err)
		}
		return typeID, nil
	}

	relationKey, err := bundle.RelationKeyFromID(id)
	if err == nil {
		relationID, err := m.space.GetRelationIdByKey(context.Background(), relationKey)
		if err != nil {
			return id, fmt.Errorf("migrate relation id %s: %w", id, err)
		}
		return relationID, nil
	}

	return id, nil
}

func (m *subObjectsAndProfileLinksMigration) canRelationContainObjectValues(format model.RelationFormat) bool {
	switch format {
	case
		model.RelationFormat_status,
		model.RelationFormat_tag,
		model.RelationFormat_object:
		return true
	default:
		return false
	}
}

package source

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	dataview2 "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Migrate old relation (rel-name, etc.) and object type (ot-page, etc.) IDs to new ones (just ordinary object IDs)
// Those old ids are ids of sub-objects, legacy system for storing types and relations inside workspace object
type subObjectsLinksMigration struct {
	spaceID             string
	systemObjectService system_object.Service
}

func newSubObjectsLinksMigration(spaceID string, systemObjectService system_object.Service) *subObjectsLinksMigration {
	return &subObjectsLinksMigration{
		spaceID:             spaceID,
		systemObjectService: systemObjectService,
	}
}

func (m *subObjectsLinksMigration) replaceSubObjectLinksInDetails(s *state.State) {
	for _, rel := range s.GetRelationLinks() {
		if m.canRelationContainObjectValues(rel.Format) {
			ids := pbtypes.GetStringList(s.Details(), rel.Key)
			changed := false
			for i, oldId := range ids {
				newId := m.migrateSubObjectId(oldId)
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

func (m *subObjectsLinksMigration) migrate(s *state.State) {
	m.replaceSubObjectLinksInDetails(s)

	s.Iterate(func(block simple.Block) bool {
		if block.Model().GetDataview() != nil {
			// Mark block as mutable
			dv := s.Get(block.Model().Id).(dataview2.Block)
			m.migrateSources(dv)
			m.migrateFilters(dv)
		}

		if _, ok := block.(simple.ObjectLinkReplacer); ok {
			// Mark block as mutable
			b := s.Get(block.Model().Id)
			replacer := b.(simple.ObjectLinkReplacer)
			replacer.ReplaceLinkIds(m.migrateSubObjectId)
		}

		return true
	})
}

func (m *subObjectsLinksMigration) migrateSubObjectId(oldId string) (newId string) {
	uniqueKey, valid := subObjectIdToUniqueKey(oldId)
	if !valid {
		return oldId
	}

	newId, err := m.systemObjectService.GetObjectIdByUniqueKey(context.Background(), m.spaceID, uniqueKey)
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

func (m *subObjectsLinksMigration) migrateFilters(dv dataview2.Block) {
	for _, view := range dv.Model().GetDataview().GetViews() {
		for _, filter := range view.GetFilters() {
			err := m.migrateFilter(filter)
			if err != nil {
				log.Errorf("failed to migrate filter %s: %s", filter.Id, err)
			}
		}
	}
}

func (m *subObjectsLinksMigration) migrateFilter(filter *model.BlockContentDataviewFilter) error {
	relation, err := m.systemObjectService.GetRelationByKey(filter.RelationKey)
	if err != nil {
		return fmt.Errorf("failed to get relation by key %s: %w", filter.RelationKey, err)
	}

	if m.canRelationContainObjectValues(relation.Format) {
		if oldID := filter.Value.GetStringValue(); oldID != "" {
			newID, err := m.migrateID(oldID)
			if err != nil {
				log.Errorf("subObjectsLinksMigration: failed to migrate filter %s with single value %s: %s", filter.Id, oldID, err)
			}

			filter.Value = pbtypes.String(newID)
		}

		if oldIDs := pbtypes.GetStringListValue(filter.Value); len(oldIDs) > 0 {
			newIDs := make([]string, 0, len(oldIDs))
			for _, oldID := range oldIDs {
				newID, err := m.migrateID(oldID)
				if err != nil {
					log.Errorf("subObjectsLinksMigration: failed to migrate filter %s with value list: id %s: %s", filter.Id, oldID, err)
				}
				newIDs = append(newIDs, newID)
			}
			filter.Value = pbtypes.StringList(newIDs)
		}
	}
	return nil
}

func (m *subObjectsLinksMigration) migrateSources(dv dataview2.Block) {
	newSources := make([]string, 0, len(dv.GetSource()))
	for _, src := range dv.GetSource() {
		newID, err := m.migrateID(src)
		if err != nil {
			log.Errorf("subObjectsLinksMigration: failed to migrate source %s: %s", src, err)
		}
		newSources = append(newSources, newID)
	}
	dv.SetSource(newSources)
}

// migrateID always returns ID, even if migration failed
func (m *subObjectsLinksMigration) migrateID(id string) (string, error) {
	typeKey, err := bundle.TypeKeyFromUrl(id)
	if err == nil {
		typeID, err := m.systemObjectService.GetTypeIdByKey(context.Background(), m.spaceID, typeKey)
		if err != nil {
			return id, fmt.Errorf("migrate object type id %s: %w", id, err)
		}
		return typeID, nil
	}

	relationKey, err := bundle.RelationKeyFromID(id)
	if err == nil {
		relationID, err := m.systemObjectService.GetRelationIdByKey(context.Background(), m.spaceID, relationKey)
		if err != nil {
			return id, fmt.Errorf("migrate relation id %s: %w", id, err)
		}
		return relationID, nil
	}

	return id, nil
}

func (m *subObjectsLinksMigration) canRelationContainObjectValues(format model.RelationFormat) bool {
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

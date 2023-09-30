package editor

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	dataview2 "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/core/domain"
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

// TODO Refactor
func (m *subObjectsLinksMigration) replaceSubObjectLinksInDetails(s *state.State) {
	for _, rel := range s.GetRelationLinks() {
		if rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_tag || rel.Format == model.RelationFormat_status {
			vals := pbtypes.GetStringList(s.Details(), rel.Key)
			changed := false
			for i := range vals {
				newId, replaced := m.migrateSubObjectId(vals[i])
				if !replaced {
					continue
				}
				vals[i] = newId
				changed = true
			}
			if changed {
				s.SetDetail(rel.Key, pbtypes.StringList(vals))
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

		if v, ok := block.(simple.ObjectLinkReplacer); ok {
			// TODO Analyze this method (ReplaceSmartIds)
			// TODO Looks like we should just map here: oldId OR newId
			v.ReplaceSmartIds(m.migrateSubObjectId)
		}

		return true
	})
}

func (m *subObjectsLinksMigration) migrateSubObjectId(id string) (newID string, migrated bool) {
	// this should be replaced by the persisted state migration
	// TODO Smells like SubObjectIdToUniqueKey should be here in-place, not in domain package!
	uk, valid := domain.SubObjectIdToUniqueKey(id)
	if !valid {
		return "", false
	}

	newID, err := m.systemObjectService.GetObjectIdByUniqueKey(context.Background(), m.spaceID, uk)
	if err != nil {
		log.With("uk", uk.Marshal()).Errorf("failed to derive id: %s", err.Error())
		return "", false
	}
	return newID, true
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

	if m.canRelationContainObjectValues(relation) {
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

func (m *subObjectsLinksMigration) canRelationContainObjectValues(relation *model.Relation) bool {
	switch relation.Format {
	case
		model.RelationFormat_status,
		model.RelationFormat_tag,
		model.RelationFormat_object,
		model.RelationFormat_relations:
		return true
	default:
		return false
	}
}

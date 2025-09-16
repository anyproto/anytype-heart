package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	dataview2 "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-mw-source-migration")

// Migrate old relation (rel-name, etc.) and object type (ot-page, etc.) IDs to new ones (just ordinary object IDs)
// Those old ids are ids of sub-objects, legacy system for storing types and relations inside workspace object
type subObjectsAndProfileLinksMigration struct {
	profileID        string
	identityObjectID string
	sbType           smartblock.SmartBlockType
	space            Space
	objectStore      spaceindex.Store
	formatFetcher    relationutils.RelationFormatFetcher
}

func NewSubObjectsAndProfileLinksMigration(
	sbType smartblock.SmartBlockType,
	space Space,
	identityObjectID string,
	objectStore spaceindex.Store,
	formatFetcher relationutils.RelationFormatFetcher,
) *subObjectsAndProfileLinksMigration {
	return &subObjectsAndProfileLinksMigration{
		space:            space,
		identityObjectID: identityObjectID,
		sbType:           sbType,
		objectStore:      objectStore,
		formatFetcher:    formatFetcher,
	}
}

func (m *subObjectsAndProfileLinksMigration) replaceLinksInDetails(s *state.State) {
	for _, key := range s.AllRelationKeys() {
		if key == bundle.RelationKeyFeaturedRelations {
			continue
		}
		if key == bundle.RelationKeySourceObject {
			// migrate broken sourceObject after v0.29.11
			// todo: remove this
			if s.UniqueKeyInternal() == "" {
				continue
			}

			internalKey := s.UniqueKeyInternal()
			switch m.sbType {
			case smartblock.SmartBlockTypeRelation:
				if bundle.HasRelation(domain.RelationKey(internalKey)) {
					s.SetDetail(bundle.RelationKeySourceObject, domain.String(domain.RelationKey(internalKey).BundledURL()))
				}
			case smartblock.SmartBlockTypeObjectType:
				if bundle.HasObjectTypeByKey(domain.TypeKey(internalKey)) {
					s.SetDetail(bundle.RelationKeySourceObject, domain.String(domain.TypeKey(internalKey).BundledURL()))
				}

			}

			continue
		}

		format, err := m.formatFetcher.GetRelationFormatByKey(m.space.Id(), key)
		if err != nil {
			// let's fall back to object relation format, so we don't miss all object ids in details
			format = model.RelationFormat_object
		}

		if !m.canRelationContainObjectValues(format) {
			continue
		}

		rawValue := s.Details().Get(key)
		if oldId := rawValue.String(); oldId != "" {
			newId := m.migrateId(oldId)
			if oldId != newId {
				s.SetDetail(key, domain.String(newId))
			}
			continue
		}

		ids := rawValue.StringList()
		if len(ids) == 0 {
			continue
		}

		changed := false
		for i, oldId := range ids {
			newId := m.migrateId(oldId)
			if oldId != newId {
				ids[i] = newId
				changed = true
			}
		}
		if changed {
			s.SetDetail(key, domain.StringList(ids))
		}
	}
}

// Migrate works only in personal space
func (m *subObjectsAndProfileLinksMigration) Migrate(s *state.State) {
	if !m.space.IsPersonal() {
		return
	}

	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeProfilePage, "")
	if err != nil {
		log.Errorf("migration: failed to create unique key for profile: %s", err)
	} else {
		// this way we will get incorrect profileID for non-personal spaces, but we are not migrating them
		id, err := m.space.DeriveObjectID(context.Background(), uk)
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
	if m.profileID != "" && m.identityObjectID != "" {
		// we substitute all links to profile object with space member object
		if oldId == m.profileID ||
			strings.HasPrefix(oldId, "_id_") { // we don't need to check the exact accountID here, because we only have links to our own identity
			return m.identityObjectID
		}
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
	// special case: we don't support bundled relations/types in uniqueKeys (GO-2394). So in case we got it, we need to replace the prefix
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		id = addr.ObjectTypeKeyToIdPrefix + strings.TrimPrefix(id, addr.BundledObjectTypeURLPrefix)
	} else if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		id = addr.RelationKeyToIdPrefix + strings.TrimPrefix(id, addr.BundledRelationURLPrefix)
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
	if filter == nil {
		return nil
	}
	if filter.Value == nil || filter.Value.Kind == nil {
		log.With("relationKey", filter.RelationKey).Warnf("empty filter value")
		return nil
	}
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

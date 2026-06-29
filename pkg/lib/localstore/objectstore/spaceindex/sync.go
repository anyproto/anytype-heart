package spaceindex

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// syncDetailRelations are the relations maintained by helper.InjectsSyncDetails.
// Keep in sync with that function.
var syncDetailRelations = []domain.RelationKey{
	bundle.RelationKeySyncStatus,
	bundle.RelationKeySyncDate,
	bundle.RelationKeySyncError,
}

// nonSyncableLayouts are layouts of objects that are indexed with details but
// are not syncable as objects (derived/system/virtual), so they must not get
// sync details. The last few are not indexed with details at all
// (smartblock.Indexable -> false) and are listed only to keep the intent
// explicit. Everything not listed here is treated as a real, syncable object.
var nonSyncableLayouts = []model.ObjectTypeLayout{
	model.ObjectType_dashboard,           // Home
	model.ObjectType_space,               // space/workspace object
	model.ObjectType_relationOptionsList, // not a real object
	model.ObjectType_spaceView,
	model.ObjectType_participant, // ACL-derived
	model.ObjectType_date,        // synthetic, not stored
	model.ObjectType_notification,
	model.ObjectType_missingObject,
	model.ObjectType_devices,
}

func (s *dsObjectStore) ListIdsWithoutSyncDetails(ctx context.Context) ([]string, error) {
	// An object needs sync details when at least one of the relations is
	// absent. Presence (not value) must be used: SyncStatusSynced and
	// SyncErrorNull are both the zero enum value, so a value/Empty filter
	// would re-select every already-synced object on each start.
	missing := make(query.Or, 0, len(syncDetailRelations))
	for _, key := range syncDetailRelations {
		missing = append(missing, query.Key{
			Path:   []string{string(key)},
			Filter: query.Not{Filter: query.Exists{}},
		})
	}

	// Exclude non-syncable system/derived layouts. Objects without a
	// resolvedLayout are kept: In.Ok returns false for an absent value, so the
	// Not is true and real objects are never dropped.
	arena := &anyenc.Arena{}
	excludedLayouts := make([]*anyenc.Value, 0, len(nonSyncableLayouts))
	for _, layout := range nonSyncableLayouts {
		excludedLayouts = append(excludedLayouts, domain.Int64(int64(layout)).ToAnyEnc(arena))
	}
	notSystem := query.Not{Filter: query.Key{
		Path:   []string{string(bundle.RelationKeyResolvedLayout)},
		Filter: query.NewInValue(excludedLayouts...),
	}}

	filter := query.And{missing, notSystem}

	iter, err := s.objects.Find(filter).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find objects without sync details: %w", err)
	}
	defer iter.Close()

	var ids []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		ids = append(ids, doc.Value().Get("id").GetString())
	}
	if err = iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate objects without sync details: %w", err)
	}
	return ids, nil
}

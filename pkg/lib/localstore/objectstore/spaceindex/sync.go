package spaceindex

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// syncDetailRelations are the relations maintained by helper.InjectsSyncDetails.
// Keep in sync with that function.
var syncDetailRelations = []domain.RelationKey{
	bundle.RelationKeySyncStatus,
	bundle.RelationKeySyncDate,
	bundle.RelationKeySyncError,
}

func (s *dsObjectStore) ListIdsWithoutSyncDetails(ctx context.Context) ([]string, error) {
	// An object needs sync details when at least one of the relations is
	// absent. Presence (not value) must be used: SyncStatusSynced and
	// SyncErrorNull are both the zero enum value, so a value/Empty filter
	// would re-select every already-synced object on each start.
	orFilter := make(query.Or, 0, len(syncDetailRelations))
	for _, key := range syncDetailRelations {
		orFilter = append(orFilter, query.Key{
			Path:   []string{string(key)},
			Filter: query.Not{Filter: query.Exists{}},
		})
	}

	iter, err := s.objects.Find(orFilter).Iter(ctx)
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

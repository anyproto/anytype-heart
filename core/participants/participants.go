// Package participants owns writes of participant object records.
//
// Participants are store-only objects: the per-space object index is their only
// persistence, no smartblock is involved. Subscription events fire from the store
// merge itself, and fulltext indexing is enqueued when name or description change
// (the only participant keys producing fulltext docs).
package participants

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// AclHeadMarkerId keys the per-space "last processed ACL head" marker in the
// object index heads-hash collection. While the marker equals the current ACL head,
// participant records in the store are up to date and per-member ACL reprocessing
// is skipped on space load. It must be cleared whenever participant records are
// wiped (see indexer.RemoveAclIndexes).
const AclHeadMarkerId = "_participants_acl_head"

// ModifyDetails merges newDetails into the participant record in the per-space
// object index. A missing record is created from baseDetails; when baseDetails is
// nil the call is update-only and a missing record is left absent.
//
// The space-id binding and the fulltext enqueue are committed BEFORE the merge:
// if they fail, the record stays unchanged and the caller's replay (ACL-head
// marker / identity profile cache not advanced) re-runs the whole write. The
// reverse order would lose them forever, because a replayed merge is a no-op and
// would never reach the side effects again. A spurious enqueue from a lost race
// is harmless: the fulltext consumer diffs against the index.
func ModifyDetails(ctx context.Context, store objectstore.ObjectStore, spaceId, id string, baseDetails, newDetails *domain.Details) error {
	spaceIndex := store.SpaceIndex(spaceId)
	current, err := spaceIndex.GetDetails(id)
	if err != nil {
		return fmt.Errorf("get current participant details: %w", err)
	}
	missing := current.Len() <= 1
	if missing && baseDetails == nil {
		return nil
	}
	base := current
	if missing {
		base = baseDetails
	}
	merged := base.Merge(newDetails)
	if merged.Equal(current) {
		return nil
	}
	if missing {
		err = store.BindSpaceId(ctx, spaceId, id)
		if err != nil {
			return fmt.Errorf("bind space id: %w", err)
		}
	}
	ftChanged := missing ||
		merged.GetString(bundle.RelationKeyName) != current.GetString(bundle.RelationKeyName) ||
		merged.GetString(bundle.RelationKeyDescription) != current.GetString(bundle.RelationKeyDescription)
	if ftChanged {
		// name and description are the only participant keys producing fulltext docs
		_, _, err = store.AddToIndexQueue(ctx, domain.FullID{SpaceID: spaceId, ObjectID: id})
		if err != nil {
			return fmt.Errorf("enqueue fulltext indexing: %w", err)
		}
	}
	// the merge is recomputed inside the store write to stay atomic against
	// concurrent participant writers (ACL vs identity paths)
	err = spaceIndex.ModifyObjectDetails(id, func(current *domain.Details) (*domain.Details, bool, error) {
		// a missing record arrives as an id-only document
		nowMissing := current.Len() <= 1
		if nowMissing && baseDetails == nil {
			return nil, false, nil
		}
		base := current
		if nowMissing {
			base = baseDetails
		}
		merged := base.Merge(newDetails)
		if merged.Equal(current) {
			return nil, false, nil
		}
		return merged, true, nil
	}, baseDetails != nil)
	if err != nil {
		return fmt.Errorf("modify participant details: %w", err)
	}
	return nil
}

// BuildIdentityDetails returns the participant details derived from an identity profile.
func BuildIdentityDetails(profile *model.IdentityProfile) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, profile.Name)
	details.SetString(bundle.RelationKeyDescription, profile.Description)
	details.SetString(bundle.RelationKeyIconImage, profile.IconCid)
	details.SetString(bundle.RelationKeyGlobalName, profile.GlobalName)
	return details
}

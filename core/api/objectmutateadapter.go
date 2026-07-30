package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// objectMutateAdapter implements apicore.ObjectMutator over the block
// service's object cache: the API v2 edit path (APIV2.md §2 Phase 3). The
// whole mutation happens under one object lock — the current read handed to
// build, the diff-apply of the snapshot build returns, and the post-apply
// heads — so the If-Match check, the edit, and the new etag are all
// consistent. The apply is the smartblock reset-to-version machinery (the
// pattern proven by core/block/import/common/objectcreator.updateExistingObject
// and core/block/history): NewDocFromSnapshot → ResetToVersion, which handles
// SetParent, local-detail injection, bundled relation links, and migrations,
// and lands the change as ONE change set.
type objectMutateAdapter struct {
	getter cache.ObjectGetter
}

func newObjectMutateAdapter(getter cache.ObjectGetter) apicore.ObjectMutator {
	return &objectMutateAdapter{getter: getter}
}

func (a *objectMutateAdapter) MutateObject(ctx context.Context, spaceId string, objectId string, build func(cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error)) ([]string, error) {
	var heads []string
	err := cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: objectId}, func(sb smartblock.SmartBlock) error {
		cur := readLiveState(sb)
		snapshot, err := build(cur)
		if err != nil {
			return err
		}
		if snapshot == nil {
			heads = cur.Heads
			return nil
		}
		st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: snapshot})
		if err != nil {
			return fmt.Errorf("state from snapshot: %w", err)
		}
		if err := guardBundledRevision(sb, st); err != nil {
			return err
		}
		if err := history.ResetToVersion(sb, st); err != nil {
			return fmt.Errorf("apply snapshot: %w", err)
		}
		heads = append([]string(nil), sb.GetDocInfo().Heads...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return heads, nil
}

// guardBundledRevision mirrors the import path's revision guard (objectcreator
// resetState / preserveBundledIdentity): an object derived from a bundled
// definition must never be reset to an older revision, and an incoming state
// that carries neither sourceObject nor revision keeps the live object's —
// resetting drops details the new state omits, and these two tie an installed
// object back to its bundled definition.
func guardBundledRevision(sb smartblock.SmartBlock, st *state.State) error {
	incomingDerived := st.Details().GetInt64(bundle.RelationKeyRevision) > 0 ||
		st.Details().GetString(bundle.RelationKeySourceObject) != ""
	if incomingDerived {
		current := sb.Details().GetInt64(bundle.RelationKeyRevision)
		incoming := st.Details().GetInt64(bundle.RelationKeyRevision)
		if current > incoming {
			return fmt.Errorf("the live object carries revision %d, newer than the document's %d — bundled definitions are never downgraded", current, incoming)
		}
		return nil
	}
	for _, key := range []domain.RelationKey{bundle.RelationKeySourceObject, bundle.RelationKeyRevision} {
		if value := sb.Details().Get(key); value.Ok() {
			st.SetDetail(key, value)
		}
	}
	return nil
}

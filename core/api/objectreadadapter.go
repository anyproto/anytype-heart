package api

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// objectReadAdapter implements apicore.ObjectReader over the block service's
// object cache: the API v2 read path (APIV2.md §8) — live smartblock state →
// snapshot, with the tree heads captured under the same lock so the etag and
// the content are consistent.
type objectReadAdapter struct {
	getter cache.ObjectGetter
	// spaces may be nil (test construction): the deletion probe and the
	// Bin lookup then do not run
	spaces spaceGetter
}

// spaceGetter is the slice of space.Service the deletion probe and the Bin
// lookup need.
type spaceGetter interface {
	Get(ctx context.Context, id string) (clientspace.Space, error)
}

func newObjectReadAdapter(getter cache.ObjectGetter, spaces spaceGetter) apicore.ObjectReader {
	return &objectReadAdapter{getter: getter, spaces: spaces}
}

func (a *objectReadAdapter) ReadObject(ctx context.Context, spaceId string, objectId string) (apicore.ObjectRead, error) {
	// a permanently deleted tree still loads for a moment: its storage is
	// removed asynchronously, and until then the cache rebuilds a stub a
	// restore answered "not archived" to (found live). The deletion mark in
	// the head storage is set before the delete RPC returns, so it is what
	// decides; a probe that fails says nothing and the read goes on.
	if a.spaces != nil {
		if spc, err := a.spaces.Get(ctx, spaceId); err == nil {
			if entry, err := spc.Storage().HeadStorage().GetEntry(ctx, objectId); err == nil && entry.DeletedStatus != headstorage.DeletedStatusNotDeleted {
				return apicore.ObjectRead{}, fmt.Errorf("read live object state: %w", spacestorage.ErrTreeStorageAlreadyDeleted)
			}
		}
	}
	var read apicore.ObjectRead
	err := cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: objectId}, func(sb smartblock.SmartBlock) error {
		read = readLiveState(sb)
		return nil
	})
	if err != nil {
		return apicore.ObjectRead{}, fmt.Errorf("read live object state: %w", err)
	}
	return read, nil
}

func (a *objectReadAdapter) InBin(ctx context.Context, spaceId string, objectId string) (bool, error) {
	if a.spaces == nil {
		return false, fmt.Errorf("read the Bin of space %s: no space service", spaceId)
	}
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return false, fmt.Errorf("get space %s: %w", spaceId, err)
	}
	var inBin bool
	err = cache.DoContextFullID(a.getter, ctx, domain.FullID{SpaceID: spaceId, ObjectID: spc.DerivedIDs().Archive}, func(sb smartblock.SmartBlock) error {
		bin, ok := sb.(blockcollection.Collection)
		if !ok {
			return fmt.Errorf("unexpected Bin object type %T", sb)
		}
		inBin, _ = bin.HasObject(objectId)
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("read the Bin of space %s: %w", spaceId, err)
	}
	return inBin, nil
}

// readLiveState captures one consistent read of a locked smartblock:
// snapshot and tree heads under the same lock, so the derived etag and the
// content always agree. Shared by the read and mutate adapters.
func readLiveState(sb smartblock.SmartBlock) apicore.ObjectRead {
	st := sb.NewState()
	return apicore.ObjectRead{
		SbType: sb.Type().ToProto(),
		Snapshot: &model.SmartBlockSnapshotBase{
			Blocks:      st.BlocksToSave(),
			Details:     st.CombinedDetails().ToProto(),
			ObjectTypes: domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			// C1: st.Store() returns the LIVE shared *types.Struct (no copy,
			// unlike BlocksToSave/CombinedDetails above). It is marshaled after
			// this lock releases, so a concurrent editor mutating the store
			// (e.g. adding/removing a collection item) would race the marshal —
			// an uncatchable "concurrent map read and map write" fatal. Copy it
			// under the lock. Must stay a copy.
			Collections: pbtypes.CopyStruct(st.Store(), true),
			Key:         st.UniqueKeyInternal(),
			FileInfo:    st.GetFileInfo().ToModel(),
		},
		Heads: append([]string(nil), sb.GetDocInfo().Heads...),
		// captured under the same lock so a dry run sees the same verdict the
		// real edit will (review C′3)
		BlocksRefused:     checkRestriction(sb, model.Restrictions_Blocks),
		DetailsRefused:    checkRestriction(sb, model.Restrictions_Details),
		TypeChangeRefused: checkRestriction(sb, model.Restrictions_TypeChange),
	}
}

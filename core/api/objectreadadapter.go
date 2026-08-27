package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// objectReadAdapter implements apicore.ObjectReader over the block service's
// object cache: the API v2 read path (APIV2.md §8) — live smartblock state →
// snapshot, with the tree heads captured under the same lock so the etag and
// the content are consistent.
type objectReadAdapter struct {
	getter cache.ObjectGetter
}

func newObjectReadAdapter(getter cache.ObjectGetter) apicore.ObjectReader {
	return &objectReadAdapter{getter: getter}
}

func (a *objectReadAdapter) ReadObject(ctx context.Context, spaceId string, objectId string) (apicore.ObjectRead, error) {
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
		BlocksRefused:  checkRestriction(sb, model.Restrictions_Blocks),
		DetailsRefused: checkRestriction(sb, model.Restrictions_Details),
	}
}

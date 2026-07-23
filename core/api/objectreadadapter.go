package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		st := sb.NewState()
		read.SbType = sb.Type().ToProto()
		read.Snapshot = &model.SmartBlockSnapshotBase{
			Blocks:      st.BlocksToSave(),
			Details:     st.CombinedDetails().ToProto(),
			ObjectTypes: domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			Collections: st.Store(),
			Key:         st.UniqueKeyInternal(),
			FileInfo:    st.GetFileInfo().ToModel(),
		}
		read.Heads = append([]string(nil), sb.GetDocInfo().Heads...)
		return nil
	})
	if err != nil {
		return apicore.ObjectRead{}, fmt.Errorf("read live object state: %w", err)
	}
	return read, nil
}

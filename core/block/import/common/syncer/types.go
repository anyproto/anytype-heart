package syncer

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Syncer interface {
	Sync(id domain.FullID, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, b simple.Block, origin model.ObjectOrigin) error
}

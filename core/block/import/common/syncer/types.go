package syncer

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
)

type Syncer interface {
	Sync(id domain.FullID, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, b simple.Block, origin *domain.ObjectOrigin) error
}

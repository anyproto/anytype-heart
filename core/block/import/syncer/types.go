package syncer

import (
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Syncer interface {
	Sync(id string, b simple.Block, origin model.ObjectOrigin) error
}

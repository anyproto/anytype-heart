package syncer

import (
	"github.com/anyproto/anytype-heart/core/block/simple"
)

type Syncer interface {
	Sync(id string, b simple.Block) error
}

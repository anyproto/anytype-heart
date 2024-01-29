package syncer

import (
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
)

type Syncer interface {
	Sync(id string, b simple.Block, origin *domain.ObjectOrigin) error
}

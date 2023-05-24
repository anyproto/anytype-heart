package syncer

import (
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
)

type Syncer interface {
	Sync(ctx *session.Context, id string, b simple.Block) error
}

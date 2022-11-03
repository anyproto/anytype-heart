package syncer

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
)

type Syncer interface {
	Sync(ctx *session.Context, id string, b simple.Block) error
}

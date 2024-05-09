package migration

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type DoableViaContext interface {
	DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error
	Id() string
}

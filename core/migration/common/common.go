package common

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

type SpaceWithCtx interface {
	DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error
	Id() string
}

type StoreWithCtx interface {
	QueryWithContext(ctx context.Context, q database.Query) (records []database.Record, err error)
}

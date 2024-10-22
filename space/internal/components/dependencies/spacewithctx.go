package dependencies

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

type SpaceWithCtx interface {
	DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error
	Id() string
	DerivedIDs() threads.DerivedSmartblockIds
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
}

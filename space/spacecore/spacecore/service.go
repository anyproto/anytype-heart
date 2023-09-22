package spacecore

import (
	"context"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

type SpaceReindexer interface {
	Reindex(ctx context.Context, spaceID string) error
}

type SpaceService interface {
	SpaceDerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error)
	DoSpaceObject(ctx context.Context, spaceID string, perform func(spaceObject SpaceObject) error) error

	app.ComponentRunnable
}

type AnySpace interface {
	commonspace.Space
}

type SpaceCoreService interface {
	Create(ctx context.Context) (AnySpace, error)
	Delete(ctx context.Context, spaceID string) (payload spacecore.NetworkStatus, err error)
	RevertDeletion(ctx context.Context) (err error)
	Get(ctx context.Context, id string) (AnySpace, error)

	StreamPool() streampool.StreamPool
	app.ComponentRunnable
}

type SpaceObject interface {
	SpaceID() string
	DerivedIDs() threads.DerivedSmartblockIds
}

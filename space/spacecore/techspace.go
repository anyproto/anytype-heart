package spacecore

import (
	"context"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

type TechSpace interface {
	SpaceDerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error)
	DoSpaceObject(ctx context.Context, spaceID string, openBlock func(spaceObject SpaceObject) error) error
	PredefinedObjects(ctx context.Context, sp commonspace.Space, create bool) (objIDs threads.DerivedSmartblockIds, err error)
	PreinstalledObjects(ctx context.Context, spaceID string) error

	CreateSpace(ctx context.Context) (SpaceObject, error)
}

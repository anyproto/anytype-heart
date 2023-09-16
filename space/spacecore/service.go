package spacecore

import (
	"context"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/net/streampool"
)

type SpaceService interface {
	AccountSpace(ctx context.Context) (commonspace.Space, error)
	AccountId() string
	CreateSpace(ctx context.Context) (container commonspace.Space, err error)
	GetSpace(ctx context.Context, id string) (commonspace.Space, error)
	DeriveSpace(ctx context.Context, payload commonspace.SpaceDerivePayload) (commonspace.Space, error)
	DeleteSpace(ctx context.Context, spaceID string, revert bool) (payload StatusPayload, err error)
	DeleteAccount(ctx context.Context, revert bool) (payload StatusPayload, err error)
	StreamPool() streampool.StreamPool
	ResolveSpaceID(objectID string) (string, error)
	StoreSpaceID(objectID, spaceID string) error

	TechSpace() TechSpace
	app.ComponentRunnable
}

package idresolver

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

const CName = "block.object.resolver"

type Resolver interface {
	app.Component
	ResolveSpaceID(objectID string) (string, error)
	ResolveSpaceIdWithRetry(ctx context.Context, objectID string) (string, error)
}

func New(retryStartDelay time.Duration, retryMaxDelay time.Duration) Resolver {
	if retryStartDelay == 0 {
		retryStartDelay = 100 * time.Millisecond
	}
	if retryMaxDelay == 0 {
		retryMaxDelay = time.Second
	}
	return &resolver{
		retryStartDelay: retryStartDelay,
		retryMaxDelay:   retryMaxDelay,
	}
}

type resolver struct {
	objectStore     objectstore.ObjectStore
	retryStartDelay time.Duration
	retryMaxDelay   time.Duration
	sync.Mutex
}

func (r *resolver) Init(a *app.App) (err error) {
	r.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return
}

func (r *resolver) Name() (name string) {
	return CName
}

func (r *resolver) ResolveSpaceID(objectID string) (string, error) {
	return r.objectStore.GetSpaceId(objectID)
}

func (r *resolver) ResolveSpaceIdWithRetry(ctx context.Context, objectId string) (string, error) {
	return retry.DoWithData(func() (string, error) {
		return r.ResolveSpaceID(objectId)
	},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(r.retryStartDelay),
		retry.MaxDelay(r.retryMaxDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

package idresolver

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

const CName = "block.object.resolver"

type Resolver interface {
	app.ComponentRunnable
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
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	objectStore     objectstore.ObjectStore
	retryStartDelay time.Duration
	retryMaxDelay   time.Duration
	sync.Mutex
}

func (r *resolver) Init(a *app.App) (err error) {
	r.componentCtx, r.componentCtxCancel = context.WithCancel(context.Background())
	r.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return
}

func (r *resolver) Run(_ context.Context) error { return nil }

func (r *resolver) Close(_ context.Context) error {
	if r.componentCtxCancel != nil {
		r.componentCtxCancel()
	}
	return nil
}

func (r *resolver) Name() (name string) {
	return CName
}

func (r *resolver) ResolveSpaceID(objectID string) (string, error) {
	select {
	case <-r.componentCtx.Done():
		return "", r.componentCtx.Err()
	default:
	}
	return r.objectStore.GetSpaceId(objectID)
}

func (r *resolver) ResolveSpaceIdWithRetry(ctx context.Context, objectId string) (string, error) {
	return retry.DoWithData(func() (string, error) {
		spaceId, err := r.ResolveSpaceID(objectId)
		if errors.Is(err, context.Canceled) {
			return "", retry.Unrecoverable(err)
		}
		return spaceId, err
	},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(r.retryStartDelay),
		retry.MaxDelay(r.retryMaxDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

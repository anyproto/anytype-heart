package space

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"
)

var loadingRetryTimeout = time.Second * 20

type spaceServiceProvider interface {
	open(ctx context.Context, spaceId string) (Space, error)
	onLoad(spaceId string, sp Space, loadErr error) (err error)
}

func newLoadingSpace(ctx context.Context, spaceID string, serviceProvider spaceServiceProvider) *loadingSpace {
	ls := &loadingSpace{
		ID:                   spaceID,
		retryTimeout:         loadingRetryTimeout,
		spaceServiceProvider: serviceProvider,
		loadCh:               make(chan struct{}),
	}
	go ls.loadRetry(ctx)
	return ls
}

type loadingSpace struct {
	ID           string
	retryTimeout time.Duration

	spaceServiceProvider spaceServiceProvider

	// results
	space   Space
	loadErr error
	loadCh  chan struct{}
}

func (ls *loadingSpace) loadRetry(ctx context.Context) {
	defer func() {
		if err := ls.spaceServiceProvider.onLoad(ls.ID, ls.space, ls.loadErr); err != nil {
			log.WarnCtx(ctx, "space onLoad error", zap.Error(err))
		}
		close(ls.loadCh)
	}()
	if ls.load(ctx) {
		return
	}
	ticker := time.NewTicker(ls.retryTimeout)
	for {
		select {
		case <-ctx.Done():
			ls.loadErr = ctx.Err()
			return
		case <-ticker.C:
			if ls.load(ctx) {
				return
			}
		}
	}
}

func (ls *loadingSpace) load(ctx context.Context) (ok bool) {
	sp, err := ls.spaceServiceProvider.open(ctx, ls.ID)
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		return false
	}
	if err == nil {
		err = sp.WaitMandatoryObjects(ctx)
	}
	if err != nil {
		ls.loadErr = err
	} else {
		ls.space = sp
	}
	return true
}

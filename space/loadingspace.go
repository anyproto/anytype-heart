package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"
)

var loadingRetryTimeout = time.Second * 20

type spaceOpener func(ctx context.Context, spaceID string) (Space, error)
type spaceOnLoad func(spaceID string, s Space, loadErr error) error

func newLoadingSpace(ctx context.Context, spaceOpener spaceOpener, spaceID string, onLoad spaceOnLoad) *loadingSpace {
	ls := &loadingSpace{
		ID:           spaceID,
		retryTimeout: loadingRetryTimeout,
		spaceOpener:  spaceOpener,
		onLoad:       onLoad,
		loadCh:       make(chan struct{}),
	}
	go ls.loadRetry(ctx)
	return ls
}

type loadingSpace struct {
	ID           string
	retryTimeout time.Duration

	spaceOpener spaceOpener
	onLoad      spaceOnLoad

	// results
	space   Space
	loadErr error
	loadCh  chan struct{}
}

func (ls *loadingSpace) loadRetry(ctx context.Context) {
	defer func() {
		if err := ls.onLoad(ls.ID, ls.space, ls.loadErr); err != nil {
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
	sp, err := ls.spaceOpener(ctx, ls.ID)
	if err == spacesyncproto.ErrSpaceMissing {
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

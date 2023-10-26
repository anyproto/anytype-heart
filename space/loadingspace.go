package space

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"
)

var loadingRetryTimeout = time.Second * 20

type spaceServiceProvider interface {
	open(ctx context.Context, spaceId string, justCreated bool) (Space, error)
	onLoad(spaceId string, sp Space, loadErr error) (err error)
}

type loadingSpace struct {
	ID           string
	retryTimeout time.Duration
	justCreated  bool // indicates that user created space

	spaceServiceProvider spaceServiceProvider

	// results
	space   Space
	loadErr error
	loadCh  chan struct{}
}

func (s *service) newLoadingSpace(ctx context.Context, spaceID string, justCreated bool) *loadingSpace {
	ls := &loadingSpace{
		ID:                   spaceID,
		retryTimeout:         loadingRetryTimeout,
		justCreated:          justCreated,
		spaceServiceProvider: s,
		loadCh:               make(chan struct{}),
	}
	go ls.loadRetry(ctx)
	return ls
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
	sp, err := ls.spaceServiceProvider.open(ctx, ls.ID, ls.justCreated)
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		return false
	}
	if err == nil {
		err = sp.WaitMandatoryObjects(ctx)
		if errors.Is(err, treechangeproto.ErrGetTree) {
			return false
		}
	}
	if err != nil {
		if sp != nil {
			closeErr := sp.Close(ctx)
			if closeErr != nil {
				log.WarnCtx(ctx, "space close error", zap.Error(closeErr))
			}
		}
		ls.loadErr = err
	} else {
		ls.space = sp
	}
	return true
}

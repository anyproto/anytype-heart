package spaceloader

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/clientspace"
)

var (
	loadingRetryTimeout = time.Second * 20
	log                 = logger.NewNamed(CName)
)

type spaceServiceProvider interface {
	open(ctx context.Context) (clientspace.Space, error)
	onLoad(sp clientspace.Space, loadErr error) (err error)
}

type loadingSpace struct {
	ID           string
	retryTimeout time.Duration

	spaceServiceProvider spaceServiceProvider

	// results
	stopIfMandatoryFail bool
	disableRemoteLoad   bool
	space               clientspace.Space
	loadErr             error
	loadCh              chan struct{}
}

func (s *spaceLoader) newLoadingSpace(ctx context.Context, stopIfMandatoryFail, disableRemoteLoad bool) *loadingSpace {
	ls := &loadingSpace{
		stopIfMandatoryFail:  stopIfMandatoryFail,
		disableRemoteLoad:    disableRemoteLoad,
		retryTimeout:         loadingRetryTimeout,
		spaceServiceProvider: s,
		loadCh:               make(chan struct{}),
	}
	go ls.loadRetry(ctx)
	return ls
}

func (ls *loadingSpace) loadRetry(ctx context.Context) {
	defer func() {
		if err := ls.spaceServiceProvider.onLoad(ls.space, ls.loadErr); err != nil {
			log.WarnCtx(ctx, "space onLoad error", zap.Error(err))
		}
		close(ls.loadCh)
	}()
	if ls.load(ctx) {
		return
	}
	timeout := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			ls.loadErr = ctx.Err()
			return
		case <-time.After(timeout):
			if ls.load(ctx) {
				return
			}
		}
		timeout = timeout * 15 / 10
		if timeout > ls.retryTimeout {
			timeout = ls.retryTimeout
		}
	}
}

func (ls *loadingSpace) load(ctx context.Context) (ok bool) {
	sp, err := ls.spaceServiceProvider.open(ctx)
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		return ls.disableRemoteLoad
	}
	if err == nil {
		err = sp.WaitMandatoryObjects(ctx)
		if errors.Is(err, treechangeproto.ErrGetTree) || errors.Is(err, objecttree.ErrHasInvalidChanges) || errors.Is(err, list.ErrNoReadKey) {
			if ls.stopIfMandatoryFail {
				ls.loadErr = err
				return true
			}
			return ls.disableRemoteLoad
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

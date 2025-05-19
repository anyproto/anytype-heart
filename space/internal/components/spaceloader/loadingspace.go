package spaceloader

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
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
	latestAclHeadId     string
	space               clientspace.Space

	loadCh chan struct{}

	lock    sync.Mutex
	loadErr error
}

func (s *spaceLoader) newLoadingSpace(ctx context.Context, stopIfMandatoryFail, disableRemoteLoad bool, aclHeadId string) *loadingSpace {
	ls := &loadingSpace{
		stopIfMandatoryFail:  stopIfMandatoryFail,
		disableRemoteLoad:    disableRemoteLoad,
		retryTimeout:         loadingRetryTimeout,
		latestAclHeadId:      aclHeadId,
		spaceServiceProvider: s,
		loadCh:               make(chan struct{}),
	}
	if s.status != nil {
		ls.ID = s.status.SpaceId()
	}
	go ls.loadRetry(ctx)
	return ls
}

func (ls *loadingSpace) getLoadErr() error {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	return ls.loadErr
}

func (ls *loadingSpace) setLoadErr(err error) {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	ls.loadErr = err
}

func (ls *loadingSpace) loadRetry(ctx context.Context) {
	defer func() {
		if err := ls.spaceServiceProvider.onLoad(ls.space, ls.getLoadErr()); err != nil {
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
			ls.setLoadErr(ctx.Err())
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

func (ls *loadingSpace) load(ctx context.Context) (notRetryable bool) {
	sp, err := ls.spaceServiceProvider.open(ctx)
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		log.WarnCtx(ctx, "space load: space is missing", zap.String("spaceId", ls.ID), zap.Bool("notRetryable", ls.disableRemoteLoad), zap.Error(err))
		return ls.disableRemoteLoad
	}
	if err == nil {
		err = sp.WaitMandatoryObjects(ctx)
		if err != nil {
			notRetryable = errors.Is(err, objecttree.ErrHasInvalidChanges)
			ls.setLoadErr(err)
			log.WarnCtx(ctx, "space load: mandatory objects error", zap.String("spaceId", ls.ID), zap.Error(err), zap.Bool("notRetryable", ls.disableRemoteLoad || notRetryable))
			return ls.disableRemoteLoad || notRetryable
		}
	}
	if err != nil {
		if sp != nil {
			closeErr := sp.Close(ctx)
			if closeErr != nil {
				log.WarnCtx(ctx, "space close error", zap.Error(closeErr))
			}
		}
		ls.setLoadErr(err)
		if errors.Is(err, context.Canceled) {
			log.WarnCtx(ctx, "space load: error: context bug", zap.String("spaceId", ls.ID), zap.Error(err), zap.Bool("notRetryable", ls.disableRemoteLoad))
			// hotfix for drpc bug
			// todo: remove after https://github.com/anyproto/any-sync/pull/448 got integrated
			return ls.disableRemoteLoad
		}
		log.WarnCtx(ctx, "space load: error", zap.String("spaceId", ls.ID), zap.Error(err), zap.Bool("notRetryable", true))
	} else {
		if ls.latestAclHeadId != "" && !ls.disableRemoteLoad {
			acl := sp.CommonSpace().Acl()
			acl.RLock()
			defer acl.RUnlock()
			_, err := acl.Get(ls.latestAclHeadId)
			if err != nil {
				log.WarnCtx(ctx, "space load: acl head not found", zap.String("spaceId", ls.ID), zap.String("aclHeadId", ls.latestAclHeadId), zap.Error(err), zap.Bool("notRetryable", false))
				return false
			}
		}
		ls.space = sp
	}
	return true
}

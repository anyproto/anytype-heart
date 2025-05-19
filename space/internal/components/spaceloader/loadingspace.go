package spaceloader

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
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
			log.WarnCtx(ctx, "space onLoad error", zap.Error(err), zap.Error(ls.getLoadErr()))
		}
		close(ls.loadCh)
	}()
	shouldReturn, err := ls.load(ctx)
	if shouldReturn {
		ls.setLoadErr(err)
		return
	}
	timeout := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			ls.setLoadErr(ctx.Err())
			return
		case <-time.After(timeout):
			shouldReturn, err := ls.load(ctx)
			if shouldReturn {
				ls.setLoadErr(err)
				return
			}
		}
		timeout = timeout * 15 / 10
		if timeout > ls.retryTimeout {
			timeout = ls.retryTimeout
		}
	}
}

func (ls *loadingSpace) logErrors(ctx context.Context, err error, mandatoryObjects bool, notRetryable bool) {
	log := log.With(zap.String("spaceId", ls.ID), zap.Error(err), zap.Bool("notRetryable", notRetryable))
	if mandatoryObjects {
		log.WarnCtx(ctx, "space load: mandatory objects error")
		if errors.Is(err, context.Canceled) {
			log.WarnCtx(ctx, "space load: error: context bug")
		}
	} else {
		log.WarnCtx(ctx, "space load: build space error")
	}
}

func (ls *loadingSpace) isNotRetryable(err error) bool {
	return errors.Is(err, objecttree.ErrHasInvalidChanges) || ls.disableRemoteLoad
}

func (ls *loadingSpace) load(ctx context.Context) (ok bool, err error) {
	sp, err := ls.spaceServiceProvider.open(ctx)
	if err != nil {
		notRetryable := ls.isNotRetryable(err)
		ls.logErrors(ctx, err, false, notRetryable)
		return notRetryable, err
	}
	err = sp.WaitMandatoryObjects(ctx)
	if err != nil {
		notRetryable := ls.isNotRetryable(err)
		ls.logErrors(ctx, err, true, notRetryable)
		return notRetryable, err
	}
	if ls.latestAclHeadId != "" && !ls.disableRemoteLoad {
		acl := sp.CommonSpace().Acl()
		acl.RLock()
		defer acl.RUnlock()
		_, err := acl.Get(ls.latestAclHeadId)
		if err != nil {
			return false, err
		}
	}
	ls.space = sp
	return true, nil
}

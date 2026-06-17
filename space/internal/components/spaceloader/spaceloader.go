package spaceloader

/*
AI generated

Name: Async Space Loader with Retry
Scope: space

## Responsibility
- Initiates asynchronous space building on Run() and exposes WaitLoad() for completion
- Updates space local status (Loading -> Ok/Missing) via spacestatus component

## Background Tasks
- loadRetry: Retries space loading with exponential backoff (1s -> 20s cap) until success or non-retryable error (loadingspace.go)

## Documentation
Non-retryable errors that stop retry loop: ErrHasInvalidChanges, ErrUnexpectedSpaceType, or when disableRemoteLoad is set.
ACL head validation: If latestAclHeadId is provided and remote load is enabled, verifies ACL contains the expected head before considering load complete.
*/

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/builder"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	spaceservice "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.spaceloader"

var (
	ErrSpaceDeleted = errors.New("space is deleted")
)

type SpaceLoader interface {
	app.ComponentRunnable
	WaitLoad(ctx context.Context) (sp clientspace.Space, err error)
}

type spaceLoader struct {
	status              spacestatus.SpaceStatus
	builder             builder.SpaceBuilder
	storageService      storage.ClientStorage
	loading             *loadingSpace
	stopIfMandatoryFail bool
	disableRemoteLoad   bool

	ctx    context.Context
	cancel context.CancelFunc
	space  clientspace.Space
	mx     sync.Mutex
}

func New(stopIfMandatoryFail, disableRemoteLoad bool) SpaceLoader {
	return &spaceLoader{
		stopIfMandatoryFail: stopIfMandatoryFail,
		disableRemoteLoad:   disableRemoteLoad,
	}
}

func (s *spaceLoader) Init(a *app.App) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.status = app.MustComponent[spacestatus.SpaceStatus](a)
	s.builder = app.MustComponent[builder.SpaceBuilder](a)
	s.storageService = app.MustComponent[storage.ClientStorage](a)
	return nil
}

func (s *spaceLoader) Name() (name string) {
	return CName
}

func (s *spaceLoader) Run(ctx context.Context) (err error) {
	return s.startLoad(ctx)
}

func (s *spaceLoader) Close(ctx context.Context) (err error) {
	s.mx.Lock()
	if s.loading == nil {
		s.mx.Unlock()
		return nil
	}
	s.mx.Unlock()
	s.cancel()
	sp, err := s.WaitLoad(ctx)
	if err != nil {
		return
	}
	return sp.Close(ctx)
}

func (s *spaceLoader) startLoad(ctx context.Context) (err error) {
	// Probe the on-disk store BEFORE taking s.mx: SpaceExists performs blocking I/O (an os.Stat
	// on anystorage, a synchronized SQL query on sqlitestorage) and depends only on the immutable
	// SpaceId, so keeping it out of the critical section avoids stalling concurrent
	// WaitLoad/onLoad/Close callers that also contend for s.mx.
	onDisk := s.storageService.SpaceExists(s.status.SpaceId())

	s.mx.Lock()
	defer s.mx.Unlock()

	if s.status.GetPersistentStatus() == spaceinfo.AccountStatusDeleted {
		return ErrSpaceDeleted
	}
	// Fast path: a space whose store still exists on disk and was already Ok in the previous
	// session keeps reporting Ok to clients. We still run the background build below; we just
	// do not publish a transient Loading (which would make the client hide the space on cold
	// start). If onLoad later fails, it sets Missing (accepted Ok->Missing regression).
	onDiskAndOk := s.status.GetLocalStatus() == spaceinfo.LocalStatusOk && onDisk
	if !onDiskAndOk {
		info := spaceinfo.NewSpaceLocalInfo(s.status.SpaceId())
		info.SetLocalStatus(spaceinfo.LocalStatusLoading)
		if err = s.status.SetLocalInfo(info); err != nil {
			return
		}
	}
	s.loading = s.newLoadingSpace(s.ctx, s.stopIfMandatoryFail, s.disableRemoteLoad, s.status.GetLatestAclHeadId())
	return
}

func (s *spaceLoader) onLoad(sp clientspace.Space, loadErr error) (err error) {
	s.mx.Lock()
	defer s.mx.Unlock()
	info := spaceinfo.NewSpaceLocalInfo(s.status.SpaceId())
	switch {
	case loadErr == nil:
		s.space = sp
		info.SetLocalStatus(spaceinfo.LocalStatusOk)
	case errors.Is(loadErr, spaceservice.ErrSpaceDeletionPending):
		info.SetLocalStatus(spaceinfo.LocalStatusMissing).
			SetRemoteStatus(spaceinfo.RemoteStatusWaitingDeletion)
	case errors.Is(loadErr, spaceservice.ErrSpaceIsDeleted):
		info.SetLocalStatus(spaceinfo.LocalStatusMissing).
			SetRemoteStatus(spaceinfo.RemoteStatusDeleted)
	case errors.Is(loadErr, context.Canceled), errors.Is(loadErr, context.DeadlineExceeded):
		// The component context was canceled (Close/shutdown), so the background build was
		// interrupted rather than genuinely failing. Persisting Missing here would knock a healthy
		// space off the optimistic-Ok fast path on the next cold start, so leave the persisted
		// status untouched and let the next session re-evaluate it.
		return nil
	default:
		info.SetLocalStatus(spaceinfo.LocalStatusMissing)
	}

	return s.status.SetLocalInfo(info)
}

func (s *spaceLoader) open(ctx context.Context) (clientspace.Space, error) {
	return s.builder.BuildSpace(ctx, s.disableRemoteLoad)
}

func (s *spaceLoader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	s.mx.Lock()
	// Readiness is driven by the loader's own lifecycle, NOT by the client-facing persisted
	// localStatus. localStatus may be an optimistic Ok (set before the background build
	// finished); returning s.space here without the loadCh wait would hand back a nil space.
	if s.loading == nil {
		s.mx.Unlock()
		return nil, fmt.Errorf("waitLoad for a not started space")
	}
	if s.space != nil {
		sp = s.space
		s.mx.Unlock()
		return sp, nil
	}
	loading := s.loading
	loadErr := loading.getLoadErr()
	if loadErr != nil {
		s.mx.Unlock()
		return nil, loadErr
	}
	waitCh := loading.loadCh
	s.mx.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-waitCh:
	}
	return s.WaitLoad(ctx)
}

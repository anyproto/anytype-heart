package spaceloader

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
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.status.GetPersistentStatus() == spaceinfo.AccountStatusDeleted {
		return ErrSpaceDeleted
	}
	info := spaceinfo.NewSpaceLocalInfo(s.status.SpaceId())
	info.SetLocalStatus(spaceinfo.LocalStatusLoading)
	if err = s.status.SetLocalInfo(info); err != nil {
		return
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
	status := s.status.GetLocalStatus()

	switch status {
	case spaceinfo.LocalStatusUnknown:
		s.mx.Unlock()
		return nil, fmt.Errorf("waitLoad for an unknown space")
	case spaceinfo.LocalStatusLoading:
		// loading in progress, wait channel and retry
		waitCh := s.loading.loadCh
		loadErr := s.loading.getLoadErr()
		s.mx.Unlock()
		if loadErr != nil {
			return nil, loadErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
		return s.WaitLoad(ctx)
	case spaceinfo.LocalStatusMissing:
		// local missing state means the loader ended with an error
		err = s.loading.getLoadErr()
	case spaceinfo.LocalStatusOk:
		sp = s.space
	default:
		err = fmt.Errorf("undefined space state: %v", status)
	}
	s.mx.Unlock()
	return
}

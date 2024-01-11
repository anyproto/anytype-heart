package spaceloader

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/builder"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/techspace"
	spaceservice "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.spaceloader"

var (
	ErrSpaceDeleted   = errors.New("space is deleted")
	ErrSpaceNotExists = errors.New("space not exists")
)

type SpaceLoader interface {
	app.ComponentRunnable
	WaitLoad(ctx context.Context) (sp clientspace.Space, err error)
}

type spaceLoader struct {
	techSpace           techspace.TechSpace
	status              spacestatus.SpaceStatus
	builder             builder.SpaceBuilder
	loading             *loadingSpace
	stopIfMandatoryFail bool

	ctx    context.Context
	cancel context.CancelFunc
	space  clientspace.Space
}

func New(stopIfMandatoryFail bool) SpaceLoader {
	return &spaceLoader{
		stopIfMandatoryFail: stopIfMandatoryFail,
	}
}

func (s *spaceLoader) Init(a *app.App) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.techSpace = app.MustComponent[techspace.TechSpace](a)
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
	s.status.Lock()
	if s.loading == nil {
		s.status.Unlock()
		return nil
	}
	s.status.Unlock()
	s.cancel()
	sp, err := s.WaitLoad(ctx)
	if err != nil {
		return
	}
	return sp.Close(ctx)
}

func (s *spaceLoader) startLoad(ctx context.Context) (err error) {
	s.status.Lock()
	defer s.status.Unlock()
	persistentStatus := s.status.GetPersistentStatus()
	if persistentStatus == spaceinfo.AccountStatusDeleted {
		return ErrSpaceDeleted
	}
	localStatus := s.status.GetLocalStatus()
	// Do nothing if space is already loading
	if localStatus != spaceinfo.LocalStatusUnknown {
		return nil
	}

	exists, err := s.techSpace.SpaceViewExists(ctx, s.status.SpaceId())
	if err != nil {
		return
	}
	if !exists {
		return ErrSpaceNotExists
	}
	info := spaceinfo.SpaceLocalInfo{
		SpaceID:     s.status.SpaceId(),
		LocalStatus: spaceinfo.LocalStatusLoading,
	}
	if err = s.status.SetLocalInfo(ctx, info); err != nil {
		return
	}
	s.loading = s.newLoadingSpace(s.ctx, s.stopIfMandatoryFail, s.status.SpaceId())
	return
}

func (s *spaceLoader) onLoad(spaceID string, sp clientspace.Space, loadErr error) (err error) {
	s.status.Lock()
	defer s.status.Unlock()

	switch {
	case loadErr == nil:
	case errors.Is(loadErr, spaceservice.ErrSpaceDeletionPending):
		return s.status.SetLocalInfo(s.ctx, spaceinfo.SpaceLocalInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusWaitingDeletion,
		})
	case errors.Is(loadErr, spaceservice.ErrSpaceIsDeleted):
		return s.status.SetLocalInfo(s.ctx, spaceinfo.SpaceLocalInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusDeleted,
		})
	default:
		return s.status.SetLocalInfo(s.ctx, spaceinfo.SpaceLocalInfo{
			SpaceID:      spaceID,
			LocalStatus:  spaceinfo.LocalStatusMissing,
			RemoteStatus: spaceinfo.RemoteStatusError,
		})
	}

	s.space = sp
	// TODO: check remote state
	return s.status.SetLocalInfo(s.ctx, spaceinfo.SpaceLocalInfo{
		SpaceID:      spaceID,
		LocalStatus:  spaceinfo.LocalStatusOk,
		RemoteStatus: spaceinfo.RemoteStatusUnknown,
	})
}

func (s *spaceLoader) open(ctx context.Context, spaceId string) (clientspace.Space, error) {
	// TODO: [MR] remove extra params
	return s.builder.BuildSpace(ctx)
}

func (s *spaceLoader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	s.status.Lock()
	status := s.status.GetLocalStatus()

	switch status {
	case spaceinfo.LocalStatusUnknown:
		return nil, fmt.Errorf("waitLoad for an unknown space")
	case spaceinfo.LocalStatusLoading:
		// loading in progress, wait channel and retry
		waitCh := s.loading.loadCh
		loadErr := s.loading.loadErr
		s.status.Unlock()
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
		err = s.loading.loadErr
	case spaceinfo.LocalStatusOk:
		sp = s.space
	default:
		err = fmt.Errorf("undefined space state: %v", status)
	}
	s.status.Unlock()
	return
}

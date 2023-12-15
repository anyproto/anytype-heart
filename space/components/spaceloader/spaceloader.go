package spaceloader

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/components/builder"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	spaceservice "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "common.clientspace.spaceloader"

var (
	ErrSpaceDeleted   = errors.New("space is deleted")
	ErrSpaceNotExists = errors.New("space not exists")
)

type SpaceLoader interface {
	app.ComponentRunnable
	WaitLoad(ctx context.Context) (sp clientspace.Space, err error)
}

type spaceLoader struct {
	techSpace techspace.TechSpace
	status    spacestatus.SpaceStatus
	builder   builder.SpaceBuilder
	loading   *loadingSpace

	ctx    context.Context
	cancel context.CancelFunc
	space  clientspace.Space

	justCreated bool
	loadCh      chan struct{}
	loadErr     error
}

func New(justCreated bool) SpaceLoader {
	return &spaceLoader{
		justCreated: justCreated,
	}
}

func (s *spaceLoader) Init(a *app.App) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.techSpace = a.MustComponent(techspace.CName).(techspace.TechSpace)
	s.loadCh = make(chan struct{})
	return nil
}

func (s *spaceLoader) Name() (name string) {
	return CName
}

func (s *spaceLoader) Run(ctx context.Context) (err error) {
	err = s.startLoad(ctx)
	if err != nil {
		return
	}
	_, err = s.WaitLoad(ctx)
	return
}

func (s *spaceLoader) Close(ctx context.Context) (err error) {
	return nil
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
	s.loading = s.newLoadingSpace(s.ctx, s.status.SpaceId(), s.justCreated)
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

	// TODO: check remote state
	return s.status.SetLocalInfo(s.ctx, spaceinfo.SpaceLocalInfo{
		SpaceID:      spaceID,
		LocalStatus:  spaceinfo.LocalStatusOk,
		RemoteStatus: spaceinfo.RemoteStatusUnknown,
	})
}

func (s *spaceLoader) open(ctx context.Context, spaceId string, justCreated bool) (clientspace.Space, error) {
	// TODO: [MR] remove extra params
	return s.builder.BuildSpace(ctx, s.justCreated)
}

func (s *spaceLoader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	s.status.Lock()
	status := s.status.GetLocalStatus()

	switch status {
	case spaceinfo.LocalStatusUnknown:
		return nil, fmt.Errorf("waitLoad for an unknown space")
	case spaceinfo.LocalStatusLoading:
		// loading in progress, wait channel and retry
		waitCh := s.loadCh
		s.status.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
		return s.WaitLoad(ctx)
	case spaceinfo.LocalStatusMissing:
		// local missing state means the loader ended with an error
		err = s.loadErr
	case spaceinfo.LocalStatusOk:
		sp = s.space
	default:
		err = fmt.Errorf("undefined space state: %v", status)
	}
	s.status.Unlock()
	return
}

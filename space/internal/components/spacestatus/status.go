package spacestatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/internal/techspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.spacestatus"

type SpaceStatus interface {
	app.Component
	sync.Locker
	SpaceId() string
	GetLocalStatus() spaceinfo.LocalStatus
	GetRemoteStatus() spaceinfo.RemoteStatus
	GetPersistentStatus() spaceinfo.AccountStatus
	UpdatePersistentStatus(ctx context.Context, status spaceinfo.AccountStatus)
	SetRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error
	SetPersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) (err error)
	SetLocalStatus(ctx context.Context, status spaceinfo.LocalStatus) error
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error)
}

type spaceStatus struct {
	sync.Mutex
	spaceId       string
	accountStatus spaceinfo.AccountStatus
	localStatus   spaceinfo.LocalStatus
	remoteStatus  spaceinfo.RemoteStatus
	techSpace     techspace.TechSpace
}

func New(spaceId string, accountStatus spaceinfo.AccountStatus) SpaceStatus {
	return &spaceStatus{
		accountStatus: accountStatus,
		spaceId:       spaceId,
	}
}

func (s *spaceStatus) Init(a *app.App) (err error) {
	s.techSpace = a.MustComponent(techspace.CName).(techspace.TechSpace)
	return nil
}

func (s *spaceStatus) Name() (name string) {
	return CName
}

func (s *spaceStatus) SpaceId() string {
	return s.spaceId
}

func (s *spaceStatus) GetLocalStatus() spaceinfo.LocalStatus {
	return s.localStatus
}

func (s *spaceStatus) GetRemoteStatus() spaceinfo.RemoteStatus {
	return s.remoteStatus
}

func (s *spaceStatus) GetPersistentStatus() spaceinfo.AccountStatus {
	return s.accountStatus
}

func (s *spaceStatus) UpdatePersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) {
	s.accountStatus = status
}

func (s *spaceStatus) SetRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	s.remoteStatus = status
	return s.setCurrentLocalInfo(ctx)
}

func (s *spaceStatus) SetLocalStatus(ctx context.Context, status spaceinfo.LocalStatus) error {
	s.localStatus = status
	return s.setCurrentLocalInfo(ctx)
}

func (s *spaceStatus) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	if s.localStatus == info.LocalStatus && info.RemoteStatus == s.remoteStatus {
		return nil
	}
	s.localStatus = info.LocalStatus
	s.remoteStatus = info.RemoteStatus
	return s.setCurrentLocalInfo(ctx)
}

func (s *spaceStatus) SetPersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) (err error) {
	if s.GetPersistentStatus() == status {
		return nil
	}
	if err = s.techSpace.SetPersistentInfo(ctx, spaceinfo.SpacePersistentInfo{
		SpaceID:       s.spaceId,
		AccountStatus: status,
	}); err != nil {
		return err
	}
	s.accountStatus = status
	return nil
}

func (s *spaceStatus) setCurrentLocalInfo(ctx context.Context) (err error) {
	return s.techSpace.SetLocalInfo(ctx, spaceinfo.SpaceLocalInfo{
		SpaceID:      s.spaceId,
		LocalStatus:  s.localStatus,
		RemoteStatus: s.remoteStatus,
	})
}

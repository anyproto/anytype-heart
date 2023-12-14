package spacestatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.common.spacestatus"

type SpaceStatus struct {
	sync.Mutex
	SpaceId       string
	AccountStatus spaceinfo.AccountStatus
	LocalStatus   spaceinfo.LocalStatus
	RemoteStatus  spaceinfo.RemoteStatus
	techSpace     techspace.TechSpace
}

func New(spaceId string, accountStatus spaceinfo.AccountStatus) *SpaceStatus {
	return &SpaceStatus{
		AccountStatus: accountStatus,
		SpaceId:       spaceId,
	}
}

func (s *SpaceStatus) Init(a *app.App) (err error) {
	s.techSpace = a.MustComponent(techspace.CName).(techspace.TechSpace)
	return nil
}

func (s *SpaceStatus) Name() (name string) {
	return CName
}

func (s *SpaceStatus) GetLocalStatus() spaceinfo.LocalStatus {
	return s.LocalStatus
}

func (s *SpaceStatus) GetRemoteStatus() spaceinfo.RemoteStatus {
	return s.RemoteStatus
}

func (s *SpaceStatus) GetPersistentStatus() spaceinfo.AccountStatus {
	return s.AccountStatus
}

func (s *SpaceStatus) UpdatePersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) {
	s.AccountStatus = status
}

func (s *SpaceStatus) SetRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	s.RemoteStatus = status
	return s.setCurrentLocalInfo(ctx)
}

func (s *SpaceStatus) UpdateLocalStatus(ctx context.Context, status spaceinfo.LocalStatus) error {
	s.LocalStatus = status
	return s.setCurrentLocalInfo(ctx)
}

func (s *SpaceStatus) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	if s.LocalStatus == info.LocalStatus && info.RemoteStatus == s.RemoteStatus {
		return nil
	}
	s.LocalStatus = info.LocalStatus
	s.RemoteStatus = info.RemoteStatus
	return s.setCurrentLocalInfo(ctx)
}

func (s *SpaceStatus) SetPersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) (err error) {
	if s.GetPersistentStatus() == status {
		return nil
	}
	if err = s.techSpace.SetPersistentInfo(ctx, spaceinfo.SpacePersistentInfo{
		SpaceID:       s.SpaceId,
		AccountStatus: status,
	}); err != nil {
		return err
	}
	s.AccountStatus = status
	return nil
}

func (s *SpaceStatus) setCurrentLocalInfo(ctx context.Context) (err error) {
	return s.techSpace.SetLocalInfo(ctx, spaceinfo.SpaceLocalInfo{
		SpaceID:      s.SpaceId,
		LocalStatus:  s.LocalStatus,
		RemoteStatus: s.RemoteStatus,
	})
}

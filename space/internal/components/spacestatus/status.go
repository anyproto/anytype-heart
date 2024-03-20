package spacestatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"

	"github.com/anyproto/anytype-heart/space/internal/techspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.spacestatus"

type SpaceStatus interface {
	app.ComponentRunnable
	sync.Locker
	SpaceId() string
	GetLocalStatus() spaceinfo.LocalStatus
	GetRemoteStatus() spaceinfo.RemoteStatus
	GetPersistentStatus() spaceinfo.AccountStatus
	LatestAclHeadId() string
	UpdatePersistentStatus(ctx context.Context, status spaceinfo.AccountStatus)
	UpdatePersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo)
	SetRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error
	SetPersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) (err error)
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error)
	SetLocalStatus(ctx context.Context, status spaceinfo.LocalStatus) error
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error)
	SetAccessType(ctx context.Context, status spaceinfo.AccessType) (err error)
}

type spaceStatus struct {
	sync.Mutex
	spaceId         string
	accountStatus   spaceinfo.AccountStatus
	localStatus     spaceinfo.LocalStatus
	remoteStatus    spaceinfo.RemoteStatus
	latestAclHeadId string
	techSpace       techspace.TechSpace
	statService     debugstat.StatService
	readLimit       uint32
	writeLimit      uint32
}

func (s *spaceStatus) ProvideStat() any {
	return spaceStatusStat{
		SpaceId:       s.spaceId,
		AccountStatus: s.accountStatus.String(),
		LocalStatus:   s.localStatus.String(),
		RemoteStatus:  s.remoteStatus.String(),
		AclHeadId:     s.latestAclHeadId,
	}
}

func (s *spaceStatus) StatId() string {
	return s.spaceId
}

func (s *spaceStatus) StatType() string {
	return CName
}

func New(spaceId string, accountStatus spaceinfo.AccountStatus, aclHeadId string) SpaceStatus {
	return &spaceStatus{
		accountStatus:   accountStatus,
		spaceId:         spaceId,
		latestAclHeadId: aclHeadId,
	}
}

func (s *spaceStatus) Init(a *app.App) (err error) {
	s.techSpace = a.MustComponent(techspace.CName).(techspace.TechSpace)
	s.statService, _ = a.Component(debugstat.CName).(debugstat.StatService)
	if s.statService == nil {
		s.statService = debugstat.NewNoOp()
	}
	s.statService.AddProvider(s)
	return nil
}

func (s *spaceStatus) Run(ctx context.Context) (err error) {
	return nil
}

func (s *spaceStatus) Close(ctx context.Context) (err error) {
	s.statService.RemoveProvider(s)
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

func (s *spaceStatus) UpdatePersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) {
	s.accountStatus = info.AccountStatus
	s.latestAclHeadId = info.AclHeadId
}

func (s *spaceStatus) LatestAclHeadId() string {
	return s.latestAclHeadId
}

func (s *spaceStatus) SetRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
	s.remoteStatus = status.RemoteStatus
	s.readLimit = status.ReadLimit
	s.writeLimit = status.WriteLimit
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

func (s *spaceStatus) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error) {
	if s.GetPersistentStatus() == info.AccountStatus {
		return nil
	}
	if err = s.techSpace.SetPersistentInfo(ctx, info); err != nil {
		return err
	}
	s.accountStatus = info.AccountStatus
	if info.AclHeadId != "" {
		s.latestAclHeadId = info.AclHeadId
	}
	return nil
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
		ReadLimit:    s.readLimit,
		WriteLimit:   s.writeLimit,
	})
}

func (s *spaceStatus) SetAccessType(ctx context.Context, acc spaceinfo.AccessType) (err error) {
	return s.techSpace.SetAccessType(ctx, s.spaceId, acc)
}

type spaceStatusStat struct {
	SpaceId       string `json:"spaceId"`
	AccountStatus string `json:"accountStatus"`
	LocalStatus   string `json:"localStatus"`
	RemoteStatus  string `json:"remoteStatus"`
	AclHeadId     string `json:"aclHeadId"`
}

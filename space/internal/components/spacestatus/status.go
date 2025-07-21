package spacestatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.components.spacestatus"

type SpaceStatus interface {
	app.ComponentRunnable
	SpaceId() string
	GetLocalStatus() spaceinfo.LocalStatus
	GetPersistentStatus() spaceinfo.AccountStatus
	GetLatestAclHeadId() string
	SetPersistentStatus(status spaceinfo.AccountStatus) (err error)
	SetPersistentInfo(info spaceinfo.SpacePersistentInfo) (err error)
	SetLocalStatus(status spaceinfo.LocalStatus) error
	SetLocalInfo(info spaceinfo.SpaceLocalInfo) (err error)
	SetAccessType(status spaceinfo.AccessType) (err error)
	SetAclInfo(isAclEmpty bool, pushKey crypto.PrivKey, pushEncryptionKey crypto.SymKey) (err error)
	SetOwner(ownerIdentity string, createdDate int64) (err error)
	GetSpaceView() techspace.SpaceView
}

type spaceStatus struct {
	spaceId     string
	techSpace   techspace.TechSpace
	spaceView   techspace.SpaceView
	statService debugstat.StatService
}

func (s *spaceStatus) ProvideStat() any {
	s.spaceView.Lock()
	defer s.spaceView.Unlock()
	localInfo := s.spaceView.GetLocalInfo()
	persistentInfo := s.spaceView.GetPersistentInfo()
	return spaceStatusStat{
		SpaceId:       s.spaceId,
		AccountStatus: persistentInfo.GetAccountStatus().String(),
		AclHeadId:     persistentInfo.GetAclHeadId(),
		LocalStatus:   localInfo.GetLocalStatus().String(),
		RemoteStatus:  localInfo.GetRemoteStatus().String(),
	}
}

func (s *spaceStatus) StatId() string {
	return s.spaceId
}

func (s *spaceStatus) StatType() string {
	return CName
}

func New(spaceId string) SpaceStatus {
	return &spaceStatus{
		spaceId: spaceId,
	}
}

func (s *spaceStatus) Init(a *app.App) (err error) {
	s.techSpace = app.MustComponent[techspace.TechSpace](a)
	s.spaceView, err = s.techSpace.GetSpaceView(context.Background(), s.spaceId)
	if err != nil {
		return err
	}
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
	if s.statService != nil {
		s.statService.RemoveProvider(s)
	}
	return nil
}

func doSpaceView[T any](spaceView techspace.SpaceView, f func(techspace.SpaceView) T) (res T) {
	spaceView.Lock()
	defer spaceView.Unlock()
	return f(spaceView)
}

func (s *spaceStatus) Name() (name string) {
	return CName
}

func (s *spaceStatus) SpaceId() string {
	return s.spaceId
}

func (s *spaceStatus) GetSpaceView() techspace.SpaceView {
	return s.spaceView
}

func (s *spaceStatus) GetLocalStatus() spaceinfo.LocalStatus {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) spaceinfo.LocalStatus {
		info := view.GetLocalInfo()
		return info.GetLocalStatus()
	})
}

func (s *spaceStatus) GetPersistentStatus() spaceinfo.AccountStatus {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) spaceinfo.AccountStatus {
		info := view.GetPersistentInfo()
		return info.GetAccountStatus()
	})
}

func (s *spaceStatus) GetLatestAclHeadId() string {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) string {
		info := view.GetPersistentInfo()
		return info.GetAclHeadId()
	})
}

func (s *spaceStatus) SetLocalStatus(status spaceinfo.LocalStatus) error {
	info := spaceinfo.NewSpaceLocalInfo(s.spaceId)
	info.SetLocalStatus(status)
	return s.SetLocalInfo(info)
}

func (s *spaceStatus) SetOwner(ownerIdentity string, createdDate int64) (err error) {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) error {
		return view.SetOwner(domain.NewParticipantId(s.spaceId, ownerIdentity), createdDate)
	})
}

func (s *spaceStatus) SetLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) error {
		return view.SetSpaceLocalInfo(info)
	})
}

func (s *spaceStatus) SetPersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) error {
		return view.SetSpacePersistentInfo(info)
	})
}

func (s *spaceStatus) SetPersistentStatus(status spaceinfo.AccountStatus) (err error) {
	info := spaceinfo.NewSpacePersistentInfo(s.spaceId)
	info.SetAccountStatus(status)
	return s.SetPersistentInfo(info)
}

func (s *spaceStatus) SetAccessType(acc spaceinfo.AccessType) (err error) {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) error {
		return view.SetAccessType(acc)
	})
}

func (s *spaceStatus) SetAclInfo(isAclEmpty bool, pushKey crypto.PrivKey, pushEncryptionKey crypto.SymKey) (err error) {
	return doSpaceView(s.spaceView, func(view techspace.SpaceView) error {
		return view.SetAclInfo(isAclEmpty, pushKey, pushEncryptionKey)
	})
}

type spaceStatusStat struct {
	SpaceId       string `json:"spaceId"`
	AccountStatus string `json:"accountStatus"`
	LocalStatus   string `json:"localStatus"`
	RemoteStatus  string `json:"remoteStatus"`
	AclHeadId     string `json:"aclHeadId"`
}

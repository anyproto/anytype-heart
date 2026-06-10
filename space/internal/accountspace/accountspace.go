// Package accountspace provides the single SpaceController implementation for
// all account spaces (personal, shareable, streamable, one-to-one). Kind
// differences are expressed as Descriptor data, not separate controller types.
package accountspace

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/joiner"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/offloader"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var log = logger.NewNamed("common.space.accountspace")

// Descriptor carries everything kind-specific about a space controller.
type Descriptor struct {
	SpaceId string
	// Status seeds lastUpdatedStatus so the first Update() after Start()
	// doesn't re-run the transition Start() already performed.
	Status spaceinfo.AccountStatus
	// IsPersonal makes the loader stop on mandatory-objects failure and is
	// reported to spaceloader (stopIfMandatoryFail).
	IsPersonal bool
	// GuestKey, when set, makes the space load with a guest signing key
	// (streamable spaces).
	GuestKey      crypto.PrivKey
	OwnerMetadata []byte
	// ExtraLoaderComponents returns components registered into each loading
	// app in addition to the standard set. Called once per loading transition
	// so components are always fresh (e.g. personalmigration).
	ExtraLoaderComponents func() []app.Component
}

type statusUpdater interface {
	UpdateCoordinatorStatus()
}

type spaceController struct {
	spaceId           string
	desc              Descriptor
	app               *app.App
	status            spacestatus.SpaceStatus
	lastUpdatedStatus spaceinfo.AccountStatus
	updater           statusUpdater
	mx                sync.Mutex

	sm *mode.StateMachine
}

func makeStatusApp(a *app.App, spaceId string) (*app.App, error) {
	newApp := a.ChildApp()
	newApp.Register(spacestatus.New(spaceId))
	err := newApp.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return newApp, nil
}

func NewSpaceController(desc Descriptor, a *app.App) (spacecontroller.SpaceController, error) {
	newApp, err := makeStatusApp(a, desc.SpaceId)
	if err != nil {
		return nil, err
	}
	s := &spaceController{
		spaceId:           desc.SpaceId,
		desc:              desc,
		status:            newApp.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus),
		lastUpdatedStatus: desc.Status,
		app:               newApp,
	}

	// this is done for tests to not complicate them :-)
	if updater, ok := a.Component(deletioncontroller.CName).(statusUpdater); ok {
		s.updater = updater
	}
	sm, err := mode.NewStateMachine(s, log.With(zap.String("spaceId", desc.SpaceId)))
	if err != nil {
		return nil, err
	}
	s.sm = sm
	return s, nil
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

func (s *spaceController) Start(ctx context.Context) error {
	defer func() {
		if s.updater != nil {
			s.updater.UpdateCoordinatorStatus()
		}
	}()
	_, err := s.sm.ChangeMode(modeOfStatus(s.status.GetPersistentStatus()))
	return err
}

func (s *spaceController) Mode() mode.Mode {
	return s.sm.GetMode()
}

func (s *spaceController) Current() any {
	return s.sm.GetProcess()
}

func (s *spaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	err := s.status.SetPersistentInfo(info)
	if err != nil {
		return err
	}
	return s.Update()
}

func (s *spaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return s.status.SetLocalInfo(info)
}

func (s *spaceController) Update() error {
	s.mx.Lock()
	status := s.status.GetPersistentStatus()
	if s.lastUpdatedStatus == status {
		s.mx.Unlock()
		return nil
	}
	s.lastUpdatedStatus = status
	s.mx.Unlock()
	_, err := s.sm.ChangeMode(modeOfStatus(status))
	return err
}

// modeOfStatus is the single AccountStatus -> Mode mapping for all space kinds.
func modeOfStatus(status spaceinfo.AccountStatus) mode.Mode {
	switch status {
	case spaceinfo.AccountStatusDeleted, spaceinfo.AccountStatusRemoving:
		return mode.ModeOffloading
	case spaceinfo.AccountStatusJoining:
		return mode.ModeJoining
	default:
		return mode.ModeLoading
	}
}

func (s *spaceController) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeLoading:
		var extraComps []app.Component
		if s.desc.ExtraLoaderComponents != nil {
			extraComps = s.desc.ExtraLoaderComponents()
		}
		return loader.New(s.app, loader.Params{
			SpaceId:         s.spaceId,
			IsPersonal:      s.desc.IsPersonal,
			OwnerMetadata:   s.desc.OwnerMetadata,
			GuestKey:        s.desc.GuestKey,
			AdditionalComps: extraComps,
		})
	case mode.ModeOffloading:
		return offloader.New(s.app)
	case mode.ModeJoining:
		return joiner.New(s.app, joiner.Params{
			SpaceId: s.spaceId,
			Status:  s.status,
			Log:     log,
		})
	default:
		return initial.New()
	}
}

func (s *spaceController) Close(ctx context.Context) error {
	s.sm.Close()
	// this closes status
	return s.app.Close(ctx)
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return s.status.GetPersistentStatus()
}

func (s *spaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return s.status.GetLocalStatus()
}

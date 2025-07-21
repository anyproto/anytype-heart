package personalspace

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/internal/components/personalmigration"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/offloader"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type Personal interface {
	spacecontroller.SpaceController
	WaitMigrations(ctx context.Context) error
}

var log = logger.NewNamed("common.space.personalspace")

type ctxKey int

const SkipCheckSpaceViewKey ctxKey = iota

func shouldCheckSpaceView(ctx context.Context) bool {
	skip, ok := ctx.Value(SkipCheckSpaceViewKey).(bool)
	return !ok || !skip
}

func NewSpaceController(ctx context.Context, spaceId string, metadata []byte, a *app.App) (spacecontroller.SpaceController, error) {
	techSpace := a.MustComponent(techspace.CName).(techspace.TechSpace)
	spaceCore := a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	var (
		exists bool
		err    error
	)
	if shouldCheckSpaceView(ctx) {
		exists, err = techSpace.SpaceViewExists(ctx, spaceId)
	}
	// This could happen for old accounts
	if !exists || err != nil {
		info := spaceinfo.NewSpacePersistentInfo(spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		err = techSpace.SpaceViewCreate(ctx, spaceId, false, info, nil)
		if err != nil {
			return nil, err
		}
	}
	newApp, err := makeStatusApp(a, spaceId)
	if err != nil {
		return nil, err
	}
	s := &spaceController{
		app:       newApp,
		spaceId:   spaceId,
		techSpace: techSpace,
		status:    newApp.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus),
		spaceCore: spaceCore,
		metadata:  metadata,
	}
	sm, err := mode.NewStateMachine(s, log.With(zap.String("spaceId", s.spaceId)))
	if err != nil {
		return nil, err
	}
	s.sm = sm
	return s, nil
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

type spaceController struct {
	app               *app.App
	spaceId           string
	metadata          []byte
	lastUpdatedStatus spaceinfo.AccountStatus

	loader    loader.Loader
	spaceCore spacecore.SpaceCoreService
	techSpace techspace.TechSpace
	status    spacestatus.SpaceStatus

	personalMigration personalmigration.Runner

	sm *mode.StateMachine
	mx sync.Mutex
}

func (s *spaceController) Start(ctx context.Context) (err error) {
	switch s.status.GetPersistentStatus() {
	case spaceinfo.AccountStatusDeleted:
		_, err := s.sm.ChangeMode(mode.ModeOffloading)
		return err
	default:
		_, err := s.sm.ChangeMode(mode.ModeLoading)
		return err
	}
}

func (s *spaceController) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeInitial:
		return initial.New()
	case mode.ModeOffloading:
		return offloader.New(s.app)
	default:
		return &personalLoader{
			newLoader: s.newLoader,
		}
	}
}

func (s *spaceController) Mode() mode.Mode {
	return s.sm.GetMode()
}

func (s *spaceController) Current() any {
	return s.sm.GetProcess()
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

func (s *spaceController) newLoader() loader.Loader {
	s.mx.Lock()
	s.personalMigration = personalmigration.New()
	s.mx.Unlock()
	return loader.New(s.app, loader.Params{
		SpaceId:       s.spaceId,
		IsPersonal:    true,
		OwnerMetadata: s.metadata,
		AdditionalComps: []app.Component{
			s.personalMigration,
		},
	})
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
	updateStatus := func(mode mode.Mode) error {
		_, err := s.sm.ChangeMode(mode)
		return err
	}
	switch status {
	case spaceinfo.AccountStatusDeleted:
		return updateStatus(mode.ModeOffloading)
	default:
		return updateStatus(mode.ModeLoading)
	}
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

func (s *spaceController) Delete(ctx context.Context) error {
	offloading, err := s.sm.ChangeMode(mode.ModeOffloading)
	if err != nil {
		return err
	}
	of := offloading.(offloader.Offloader)
	return of.WaitOffload(ctx)
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

func (s *spaceController) WaitMigrations(ctx context.Context) error {
	s.mx.Lock()
	if s.personalMigration == nil {
		s.mx.Unlock()
		return nil
	}
	s.mx.Unlock()
	return s.personalMigration.WaitProfile(ctx)
}

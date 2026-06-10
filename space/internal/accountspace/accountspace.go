// Package accountspace provides the single SpaceController implementation for
// all account spaces (personal, shareable, streamable, one-to-one). Kind
// differences are expressed as Descriptor data, not separate controller types.
package accountspace

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/clientspace"
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
	spaceId string
	desc    Descriptor
	app     *app.App
	status  spacestatus.SpaceStatus
	updater statusUpdater

	rec *reconciler
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
		spaceId: desc.SpaceId,
		desc:    desc,
		status:  newApp.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus),
		app:     newApp,
	}

	// this is done for tests to not complicate them :-)
	if updater, ok := a.Component(deletioncontroller.CName).(statusUpdater); ok {
		s.updater = updater
	}
	s.rec = newReconciler(s, log.With(zap.String("spaceId", desc.SpaceId)))
	// seed the desired status so deletion/joining converge in the background
	// even when the space is never demanded (deletion outranks demand)
	s.rec.setStatus(s.status.GetPersistentStatus())
	return s, nil
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

// Start demands the space and blocks until the reconciler converges on the
// first target (loading/joining/offloading started), returning the real
// transition error on failure.
func (s *spaceController) Start(ctx context.Context) error {
	defer func() {
		if s.updater != nil {
			s.updater.UpdateCoordinatorStatus()
		}
	}()
	s.rec.setInputs(s.status.GetPersistentStatus(), true)
	_, err := s.rec.waitConverged(ctx)
	return err
}

// Demand marks the space as wanted-loaded; loading happens in the background.
func (s *spaceController) Demand() {
	s.rec.setDemand()
}

// WaitLoad demands the space and blocks until it is fully loaded. It fails
// with the real load error, with ErrModeUnreachable when the space status
// dictates another mode (offloading/joining), or on ctx/close.
func (s *spaceController) WaitLoad(ctx context.Context) (clientspace.Space, error) {
	s.rec.setDemand()
	proc, err := s.rec.waitMode(ctx, mode.ModeLoading)
	if err != nil {
		return nil, err
	}
	ld, ok := proc.(loader.LoadWaiter)
	if !ok {
		return nil, fmt.Errorf("loading process does not support WaitLoad")
	}
	return ld.WaitLoad(ctx)
}

func (s *spaceController) Mode() mode.Mode {
	return s.rec.getMode()
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

// Update pushes the latest persistent status into the reconciler. It never
// blocks on the resulting transition; convergence is observed via Wait*.
func (s *spaceController) Update() error {
	s.rec.setStatus(s.status.GetPersistentStatus())
	return nil
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
	s.rec.close(ctx)
	// this closes status
	return s.app.Close(ctx)
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return s.status.GetPersistentStatus()
}

func (s *spaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return s.status.GetLocalStatus()
}

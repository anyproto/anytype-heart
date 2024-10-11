package syncstopper

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/periodicsync"

	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	space "github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "common.components.syncstopper"

const (
	timeout          = 30 * time.Second
	loopIntervalSecs = 30
	stopSyncTimeout  = 10 * time.Minute
)

var log = logger.NewNamed(CName)

type SyncStopper struct {
	spaceCore    space.SpaceCoreService
	spaceStatus  spacestatus.SpaceStatus
	periodicCall periodicsync.PeriodicSync
	startTime    time.Time
}

func New() *SyncStopper {
	return &SyncStopper{}
}

func (s *SyncStopper) Init(a *app.App) (err error) {
	s.spaceCore = a.MustComponent(space.CName).(space.SpaceCoreService)
	s.spaceStatus = a.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus)
	return
}

func (s *SyncStopper) Name() (name string) {
	return CName
}

func (s *SyncStopper) Run(ctx context.Context) (err error) {
	s.startTime = time.Now()
	s.periodicCall = periodicsync.NewPeriodicSync(loopIntervalSecs, timeout, s.spaceCheck, log)
	return nil
}

func (s *SyncStopper) spaceCheck(ctx context.Context) (err error) {
	sp, err := s.spaceCore.Pick(ctx, s.spaceStatus.SpaceId())
	if err != nil {
		return
	}
	if time.Since(s.startTime) > stopSyncTimeout {
		sp.TreeSyncer().StopSync()
	}
	return nil
}

func (s *SyncStopper) Close(ctx context.Context) (err error) {
	if s.periodicCall != nil {
		s.periodicCall.Close()
	}
	return nil
}

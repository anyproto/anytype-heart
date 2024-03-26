package joiner

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const statusChangerCName = "joiner.statuschanger"

type statusChanger struct {
	status spacestatus.SpaceStatus
}

func newStatusChanger() *statusChanger {
	return &statusChanger{}
}

func (s *statusChanger) Init(a *app.App) (err error) {
	s.status = a.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus)
	return nil
}

func (s *statusChanger) Name() (name string) {
	return statusChangerCName
}

func (s *statusChanger) Run(ctx context.Context) (err error) {
	s.status.Lock()
	defer s.status.Unlock()
	return s.status.SetLocalStatus(ctx, spaceinfo.LocalStatusUnknown)
}

func (s *statusChanger) Close(ctx context.Context) (err error) {
	return nil
}

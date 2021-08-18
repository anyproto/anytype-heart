package listener

import (
	"context"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

const CName = "block/listener"

var log = logging.Logger("anytype-mw-block-listener")

func New() Listener {
	return &listener{}
}

type OnDocChangeCallback func(ctx context.Context, id string, s *state.State) error

type StateChange struct {
	State  *state.State
	Events []*pb.EventMessage
}

type Listener interface {
	ReportChange(ctx context.Context, id string, s *state.State)
	OnWholeChange(cb OnDocChangeCallback)
	app.Component
}

type listener struct {
	wholeCallbacks []OnDocChangeCallback

	m sync.RWMutex
}

func (l *listener) Init(a *app.App) (err error) {
	return
}

func (l *listener) Name() (name string) {
	return CName
}

func (l *listener) ReportChange(ctx context.Context, id string, s *state.State) {
	l.m.RLock()
	defer l.m.RUnlock()
	for _, cb := range l.wholeCallbacks {
		if err := cb(ctx, id, s); err != nil {
			log.Errorf("state change callback error: %v", err)
		}
	}
}

func (l *listener) OnWholeChange(cb OnDocChangeCallback) {
	l.m.Lock()
	defer l.m.Unlock()
	l.wholeCallbacks = append(l.wholeCallbacks, cb)
}

package profiler

import (
	"context"

	"github.com/anyproto/any-sync/app"

	debug2 "github.com/anyproto/anytype-heart/core/debug"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("profiler")

type Service interface {
	app.ComponentRunnable
}

type service struct {
	closeCh      chan struct{}
	debugService debug2.Debug

	timesHighMemoryUsageDetected int
	previousHighMemoryDetected   uint64
}

func New() Service {
	return &service{
		closeCh: make(chan struct{}),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.debugService = app.MustComponent[debug2.Debug](a)
	return nil
}

func (s *service) Name() (name string) {
	return "profiler"
}

func (s *service) Run(ctx context.Context) (err error) {
	go s.run()

	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closeCh)
	return nil
}

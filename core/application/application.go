package application

import (
	"context"
	"errors"
	"runtime/trace"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.RWMutex

	app *app.App

	mnemonic string

	// memoized private key derived from mnemonic, used for signing session tokens
	sessionSigningKey []byte

	rootPath          string
	clientWithVersion string
	eventSender       event.Sender
	sessions          session.Service
	traceRecorder     *traceRecorder

	appAccountStartInProcessCancel      context.CancelFunc
	appAccountStartInProcessCancelMutex sync.Mutex
}

func New() *Service {
	return &Service{
		sessions:      session.New(),
		traceRecorder: &traceRecorder{},
	}
}

func (s *Service) GetApp() *app.App {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.app
}

func (s *Service) requireClientWithVersion() {
	if s.clientWithVersion == "" {
		panic(errors.New("client platform with the version must be set using the MetricsSetParameters method"))
	}
}

func (s *Service) Stop() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.stop()
}

func (s *Service) stop() error {
	ctx, task := trace.NewTask(context.Background(), "application.stop")
	defer task.End()

	if s != nil && s.app != nil {
		err := s.app.Close(ctx)
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
		}

		s.app = nil
	}
	return nil
}

func (s *Service) SetClientVersion(platform string, version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.clientWithVersion = platform + ":" + version
}

func (s *Service) SetEventSender(sender event.Sender) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.eventSender = sender
}

func (s *Service) GetEventSender() event.Sender {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.eventSender
}

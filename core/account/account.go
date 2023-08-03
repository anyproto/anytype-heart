package account

import (
	"sync"
	"github.com/anyproto/any-sync/app"
	"context"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"errors"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.RWMutex

	app *app.App

	mnemonic string

	// memoized private key derived from mnemonic, used for signing session tokens
	sessionKey []byte

	rootPath          string
	clientWithVersion string
	eventSender       event.Sender

	sessions session.Service
}

func New() *Service {
	return &Service{
		sessions: session.New(),
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
	if s != nil && s.app != nil {
		err := s.app.Close(context.Background())
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

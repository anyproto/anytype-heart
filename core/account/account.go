package account

import (
	"sync"
	"github.com/anyproto/any-sync/app"
	"context"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"errors"
	"github.com/anyproto/anytype-heart/core/event"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.Mutex

	app               *app.App
	mnemonic          string
	rootPath          string
	clientWithVersion string
	eventSender       event.Sender
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

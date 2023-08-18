package application

import (
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/localdiscovery"
)

func (s *Service) EnableLocalNetworkSync() error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.app == nil {
		return ErrApplicationIsNotRunning
	}
	localDiscoveryService := app.MustComponent[localdiscovery.LocalDiscovery](s.app)
	return localDiscoveryService.Start()
}

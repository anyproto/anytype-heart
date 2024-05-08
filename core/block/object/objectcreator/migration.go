package objectcreator

import (
	"context"

	"github.com/anyproto/anytype-heart/core/migration"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) RunMigrations(ctx context.Context, space clientspace.Space) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelMigrations = append(s.cancelMigrations, cancel)
	go migration.Run(ctx, s.objectStore, space)
}

func (s *service) Run(context.Context) error {
	return nil
}

func (s *service) Close(context.Context) error {
	for _, cancel := range s.cancelMigrations {
		cancel()
	}
	return nil
}

package objectcreator

import (
	"context"

	"github.com/anyproto/anytype-heart/core/migration"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) RunMigrations(space clientspace.Space) {
	go migration.Run(s.ctxMigration, s.objectStore, space)
}

func (s *service) Run(context.Context) error {
	return nil
}

func (s *service) Close(context.Context) error {
	s.cancelMigration()
	return nil
}

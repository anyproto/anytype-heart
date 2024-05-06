package objectcreator

import (
	"github.com/anyproto/anytype-heart/core/migration"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) RunMigrations(space clientspace.Space) {
	go migration.Run(s.objectStore, space)
}

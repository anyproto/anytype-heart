package dependencies

import (
	"github.com/anyproto/anytype-heart/space/clientspace"
)

type SpaceMigrationRunner interface {
	RunMigrations(space clientspace.Space)
}

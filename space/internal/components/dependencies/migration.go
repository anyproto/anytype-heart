package dependencies

import (
	"context"

	"github.com/anyproto/anytype-heart/space/clientspace"
)

type SpaceMigrationRunner interface {
	RunMigrations(ctx context.Context, space clientspace.Space)
}

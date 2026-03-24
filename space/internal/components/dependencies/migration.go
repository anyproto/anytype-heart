package dependencies

import (
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

type MigrationService interface {
	RunMigrationsWhenIdle(spaceId string, derivedIDs threads.DerivedSmartblockIds)
}

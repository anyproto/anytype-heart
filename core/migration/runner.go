package migration

import (
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const CName = "migration-runner"

var log = logging.Logger(CName)

type Migration interface {
	Run(objectstore.ObjectStore, clientspace.Space) (toMigrate, migrated int, err error)
	Name() string
}

func Run(store objectstore.ObjectStore, space clientspace.Space) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic while running space migrations: %v", r)
		}
	}()

	for _, m := range []Migration{
		systemObjectReviser{},
		readonlyRelationsFixer{},
	} {
		toMigrate, migrated, err := m.Run(store, space)
		if err != nil {
			log.Errorf("failed to run migration '%s' in space '%s': %v. %d out of %d objects were migrated",
				m.Name(), space.Id(), err, migrated, toMigrate)
			continue
		}
		log.Debugf("migration '%s' in space '%s' is successful.  %d out of %d objects were migrated",
			m.Name(), space.Id(), migrated, toMigrate)
	}
}

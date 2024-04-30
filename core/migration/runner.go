package migration

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName   = "migration-runner"
	timeout = 30 * time.Second
)

var log = logging.Logger(CName)

type Migration interface {
	Run(objectstore.ObjectStore, clientspace.Space) (toMigrate, migrated int, err error)
	Name() string
}

type Runner struct {
	store       objectstore.ObjectStore
	spaceGetter space.Service

	migrations []Migration

	app.ComponentRunnable
}

func New() *Runner {
	return &Runner{}
}

func (r *Runner) Name() string {
	return CName
}

func (r *Runner) Init(a *app.App) error {
	r.spaceGetter = app.MustComponent[space.Service](a)
	r.store = app.MustComponent[objectstore.ObjectStore](a)

	r.migrations = []Migration{
		readonlyRelationsFixer{},
		systemObjectReviser{},
	}

	return nil
}

func (r *Runner) Run(context.Context) error {
	go r.run()
	return nil
}

func (r *Runner) run() {
	// wait until spaces come up
	time.Sleep(timeout)

	spaces, err := r.listSpaceIds()
	if err != nil {
		log.Errorf("failed to list spaces for performing migrations: %v", err)
		return
	}

	for _, spaceId := range spaces {
		spc, err := r.spaceGetter.Get(context.Background(), spaceId)
		if err != nil {
			log.Errorf("failed to get space '%s': %v", spaceId, err)
			continue
		}

		for _, migration := range r.migrations {
			toMigrate, migrated, err := migration.Run(r.store, spc)
			if err != nil {
				log.Errorf("failed to run migration '%s' in space '%s': %v. %d out of %d objects were migrated",
					migration.Name(), spc.Id(), err, migrated, toMigrate)
				continue
			}
			log.Debugf("migration '%s' in space '%s' is successful.  %d out of %d objects were migrated",
				migration.Name(), spc.Id(), migrated, toMigrate)
		}
	}

	return
}

func (r *Runner) Close(context.Context) error {
	return nil
}

func (r *Runner) listSpaceIds() (ids []string, err error) {
	records, _, err := r.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		ids = append(ids, pbtypes.GetString(record.Details, bundle.RelationKeyTargetSpaceId.String()))
	}
	return
}

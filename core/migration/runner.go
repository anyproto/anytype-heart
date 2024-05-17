package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/migration/common"
	"github.com/anyproto/anytype-heart/core/migration/readonlyfixer"
	"github.com/anyproto/anytype-heart/core/migration/systemobjectreviser"
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
	CName     = "migration-runner"
	errFormat = "failed to run migration '%s' in space '%s': %w. %d out of %d objects were migrated"
)

var log = logging.Logger(CName)

type Migration interface {
	Run(context.Context, common.StoreWithCtx, common.SpaceWithCtx) (toMigrate, migrated int, err error)
	Name() string
}

func New() *Runner {
	return &Runner{}
}

type Runner struct {
	store        objectstore.ObjectStore
	spaceService space.Service

	ctx    context.Context
	cancel context.CancelFunc

	app.ComponentRunnable
}

func (r *Runner) Name() string {
	return CName
}

func (r *Runner) Init(a *app.App) error {
	r.store = app.MustComponent[objectstore.ObjectStore](a)
	r.spaceService = app.MustComponent[space.Service](a)
	r.ctx, r.cancel = context.WithCancel(context.Background())
	return nil
}

func (r *Runner) Run(context.Context) error {
	go r.runMigrations()
	return nil
}

func (r *Runner) Close(context.Context) error {
	r.cancel()
	return nil
}

func (r *Runner) runMigrations() {
	spaceViews, err := r.store.QueryWithContext(r.ctx, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.ObjectType_spaceView)),
			},
		},
	})

	if err != nil {
		log.Errorf("failed to get space views from store: %v", err)
		return
	}

	for _, view := range spaceViews {
		spaceId := pbtypes.GetString(view.Details, bundle.RelationKeyTargetSpaceId.String())
		spc, err := r.spaceService.Get(r.ctx, spaceId)
		if err != nil {
			log.Errorf("failed to get space %s: %s", spaceId, err.Error())
			if errors.Is(err, context.Canceled) {
				return
			}
			continue
		}

		if err = r.runForSpace(spc); err != nil {
			log.Errorf("failed to run default migrations for space %s: %s", spaceId, err.Error())
			if errors.Is(err, context.Canceled) {
				return
			}
			continue
		}
	}
}

func (r *Runner) runForSpace(space clientspace.Space) error {
	return r.run(r.ctx, space,
		systemobjectreviser.Migration{},
		readonlyfixer.Migration{},
	)
}

func (r *Runner) run(ctx context.Context, space clientspace.Space, migrations ...Migration) (mErr error) {
	spaceId := space.Id()

	for _, m := range migrations {
		toMigrate, migrated, err := m.Run(ctx, r.store, space)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			fErr := fmt.Errorf(errFormat, m.Name(), spaceId, err, migrated, toMigrate)
			log.Error(fErr)
			mErr = multierror.Append(mErr, fErr)
			continue
		}
		log.Debugf("migration '%s' in space '%s' is successful. %d out of %d objects were migrated",
			m.Name(), spaceId, migrated, toMigrate)
	}
	return
}

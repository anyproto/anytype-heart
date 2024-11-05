package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/readonlyfixer"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/systemobjectreviser"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
)

const (
	CName     = "common.components.migration"
	errFormat = "failed to run migration '%s' in space '%s': %w. %d out of %d objects were migrated"
)

var log = logger.NewNamed(CName)

type Migration interface {
	Run(context.Context, logger.CtxLogger, objectstore.ObjectStore, dependencies.SpaceWithCtx) (toMigrate, migrated int, err error)
	Name() string
}

func New() *Runner {
	return &Runner{}
}

type Runner struct {
	store       objectstore.ObjectStore
	spaceLoader spaceloader.SpaceLoader

	ctx      context.Context
	cancel   context.CancelFunc
	spc      clientspace.Space
	loadErr  error
	waitLoad chan struct{}
	started  bool

	app.ComponentRunnable
}

func (r *Runner) Name() string {
	return CName
}

func (r *Runner) Init(a *app.App) error {
	r.store = app.MustComponent[objectstore.ObjectStore](a)
	r.spaceLoader = app.MustComponent[spaceloader.SpaceLoader](a)
	r.waitLoad = make(chan struct{})
	return nil
}

func (r *Runner) Run(context.Context) error {
	r.started = true
	r.ctx, r.cancel = context.WithCancel(context.Background())
	go r.waitSpace()
	go r.runMigrations()
	return nil
}

func (r *Runner) Close(context.Context) error {
	if r.started {
		r.cancel()
	}
	return nil
}

func (r *Runner) waitSpace() {
	r.spc, r.loadErr = r.spaceLoader.WaitLoad(r.ctx)
	close(r.waitLoad)
}

func (r *Runner) runMigrations() {
	select {
	case <-r.ctx.Done():
		return
	case <-r.waitLoad:
		if r.loadErr != nil {
			log.Error("failed to load space", zap.Error(r.loadErr))
			return
		}
		break
	}

	migrations := []Migration{
		systemobjectreviser.Migration{},
		readonlyfixer.Migration{},
	}

	if err := r.run(migrations...); err != nil {
		log.Error("failed to run default migrations", zap.String("spaceId", r.spc.Id()), zap.Error(err))
	}
}

func (r *Runner) run(migrations ...Migration) (err error) {
	spaceId := r.spc.Id()

	for _, m := range migrations {
		if e := r.ctx.Err(); e != nil {
			err = errors.Join(err, e)
			return
		}
		toMigrate, migrated, e := m.Run(r.ctx, log, r.store, r.spc)
		if e != nil {
			err = errors.Join(err, wrapError(e, m.Name(), spaceId, migrated, toMigrate))
			continue
		}
		log.Debug(fmt.Sprintf("migration '%s' in space '%s' is successful. %d out of %d objects were migrated",
			m.Name(), spaceId, migrated, toMigrate))
	}
	return
}

func wrapError(err error, migrationName, spaceId string, migrated, toMigrate int) error {
	return fmt.Errorf(errFormat, migrationName, spaceId, err, migrated, toMigrate)
}

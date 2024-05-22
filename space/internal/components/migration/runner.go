package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/common"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/readonlyfixer"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/systemobjectreviser"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
)

const (
	CName     = "common.components.migration-runner"
	errFormat = "failed to run migration '%s' in space '%s': %w. %d out of %d objects were migrated"
)

var log = logger.NewNamed(CName)

type Migration interface {
	Run(context.Context, common.StoreWithCtx, common.SpaceWithCtx) (toMigrate, migrated int, err error)
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

	if err := r.run(systemobjectreviser.Migration{}, readonlyfixer.Migration{}); err != nil {
		log.Error("failed to run default migrations", zap.String("spaceId", r.spc.Id()), zap.Error(err))
	}
}

func (r *Runner) run(migrations ...Migration) (mErr error) {
	spaceId := r.spc.Id()

	for _, m := range migrations {
		toMigrate, migrated, err := m.Run(r.ctx, r.store, r.spc)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			mErr = multierror.Append(mErr, fmt.Errorf(errFormat, m.Name(), spaceId, err, migrated, toMigrate))
			continue
		}
		log.Debug(fmt.Sprintf("migration '%s' in space '%s' is successful. %d out of %d objects were migrated",
			m.Name(), spaceId, migrated, toMigrate))
	}
	return
}

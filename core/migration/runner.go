package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const (
	loggerName = "migration-runner"
	errFormat  = "failed to run migration '%s' in space '%s': %w. %d out of %d objects were migrated"
)

var log = logging.Logger(loggerName)

type doableViaContext interface {
	DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error
	Id() string
}

type Migration interface {
	Run(context.Context, queryableStore, doableViaContext) (toMigrate, migrated int, err error)
	Name() string
}

func Run(ctx context.Context, store objectstore.ObjectStore, space clientspace.Space) {
	if err := run(ctx, store, space,
		systemObjectReviser{},
		readonlyRelationsFixer{},
	); err != nil {
		log.Errorf("failed to run default migrations: %v", err)
	}
}

func run(ctx context.Context, store objectstore.ObjectStore, space clientspace.Space, migrations ...Migration) (mErr error) {
	var (
		spaceId       = space.Id()
		finish        = make(chan struct{})
		lockableStore = &storeWithLock{store: store}
	)

	go func() {
		for {
			select {
			case <-ctx.Done():
				lockableStore.Lock()
				return
			case <-finish:
				return
			}
		}
	}()

	for _, m := range migrations {
		toMigrate, migrated, err := m.Run(ctx, lockableStore, space)
		if err != nil {
			fErr := fmt.Errorf(errFormat, m.Name(), spaceId, err, migrated, toMigrate)
			mErr = multierror.Append(mErr, fErr)
			log.Error(fErr)
			if errors.Is(err, context.Canceled) {
				return
			}
			continue
		}
		log.Debugf("migration '%s' in space '%s' is successful. %d out of %d objects were migrated",
			m.Name(), spaceId, migrated, toMigrate)
	}
	finish <- struct{}{}
	return
}

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/readonlyfixer"
	"github.com/anyproto/anytype-heart/space/internal/components/migration/systemobjectreviser"
)

func TestRunner(t *testing.T) {
	// TODO: we should revive this test when context query for ObjectStore will be implemented
	// t.Run("context exceeds + store operation in progress -> context.Canceled", func(t *testing.T) {
	// 	// given
	// 	store := objectstore.NewStoreFixture(t)
	// 	ctx, cancel := context.WithCancel(context.Background())
	// 	space := mock_space.NewMockSpace(t)
	// 	space.EXPECT().Id().Times(1).Return("")
	// 	runner := Runner{ctx: ctx, store: store, spc: space}
	//
	// 	// when
	// 	go func() {
	// 		time.Sleep(10 * time.Millisecond)
	// 		cancel()
	// 	}()
	// 	err := runner.run(longStoreMigration{})
	//
	// 	// then
	// 	assert.Error(t, err)
	// 	assert.True(t, errors.Is(err, context.Canceled))
	// })

	t.Run("context exceeds + space operation in progress -> context.Canceled", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "spaceId", []spaceindex.TestObject{})
		store.AddObjects(t, addr.AnytypeMarketplaceWorkspace, []spaceindex.TestObject{})
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ string, _ func(smartblock.SmartBlock) error) error {
				timer := time.NewTimer(1 * time.Millisecond)
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-timer.C:
					return nil
				}
			},
		)
		runner := Runner{ctx: ctx, spc: space, store: store}

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := runner.run(longSpaceMigration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("context exceeds + migration is finished -> no error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "spaceId", []spaceindex.TestObject{})
		store.AddObjects(t, addr.AnytypeMarketplaceWorkspace, []spaceindex.TestObject{})
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("spaceId")
		runner := Runner{ctx: ctx, store: store, spc: space}

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := runner.run(instantMigration{})

		// then
		assert.NoError(t, err)
	})

	t.Run("no ctx exceed + migration is finished -> no error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Return("spaceId").Maybe()
		runner := Runner{ctx: context.Background(), store: store, spc: space}

		// when
		err := runner.run(systemobjectreviser.Migration{})

		// then
		assert.NoError(t, err)
	})

	t.Run("no ctx exceed + migration failure -> error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeySpaceId:               domain.String("space1"),
			bundle.RelationKeyRelationFormat:        domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyId:                    domain.String("rel-tag"),
			bundle.RelationKeyRelationReadonlyValue: domain.Bool(true),
		}})
		spaceErr := errors.New("failed to get object")
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Return("space1").Maybe()
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Maybe().Return(spaceErr)
		runner := Runner{ctx: context.Background(), store: store, spc: space}

		// when
		err := runner.run(readonlyfixer.Migration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, spaceErr))
	})
}

type longStoreMigration struct{}

func (m longStoreMigration) Name() string {
	return "long migration"
}

func (m longStoreMigration) Run(ctx context.Context, _ logger.CtxLogger, store, queryableStore dependencies.QueryableStore, _ dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	for {
		if _, err = store.Query(database.Query{}); err != nil {
			return 0, 0, err
		}
	}
}

type longSpaceMigration struct{}

func (m longSpaceMigration) Name() string {
	return "long migration"
}

func (m longSpaceMigration) Run(ctx context.Context, _ logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	for {
		if err = space.DoCtx(ctx, "", func(smartblock.SmartBlock) error {
			// do smth
			return nil
		}); err != nil {
			return 0, 0, err
		}
	}
}

type instantMigration struct{}

func (m instantMigration) Name() string {
	return "instant migration"
}

func (m instantMigration) Run(context.Context, logger.CtxLogger, dependencies.QueryableStore, dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	return 0, 0, nil
}

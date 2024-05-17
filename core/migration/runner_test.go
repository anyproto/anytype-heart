package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/migration/common"
	"github.com/anyproto/anytype-heart/core/migration/readonlyfixer"
	"github.com/anyproto/anytype-heart/core/migration/systemobjectreviser"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestRunner(t *testing.T) {
	t.Run("context exceeds + store operation in progress -> context.Canceled", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("")
		runner := Runner{ctx: ctx, store: store}

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := runner.run(ctx, space, longStoreMigration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("context exceeds + space operation in progress -> context.Canceled", func(t *testing.T) {
		// given
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
		runner := Runner{ctx: ctx}

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := runner.run(ctx, space, longSpaceMigration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("context exceeds + migration is finished -> no error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("")
		runner := Runner{ctx: ctx, store: store}

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := runner.run(ctx, space, instantMigration{})

		// then
		assert.NoError(t, err)
	})

	t.Run("no ctx exceed + migration is finished -> no error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Return("").Maybe()
		runner := Runner{store: store}

		// when
		err := runner.run(context.Background(), space, systemobjectreviser.Migration{})

		// then
		assert.NoError(t, err)
	})

	t.Run("no ctx exceed + migration failure -> error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:               pbtypes.String("space1"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyId:                    pbtypes.String("rel-tag"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(true),
		}})
		spaceErr := errors.New("failed to get object")
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Return("space1").Maybe()
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Maybe().Return(spaceErr)
		runner := Runner{store: store}

		// when
		err := runner.run(context.Background(), space, readonlyfixer.Migration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, spaceErr))
	})
}

type longStoreMigration struct{}

func (longStoreMigration) Name() string {
	return "long migration"
}

func (longStoreMigration) Run(ctx context.Context, store common.StoreWithCtx, _ common.SpaceWithCtx) (toMigrate, migrated int, err error) {
	for {
		if _, err = store.QueryWithContext(ctx, database.Query{}); err != nil {
			return 0, 0, err
		}
	}
}

type longSpaceMigration struct{}

func (longSpaceMigration) Name() string {
	return "long migration"
}

func (longSpaceMigration) Run(ctx context.Context, _ common.StoreWithCtx, space common.SpaceWithCtx) (toMigrate, migrated int, err error) {
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

func (instantMigration) Name() string {
	return "instant migration"
}

func (instantMigration) Run(_ context.Context, _ common.StoreWithCtx, _ common.SpaceWithCtx) (toMigrate, migrated int, err error) {
	return 0, 0, nil
}

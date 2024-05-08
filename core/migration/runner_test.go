package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

func TestRunner(t *testing.T) {
	t.Run("context exceeds + migration is not finished -> ErrContextExceeded is returned", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("")

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := run(ctx, store, space, longMigration{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCtxExceeded))
	})

	t.Run("context exceeds + migration is finished -> no error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Id().Times(1).Return("")

		// when
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := run(ctx, store, space, instantMigration{})

		// then
		assert.NoError(t, err)
	})
}

type longMigration struct{}

func (longMigration) Name() string {
	return "long migration"
}

func (longMigration) Run(store QueryableStore, _ DoableSpace) (toMigrate, migrated int, err error) {
	for {
		if _, err = store.Query(database.Query{}); err != nil {
			return 0, 0, err
		}
	}
}

type instantMigration struct{}

func (instantMigration) Name() string {
	return "instant migration"
}

func (instantMigration) Run(_ QueryableStore, _ DoableSpace) (toMigrate, migrated int, err error) {
	return 0, 0, nil
}

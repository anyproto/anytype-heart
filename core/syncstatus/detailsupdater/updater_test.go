package detailsupdater

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSyncStatusUpdater_UpdateDetails(t *testing.T) {
	t.Run("update sync status and date", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null})
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(domain.Synced)), details[bundle.RelationKeySyncStatus.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
	t.Run("update sync status, error and date", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Error, domain.NetworkError})
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(domain.Error)), details[bundle.RelationKeySyncStatus.String()])
		assert.Equal(t, pbtypes.Int64(int64(domain.NetworkError)), details[bundle.RelationKeySyncError.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
}

func newFixture(t *testing.T) *fixture {
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	smartTest := smarttest.New("id")
	objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartTest, nil)
	storeFixture := objectstore.NewStoreFixture(t)
	updater := &syncStatusUpdater{batcher: mb.New[*syncStatusDetails](0), finish: make(chan struct{})}
	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter)).Register(storeFixture)
	err := updater.Init(a)
	assert.Nil(t, err)
	return &fixture{
		updater: updater,
		sb:      smartTest,
	}
}

type fixture struct {
	sb      *smarttest.SmartTest
	updater *syncStatusUpdater
}

package updater

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/syncstatus/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSyncStatusUpdater_UpdateDetails(t *testing.T) {
	t.Run("update sync status and date", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.UpdateDetails("id", helpers.Synced, helpers.Null)
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(helpers.Synced)), details[bundle.RelationKeySyncStatus.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
	t.Run("update sync status, error and date", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.UpdateDetails("id", helpers.Error, helpers.NetworkError)
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(helpers.Error)), details[bundle.RelationKeySyncStatus.String()])
		assert.Equal(t, pbtypes.Int64(int64(helpers.NetworkError)), details[bundle.RelationKeySyncError.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
}

func newFixture(t *testing.T) *fixture {
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	smartTest := smarttest.New("id")
	objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartTest, nil)
	updater := NewUpdater()
	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter))
	err := updater.Init(a)
	assert.Nil(t, err)
	return &fixture{
		Updater: updater,
		sb:      smartTest,
	}
}

type fixture struct {
	Updater
	sb *smarttest.SmartTest
}

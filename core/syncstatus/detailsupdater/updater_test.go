package detailsupdater

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	domain "github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/mock_detailsupdater"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscritions"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSyncStatusUpdater_UpdateDetails(t *testing.T) {

}

func TestSyncStatusUpdater_Run(t *testing.T) {

}

func TestSyncStatusUpdater_setSyncDetails(t *testing.T) {
	t.Run("set smartblock details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.setSyncDetails(fixture.sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(domain.SpaceSyncStatusError)), details[bundle.RelationKeySyncStatus.String()])
		assert.Equal(t, pbtypes.Int64(int64(domain.SyncErrorNetworkError)), details[bundle.RelationKeySyncError.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
	t.Run("not set smartblock details, because it doesn't implement interface DetailsSettable", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypePage)
		err := fixture.updater.setSyncDetails(editor.NewMissingObject(fixture.sb), domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
	t.Run("not set smartblock details, because it doesn't need details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypeHome)
		err := fixture.updater.setSyncDetails(fixture.sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_isLayoutSuitableForSyncRelations(t *testing.T) {
	t.Run("isLayoutSuitableForSyncRelations - participant details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyLayout.String(): pbtypes.Float64(float64(model.ObjectType_participant)),
		}}
		isSuitable := fixture.updater.isLayoutSuitableForSyncRelations(details)

		// then
		assert.False(t, isSuitable)
	})

	t.Run("isLayoutSuitableForSyncRelations - basic details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
		}}
		isSuitable := fixture.updater.isLayoutSuitableForSyncRelations(details)

		// then
		assert.True(t, isSuitable)
	})
}

func newFixture(t *testing.T) *fixture {
	smartTest := smarttest.New("id")
	service := mock_space.NewMockService(t)
	updater := &syncStatusUpdater{
		finish:  make(chan struct{}),
		entries: map[string]*syncStatusDetails{},
	}
	statusUpdater := mock_detailsupdater.NewMockSpaceStatusUpdater(t)

	subscriptionService := subscription.NewInternalTestService(t)

	syncSub := syncsubscritions.New()

	ctx := context.Background()

	a := &app.App{}
	a.Register(subscriptionService)
	a.Register(syncSub)
	a.Register(testutil.PrepareMock(ctx, a, service))
	a.Register(testutil.PrepareMock(ctx, a, statusUpdater))
	err := updater.Init(a)
	assert.Nil(t, err)
	return &fixture{
		updater:             updater,
		sb:                  smartTest,
		service:             service,
		statusUpdater:       statusUpdater,
		subscriptionService: subscriptionService,
	}
}

type fixture struct {
	sb                  *smarttest.SmartTest
	updater             *syncStatusUpdater
	service             *mock_space.MockService
	statusUpdater       *mock_detailsupdater.MockSpaceStatusUpdater
	subscriptionService *subscription.InternalTestService
}

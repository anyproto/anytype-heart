package objectsyncstatus

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/headsync/statestorage/mock_statestorage"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/spacestorage/mock_spacestorage"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus/mock_objectsyncstatus"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	testSpaceSettingsId = "testSpaceSettingsId"
)

func Test_UseCases(t *testing.T) {
	t.Run("HeadsChange: new object", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSyncing, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)
	})
	t.Run("HeadsChange then HeadsApply: responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSyncing, "spaceId")
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSynced, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)

		s.nodeConfService.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})

		s.HeadsApply("peerId", "id", []string{"head1", "head2"}, true)

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusSynced, s.treeHeads["id"].syncStatus)
	})
	t.Run("HeadsChange then HeadsApply: not responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSyncing, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)

		s.nodeConfService.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})

		s.HeadsApply("peerId", "id", []string{"head1", "head2"}, true)

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusNotSynced, s.treeHeads["id"].syncStatus)
		assert.Contains(t, s.tempSynced, "id")
	})
	t.Run("ObjectReceive: responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.nodeConfService.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSynced, "spaceId")

		s.ObjectReceive("peerId", "id", []string{"head1", "head2"})
	})
	t.Run("ObjectReceive: not responsible, but then sync with responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.nodeConfService.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})
		s.syncDetailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncStatusSynced, "spaceId")

		s.ObjectReceive("peerId", "id", []string{"head1", "head2"})

		require.Contains(t, s.tempSynced, "id")

		s.nodeConfService.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})

		s.RemoveAllExcept("peerId1", []string{})
	})
	t.Run("HeadsChange: settings object is changed", func(t *testing.T) {
		s := newFixture(t, "spaceId")

		s.HeadsChange(testSpaceSettingsId, []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads[testSpaceSettingsId])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads[testSpaceSettingsId].heads)
	})
}

func TestSyncStatusService_Run(t *testing.T) {
	t.Run("successful run", func(t *testing.T) {
		s := newFixture(t, "spaceId")

		err := s.Run(context.Background())

		assert.Nil(t, err)
		err = s.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestSyncStatusService_RemoveAllExcept(t *testing.T) {
	t.Run("no existing id", func(t *testing.T) {
		f := newFixture(t, "spaceId")
		f.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"heads"}}

		f.nodeConfService.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", nil)

		assert.Equal(t, StatusSynced, f.treeHeads["id"].syncStatus)
	})
	t.Run("same ids", func(t *testing.T) {
		f := newFixture(t, "id")
		f.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"heads"}}

		f.nodeConfService.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", []string{"id"})

		assert.Equal(t, StatusNotSynced, f.treeHeads["id"].syncStatus)
	})
	t.Run("sender not responsible", func(t *testing.T) {
		f := newFixture(t, "spaceId")
		f.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"heads"}}

		f.nodeConfService.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId1"})
		f.RemoveAllExcept("peerId", nil)

		assert.Equal(t, StatusNotSynced, f.treeHeads["id"].syncStatus)
	})
}

func TestHeadsChange(t *testing.T) {
	fx := newFixture(t, "space1")
	fx.syncDetailsUpdater.EXPECT().UpdateDetails("obj1", domain.ObjectSyncStatusSyncing, "space1")
	inputHeads := []string{"b", "c", "a"}

	fx.HeadsChange("obj1", inputHeads)

	got, ok := fx.treeHeads["obj1"]
	require.True(t, ok)

	want := treeHeadsEntry{
		heads:      []string{"a", "b", "c"},
		syncStatus: StatusNotSynced,
	}
	assert.Equal(t, want, got)
	assert.Equal(t, []string{"b", "c", "a"}, inputHeads, "heads should be copied")

}

type fixture struct {
	*syncStatusService
	nodeConfService    *mock_nodeconf.MockService
	spaceStorage       *mock_spacestorage.MockSpaceStorage
	config             *config.Config
	syncDetailsUpdater *mock_objectsyncstatus.MockUpdater
	nodeStatus         nodestatus.NodeStatus
}

func newFixture(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	service := mock_nodeconf.NewMockService(ctrl)
	storage := mock_spacestorage.NewMockSpaceStorage(ctrl)
	stateStorage := mock_statestorage.NewMockStateStorage(ctrl)
	storage.EXPECT().StateStorage().AnyTimes().Return(stateStorage)
	stateStorage.EXPECT().SettingsId().AnyTimes().Return(testSpaceSettingsId)

	spaceState := &spacestate.SpaceState{SpaceId: spaceId}
	config := &config.Config{}
	detailsUpdater := mock_objectsyncstatus.NewMockUpdater(t)
	nodeStatus := nodestatus.NewNodeStatus()
	a := &app.App{}

	a.Register(testutil.PrepareMock(context.Background(), a, service)).
		Register(testutil.PrepareMock(context.Background(), a, storage)).
		Register(testutil.PrepareMock(context.Background(), a, detailsUpdater)).
		Register(nodeStatus).
		Register(config).
		Register(spaceState)

	err := nodeStatus.Init(a)
	assert.Nil(t, err)

	statusService := NewSyncStatusService()
	err = statusService.Init(a)
	assert.Nil(t, err)
	return &fixture{
		syncStatusService:  statusService.(*syncStatusService),
		nodeConfService:    service,
		spaceStorage:       storage,
		config:             config,
		syncDetailsUpdater: detailsUpdater,
		nodeStatus:         nodeStatus,
	}
}

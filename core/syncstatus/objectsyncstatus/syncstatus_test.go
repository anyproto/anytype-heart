package objectsyncstatus

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
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

func Test_UseCases(t *testing.T) {
	t.Run("HeadsChange: new object", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.detailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncing, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)
	})
	t.Run("HeadsChange then HeadsApply: responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.detailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncing, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)

		s.service.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})

		s.HeadsApply("peerId", "id", []string{"head1", "head2"}, true)

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusSynced, s.treeHeads["id"].syncStatus)
		assert.Equal(t, s.synced, []string{"id"})
	})
	t.Run("HeadsChange then HeadsApply: not responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.detailsUpdater.EXPECT().UpdateDetails("id", domain.ObjectSyncing, "spaceId")

		s.HeadsChange("id", []string{"head1", "head2"})

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)

		s.service.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})

		s.HeadsApply("peerId", "id", []string{"head1", "head2"}, true)

		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusNotSynced, s.treeHeads["id"].syncStatus)
		assert.Contains(t, s.tempSynced, "id")
		assert.Nil(t, s.synced)
	})
	t.Run("ObjectReceive: responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.service.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})

		s.ObjectReceive("peerId", "id", []string{"head1", "head2"})

		assert.Equal(t, s.synced, []string{"id"})
	})
	t.Run("ObjectReceive: not responsible, but then sync with responsible", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		s.service.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})

		s.ObjectReceive("peerId", "id", []string{"head1", "head2"})

		require.Contains(t, s.tempSynced, "id")

		s.service.EXPECT().NodeIds("spaceId").Return([]string{"peerId1"})

		s.RemoveAllExcept("peerId1", []string{})

		assert.Equal(t, s.synced, []string{"id"})
	})
}

func TestSyncStatusService_Watch_Unwatch(t *testing.T) {
	t.Run("watch", func(t *testing.T) {
		s := newFixture(t, "spaceId")

		s.storage.EXPECT().TreeStorage("id").Return(treestorage.NewInMemoryTreeStorage(&treechangeproto.RawTreeChangeWithId{Id: "id"}, []string{"headId"}, nil))
		err := s.Watch("id")
		assert.Nil(t, err)
		assert.Contains(t, s.watchers, "id")
		assert.Equal(t, []string{"headId"}, s.treeHeads["id"].heads)
	})
	t.Run("unwatch", func(t *testing.T) {
		s := newFixture(t, "spaceId")

		s.storage.EXPECT().TreeStorage("id").Return(treestorage.NewInMemoryTreeStorage(&treechangeproto.RawTreeChangeWithId{Id: "id"}, []string{"headId"}, nil))
		err := s.Watch("id")
		assert.Nil(t, err)

		s.Unwatch("id")
		assert.NotContains(t, s.watchers, "id")
		assert.Equal(t, []string{"headId"}, s.treeHeads["id"].heads)
	})
}

func TestSyncStatusService_update(t *testing.T) {
	t.Run("update: got updates on objects", func(t *testing.T) {
		s := newFixture(t, "spaceId")
		updateReceiver := NewMockUpdateReceiver(t)
		updateReceiver.EXPECT().UpdateNodeStatus().Return()
		updateReceiver.EXPECT().UpdateTree(context.Background(), "id", StatusSynced).Return(nil)
		updateReceiver.EXPECT().UpdateTree(context.Background(), "id2", StatusNotSynced).Return(nil)
		s.SetUpdateReceiver(updateReceiver)

		s.detailsUpdater.EXPECT().UpdateDetails("id3", domain.ObjectSynced, "spaceId")
		s.synced = []string{"id3"}
		s.tempSynced["id4"] = struct{}{}
		s.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusSynced, heads: []string{"headId"}}
		s.treeHeads["id2"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"headId"}}
		s.watchers["id"] = struct{}{}
		s.watchers["id2"] = struct{}{}
		err := s.update(context.Background())
		require.NoError(t, err)
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

		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", nil)

		assert.Equal(t, StatusSynced, f.treeHeads["id"].syncStatus)
	})
	t.Run("same ids", func(t *testing.T) {
		f := newFixture(t, "id")
		f.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"heads"}}

		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", []string{"id"})

		assert.Equal(t, StatusNotSynced, f.treeHeads["id"].syncStatus)
	})
	t.Run("sender not responsible", func(t *testing.T) {
		f := newFixture(t, "spaceId")
		f.treeHeads["id"] = treeHeadsEntry{syncStatus: StatusNotSynced, heads: []string{"heads"}}

		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId1"})
		f.RemoveAllExcept("peerId", nil)

		assert.Equal(t, StatusNotSynced, f.treeHeads["id"].syncStatus)
	})
}

type fixture struct {
	*syncStatusService
	service        *mock_nodeconf.MockService
	storage        *mock_spacestorage.MockSpaceStorage
	config         *config.Config
	detailsUpdater *mock_objectsyncstatus.MockUpdater
	nodeStatus     nodestatus.NodeStatus
}

func newFixture(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	service := mock_nodeconf.NewMockService(ctrl)
	storage := mock_spacestorage.NewMockSpaceStorage(ctrl)
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
		syncStatusService: statusService.(*syncStatusService),
		service:           service,
		storage:           storage,
		config:            config,
		detailsUpdater:    detailsUpdater,
		nodeStatus:        nodeStatus,
	}
}

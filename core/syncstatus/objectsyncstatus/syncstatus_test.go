package objectsyncstatus

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/spacestorage/mock_spacestorage"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/tests/testutil"
)

func Test_HeadsChange(t *testing.T) {
	t.Run("HeadsChange: new object", func(t *testing.T) {
		// given
		s := &syncStatusService{treeHeads: map[string]treeHeadsEntry{}}

		// when
		s.HeadsChange("id", []string{"head1", "head2"})

		// then
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head1", "head2"}, s.treeHeads["id"].heads)
	})
	t.Run("HeadsChange: update existing object", func(t *testing.T) {
		// given
		s := &syncStatusService{treeHeads: map[string]treeHeadsEntry{}}

		// when
		s.HeadsChange("id", []string{"head1", "head2"})
		s.HeadsChange("id", []string{"head3"})

		// then
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, []string{"head3"}, s.treeHeads["id"].heads)
	})
}

func TestSyncStatusService_HeadsReceive(t *testing.T) {
	t.Run("HeadsReceive: heads not changed ", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		s.HeadsReceive("peerId", "id", []string{"head1", "head2"})

		// then
		_, ok := s.treeHeads["id"]
		assert.False(t, ok)
	})
	t.Run("HeadsReceive: object synced", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		s.treeHeads["id"] = treeHeadsEntry{
			syncStatus: StatusSynced,
		}
		s.HeadsReceive("peerId", "id", []string{"head1", "head2"})

		// then
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusSynced, s.treeHeads["id"].syncStatus)
	})
	t.Run("HeadsReceive: sender in not responsible", func(t *testing.T) {
		// given
		s := newFixture(t)
		s.service.EXPECT().NodeIds(s.spaceId).Return([]string{"peerId2"})

		// when
		s.HeadsChange("id", []string{"head1"})
		s.HeadsReceive("peerId", "id", []string{"head2"})

		// then
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusNotSynced, s.treeHeads["id"].syncStatus)
	})
	t.Run("HeadsReceive: object is synced", func(t *testing.T) {
		// given
		s := newFixture(t)
		s.service.EXPECT().NodeIds(s.spaceId).Return([]string{"peerId"})

		// when
		s.HeadsChange("id", []string{"head1"})
		s.HeadsReceive("peerId", "id", []string{"head1"})

		// then
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusSynced, s.treeHeads["id"].syncStatus)
	})
}

func TestSyncStatusService_Watch(t *testing.T) {
	t.Run("Watch: object exist", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		s.HeadsChange("id", []string{"head1"})
		err := s.Watch("id")

		// then
		assert.Nil(t, err)
		_, ok := s.watchers["id"]
		assert.True(t, ok)
	})
	t.Run("Watch: object not exist", func(t *testing.T) {
		// given
		s := newFixture(t)
		accountKeys, err := accountdata.NewRandom()
		assert.Nil(t, err)
		acl, err := list.NewTestDerivedAcl("spaceId", accountKeys)
		assert.Nil(t, err)

		root, err := objecttree.CreateObjectTreeRoot(objecttree.ObjectTreeCreatePayload{
			PrivKey:       accountKeys.SignKey,
			ChangeType:    "changeType",
			ChangePayload: nil,
			SpaceId:       "spaceId",
			IsEncrypted:   true,
		}, acl)
		storage, err := treestorage.NewInMemoryTreeStorage(root, []string{"head1"}, nil)
		assert.Nil(t, err)

		s.storage.EXPECT().TreeStorage("id").Return(storage, nil)

		// when
		err = s.Watch("id")

		// then
		assert.Nil(t, err)
		_, ok := s.watchers["id"]
		assert.True(t, ok)
		assert.NotNil(t, s.treeHeads["id"])
		assert.Equal(t, StatusUnknown, s.treeHeads["id"].syncStatus)
	})
}

func TestSyncStatusService_Unwatch(t *testing.T) {
	t.Run("Unwatch: object exist", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		s.HeadsChange("id", []string{"head1"})
		err := s.Watch("id")
		assert.Nil(t, err)

		s.Unwatch("id")

		// then
		_, ok := s.watchers["id"]
		assert.False(t, ok)
	})
}

func TestSyncStatusService_update(t *testing.T) {
	t.Run("update: got updates on objects", func(t *testing.T) {
		// given
		s := newFixture(t)
		updateReceiver := NewMockUpdateReceiver(t)
		updateReceiver.EXPECT().UpdateNodeStatus().Return()
		updateReceiver.EXPECT().UpdateTree(context.Background(), "id", StatusNotSynced).Return(nil)
		s.SetUpdateReceiver(updateReceiver)

		// when
		s.HeadsChange("id", []string{"head1"})
		err := s.Watch("id")
		assert.Nil(t, err)
		err = s.update(context.Background())

		// then
		assert.Nil(t, err)
		updateReceiver.AssertCalled(t, "UpdateTree", context.Background(), "id", StatusNotSynced)
	})
	t.Run("update: watch object, but no update received", func(t *testing.T) {
		// given
		s := newFixture(t)
		updateReceiver := NewMockUpdateReceiver(t)
		s.SetUpdateReceiver(updateReceiver)

		// when
		s.HeadsChange("id", []string{"head1"})
		err := s.Watch("id")
		assert.Nil(t, err)
		delete(s.treeHeads, "id")
		err = s.update(context.Background())

		// then
		assert.NotNil(t, err)
		updateReceiver.AssertNotCalled(t, "UpdateTree")
	})
}

func TestSyncStatusService_Run(t *testing.T) {
	t.Run("successful run", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		err := s.Run(context.Background())

		// then
		assert.Nil(t, err)
		err = s.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestSyncStatusService_RemoveAllExcept(t *testing.T) {
	t.Run("no existing id", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.treeHeads["heads"] = treeHeadsEntry{syncStatus: StatusNotSynced}

		// when
		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", nil)

		// then
		assert.Equal(t, StatusSynced, f.treeHeads["heads"].syncStatus)
	})
	t.Run("same ids", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.treeHeads["heads1"] = treeHeadsEntry{syncStatus: StatusNotSynced}

		// when
		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{"peerId"})
		f.RemoveAllExcept("peerId", []string{"heads", "heads"})

		// then
		assert.Equal(t, StatusSynced, f.treeHeads["heads1"].syncStatus)
	})
	t.Run("sender not responsible", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.treeHeads["heads1"] = treeHeadsEntry{syncStatus: StatusNotSynced}

		// when
		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{})
		f.RemoveAllExcept("peerId", []string{"heads"})

		// then
		assert.Equal(t, StatusNotSynced, f.treeHeads["heads1"].syncStatus)
	})
	t.Run("current state is outdated", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.treeHeads["heads1"] = treeHeadsEntry{syncStatus: StatusNotSynced, stateCounter: 1}

		// when
		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{})
		f.RemoveAllExcept("peerId", []string{"heads"})

		// then
		assert.Equal(t, StatusNotSynced, f.treeHeads["heads1"].syncStatus)
	})
	t.Run("tree is not synced", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.treeHeads["heads"] = treeHeadsEntry{syncStatus: StatusNotSynced}

		// when
		f.service.EXPECT().NodeIds(f.spaceId).Return([]string{})
		f.RemoveAllExcept("peerId", []string{"heads"})

		// then
		assert.Equal(t, StatusNotSynced, f.treeHeads["heads"].syncStatus)
	})
}

type fixture struct {
	*syncStatusService
	service *mock_nodeconf.MockService
	storage *mock_spacestorage.MockSpaceStorage
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	service := mock_nodeconf.NewMockService(ctrl)
	storage := mock_spacestorage.NewMockSpaceStorage(ctrl)
	spaceState := &spacestate.SpaceState{SpaceId: "spaceId"}

	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, service)).
		Register(testutil.PrepareMock(context.Background(), a, storage)).
		Register(spaceState)

	syncStatusService := &syncStatusService{
		treeHeads: map[string]treeHeadsEntry{},
		watchers:  map[string]struct{}{},
	}
	err := syncStatusService.Init(a)
	assert.Nil(t, err)
	return &fixture{
		syncStatusService: syncStatusService,
		service:           service,
		storage:           storage,
	}
}

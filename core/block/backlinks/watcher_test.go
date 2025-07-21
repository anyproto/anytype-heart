package backlinks

import (
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

const spaceId = "spc1"

type testUpdater struct {
	callback func(info spaceindex.LinksUpdateInfo)
	runFunc  func(callback func(info spaceindex.LinksUpdateInfo))
}

func (u *testUpdater) SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo)) {
	u.callback = callback
}

func (u *testUpdater) start() {
	go u.runFunc(u.callback)
}

type fixture struct {
	store        *objectstore.StoreFixture
	spaceService *mock_space.MockService
	updater      *testUpdater
	*watcher
}

func newFixture(t *testing.T, aggregationInterval time.Duration) *fixture {
	updater := &testUpdater{}
	store := objectstore.NewStoreFixture(t)
	spaceSvc := mock_space.NewMockService(t)

	w := &watcher{
		updater:      updater,
		store:        store,
		spaceService: spaceSvc,

		aggregationInterval:  aggregationInterval,
		infoBatch:            mb.New[spaceindex.LinksUpdateInfo](0),
		accumulatedBacklinks: make(map[domain.FullID]*backLinksUpdate),
	}

	return &fixture{
		store:        store,
		spaceService: spaceSvc,
		updater:      updater,
		watcher:      w,
	}
}

func TestWatcher_Run(t *testing.T) {
	t.Run("backlinks update asynchronously", func(t *testing.T) {
		// given
		interval := 500 * time.Millisecond
		fromId := domain.FullID{ObjectID: "obj1", SpaceID: spaceId}
		f := newFixture(t, interval)

		f.updater.runFunc = func(callback func(info spaceindex.LinksUpdateInfo)) {
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: fromId,
				Added:       []string{"obj2", "obj3"},
				Removed:     nil,
			})
			time.Sleep(interval / 2)
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: fromId,
				Added:       []string{"obj4", "obj5"},
				Removed:     []string{"obj2"},
			})
			time.Sleep(interval / 2)
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: fromId,
				Added:       []string{"obj6"},
				Removed:     []string{"obj5"},
			})
		}

		spc := mock_clientspace.NewMockSpace(t)
		f.spaceService.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil)

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Archive: "archive",
		})
		spc.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func() error) error {
			if id == "obj2" {
				return ocache.ErrExists
			}
			return nil
		})

		spc.EXPECT().Do(mock.Anything, mock.Anything).Return(nil)

		// when
		err := f.watcher.Run(nil)
		require.NoError(t, err)

		f.updater.start()

		time.Sleep(4 * interval)
		err = f.watcher.Close(nil)

		// then
		assert.NoError(t, err)
	})
}

func TestWatcher_updateAccumulatedBacklinks(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		// given
		f := newFixture(t, time.Second)

		f.store.AddObjects(t, spaceId, []spaceindex.TestObject{{
			bundle.RelationKeyId:        domain.String("obj1"),
			bundle.RelationKeySpaceId:   domain.String(spaceId),
			bundle.RelationKeyBacklinks: domain.StringList([]string{"obj4", "obj5", "obj6"}),
		}, {
			bundle.RelationKeyId:        domain.String("obj3"),
			bundle.RelationKeySpaceId:   domain.String(spaceId),
			bundle.RelationKeyBacklinks: domain.StringList([]string{"obj1", "obj2", "obj4"}),
		}})

		spc := mock_clientspace.NewMockSpace(t)
		f.spaceService.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil)
		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Archive: "archive",
		})
		spc.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func() error) error {
			if id == "obj2" {
				return ocache.ErrExists
			}
			return apply()
		})

		spc.EXPECT().Do(mock.Anything, mock.Anything).Return(nil).Once()

		f.watcher.accumulatedBacklinks = map[domain.FullID]*backLinksUpdate{
			domain.FullID{ObjectID: "obj1", SpaceID: spaceId}: {
				added:   []string{"obj2", "obj3"},
				removed: []string{"obj4", "obj5"},
			},
			domain.FullID{ObjectID: "obj2", SpaceID: spaceId}: {
				added: []string{"obj4", "obj5"},
			},
			domain.FullID{ObjectID: "obj3", SpaceID: spaceId}: {
				removed: []string{"obj1", "obj4"},
			},
		}

		// when
		f.watcher.updateAccumulatedBacklinks()

		// then
		details, err := f.store.SpaceIndex(spaceId).GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, []string{"obj6", "obj2", "obj3"}, details.GetStringList(bundle.RelationKeyBacklinks))
		details, err = f.store.SpaceIndex(spaceId).GetDetails("obj3")
		require.NoError(t, err)
		assert.Equal(t, []string{"obj2"}, details.GetStringList(bundle.RelationKeyBacklinks))
	})
}

func TestApplyUpdate(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		// given
		m := make(map[domain.FullID]*backLinksUpdate)

		// when
		applyUpdate(m, spaceindex.LinksUpdateInfo{
			LinksFromId: domain.FullID{ObjectID: "obj1", SpaceID: spaceId},
			Added:       []string{"obj2", "spc2/obj3"},
			Removed:     []string{"spc1/obj4", "spc3/obj5"},
		}, parseId)

		// then
		require.Len(t, m, 4)

		update, ok := m[domain.FullID{ObjectID: "obj2", SpaceID: spaceId}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.added)
		assert.Empty(t, update.removed)

		update, ok = m[domain.FullID{ObjectID: "obj3", SpaceID: "spc2"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.added)
		assert.Empty(t, update.removed)

		update, ok = m[domain.FullID{ObjectID: "obj4", SpaceID: "spc1"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.removed)
		assert.Empty(t, update.added)

		update, ok = m[domain.FullID{ObjectID: "obj5", SpaceID: "spc3"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.removed)
		assert.Empty(t, update.added)
	})

	t.Run("prefilled map", func(t *testing.T) {
		// given
		m := map[domain.FullID]*backLinksUpdate{
			domain.FullID{ObjectID: "obj2", SpaceID: spaceId}: {
				added: []string{"obj3"},
			},
			domain.FullID{ObjectID: "obj3", SpaceID: "spc2"}: {
				removed: []string{"obj1"},
			},
			domain.FullID{ObjectID: "obj5", SpaceID: "spc3"}: {
				added:   []string{"obj1", "obj2"},
				removed: []string{"obj3"},
			},
		}

		// when
		applyUpdate(m, spaceindex.LinksUpdateInfo{
			LinksFromId: domain.FullID{ObjectID: "obj1", SpaceID: spaceId},
			Added:       []string{"obj2", "spc2/obj3"},
			Removed:     []string{"spc1/obj4", "spc3/obj5"},
		}, parseId)

		// then
		require.Len(t, m, 4)

		update, ok := m[domain.FullID{ObjectID: "obj2", SpaceID: spaceId}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj3", "obj1"}, update.added)
		assert.Empty(t, update.removed)

		update, ok = m[domain.FullID{ObjectID: "obj3", SpaceID: "spc2"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.added)
		assert.Empty(t, update.removed)

		update, ok = m[domain.FullID{ObjectID: "obj4", SpaceID: "spc1"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, update.removed)
		assert.Empty(t, update.added)

		update, ok = m[domain.FullID{ObjectID: "obj5", SpaceID: "spc3"}]
		require.True(t, ok)
		assert.Equal(t, []string{"obj3", "obj1"}, update.removed)
		assert.Equal(t, []string{"obj2"}, update.added)
	})
}

func parseId(id string) (domain.FullID, error) {
	parts := strings.Split(id, "/")
	switch len(parts) {
	case 0:
		return domain.FullID{}, domain.ErrParseLongId
	case 1:
		return domain.FullID{ObjectID: parts[0]}, nil
	default:
		return domain.FullID{ObjectID: parts[1], SpaceID: parts[0]}, nil
	}
}

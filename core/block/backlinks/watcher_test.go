package backlinks

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	resolver     *mock_idresolver.MockResolver
	spaceService *mock_space.MockService
	updater      *testUpdater
	*watcher
}

func newFixture(t *testing.T, aggregationInterval time.Duration) *fixture {
	updater := &testUpdater{}
	store := objectstore.NewStoreFixture(t)
	resolver := mock_idresolver.NewMockResolver(t)
	spaceSvc := mock_space.NewMockService(t)

	w := &watcher{
		updater:      updater,
		store:        store,
		resolver:     resolver,
		spaceService: spaceSvc,

		aggregationInterval:  aggregationInterval,
		infoBatch:            mb.New(0),
		accumulatedBacklinks: make(map[string]*backLinksUpdate),
	}

	return &fixture{
		store:        store,
		resolver:     resolver,
		spaceService: spaceSvc,
		updater:      updater,
		watcher:      w,
	}
}

func TestWatcher_Run(t *testing.T) {
	t.Run("backlinks update asynchronously", func(t *testing.T) {
		// given
		interval := 500 * time.Millisecond
		f := newFixture(t, interval)

		f.resolver.EXPECT().ResolveSpaceID(mock.Anything).Return(spaceId, nil)

		f.updater.runFunc = func(callback func(info spaceindex.LinksUpdateInfo)) {
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: "obj1",
				Added:       []string{"obj2", "obj3"},
				Removed:     nil,
			})
			time.Sleep(interval / 2)
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: "obj1",
				Added:       []string{"obj4", "obj5"},
				Removed:     []string{"obj2"},
			})
			time.Sleep(interval / 2)
			callback(spaceindex.LinksUpdateInfo{
				LinksFromId: "obj1",
				Added:       []string{"obj6"},
				Removed:     []string{"obj5"},
			})
		}

		spc := mock_clientspace.NewMockSpace(t)
		f.spaceService.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil)

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
		f.resolver.EXPECT().ResolveSpaceID(mock.Anything).Return(spaceId, nil)

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

		spc.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func() error) error {
			if id == "obj2" {
				return ocache.ErrExists
			}
			return apply()
		})

		spc.EXPECT().Do(mock.Anything, mock.Anything).Return(nil).Once()

		f.watcher.accumulatedBacklinks = map[string]*backLinksUpdate{
			"obj1": {
				added:   []string{"obj2", "obj3"},
				removed: []string{"obj4", "obj5"},
			},
			"obj2": {
				added: []string{"obj4", "obj5"},
			},
			"obj3": {
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

package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor"
	pfeditor "github.com/anyproto/anytype-heart/core/block/editor/personalfavorites"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/personalfavorites"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

const (
	widgetTestSpaceId = "space1.rk"
	widgetTestRootId  = "widgets-root"
)

// widgetTestGetter serves the widget roots by full id: the space must be the
// one asked for, the way the real cache resolves a FullID.
type widgetTestGetter struct {
	roots map[string]smartblock.SmartBlock
	// fetched receives the object id of every root handed out, which is the
	// step right before the adapter takes the root's lock: a contention
	// test waits on it, then changes the world, then releases the lock
	fetched chan string
}

func (g widgetTestGetter) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	return g.GetObjectByFullID(ctx, domain.FullID{SpaceID: widgetTestSpaceId, ObjectID: objectId})
}

func (g widgetTestGetter) GetObjectByFullID(_ context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	if id.SpaceID != widgetTestSpaceId {
		return nil, fmt.Errorf("object %s asked in space %q, not %q", id.ObjectID, id.SpaceID, widgetTestSpaceId)
	}
	sb, ok := g.roots[id.ObjectID]
	if !ok {
		return nil, apicore.ErrWidgetNotFound
	}
	if g.fetched != nil {
		g.fetched <- id.ObjectID
	}
	return sb, nil
}

type widgetTestSpaces struct{ spc clientspace.Space }

func (s widgetTestSpaces) Get(context.Context, string) (clientspace.Space, error) { return s.spc, nil }

// fakeFavorites is an in-memory personalfavorites.Service: what the personal
// root's Apply override pushes its deltas into. Entries are kept as the
// store keeps them (by link id, ordered through AfterId), so the test reads
// back exactly what a restart would rebuild the sidebar from.
type fakeFavorites struct {
	mu      sync.Mutex
	entries map[string]personalfavorites.WidgetEntry
}

func newFakeFavorites() *fakeFavorites {
	return &fakeFavorites{entries: map[string]personalfavorites.WidgetEntry{}}
}

func (f *fakeFavorites) Init(*app.App) error { return nil }
func (f *fakeFavorites) Name() string        { return "fakeFavorites" }
func (f *fakeFavorites) Subscribe(personalfavorites.SubscribeParams) func() {
	return func() {}
}
func (f *fakeFavorites) OnStoreUpdate(string, []personalfavorites.WidgetChange) {}

func (f *fakeFavorites) CreateWidget(_ context.Context, entry personalfavorites.WidgetEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[entry.Id] = entry
	return nil
}

func (f *fakeFavorites) DeleteWidget(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, id)
	return nil
}

func (f *fakeFavorites) UpdateWidget(_ context.Context, id string, update personalfavorites.WidgetUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[id]
	if !ok {
		return fmt.Errorf("no entry %s", id)
	}
	if update.Layout != nil {
		entry.Layout = *update.Layout
	}
	if update.Limit != nil {
		entry.Limit = *update.Limit
	}
	if update.ViewId != nil {
		entry.ViewId = *update.ViewId
	}
	if update.AfterId != nil {
		entry.AfterId = *update.AfterId
	}
	f.entries[id] = entry
	return nil
}

// GetWidgets walks the AfterId chain from the head, the order the store
// projects onto the sidebar.
func (f *fakeFavorites) GetWidgets(_ context.Context, spaceId string) ([]personalfavorites.WidgetEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byAfter := map[string]personalfavorites.WidgetEntry{}
	for _, entry := range f.entries {
		if entry.SpaceId == spaceId {
			byAfter[entry.AfterId] = entry
		}
	}
	var ordered []personalfavorites.WidgetEntry
	for prev := ""; ; {
		entry, ok := byAfter[prev]
		if !ok || len(ordered) > len(f.entries) {
			break
		}
		ordered = append(ordered, entry)
		prev = entry.Id
	}
	return ordered, nil
}

// targets is the store's sidebar order, by target.
func (f *fakeFavorites) targets(t *testing.T) []string {
	t.Helper()
	entries, err := f.GetWidgets(context.Background(), widgetTestSpaceId)
	require.NoError(t, err)
	targets := make([]string, len(entries))
	for i, entry := range entries {
		targets[i] = entry.TargetId
	}
	return targets
}

type widgetAdapterFixture struct {
	adapter   apicore.Widgets
	getter    widgetTestGetter
	spaceRoot *smarttest.SmartTest
	favorites *fakeFavorites
}

func widgetTestRoot(id string) *smarttest.SmartTest {
	root := smarttest.New(id)
	root.SetSpaceId(widgetTestSpaceId)
	// smarttest starts with no blocks at all; a widget root is a smartblock
	// block whose children are the wrappers
	root.AddBlock(simple.New(&model.Block{Id: id, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	return root
}

// permissiveSpace is a space whose verdicts allow every write.
func permissiveSpace(t *testing.T) *mock_clientspace.MockSpace {
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
	space.EXPECT().CanManageSpace().Return(true).Maybe()
	space.EXPECT().IsReadOnly().Return(false).Maybe()
	space.EXPECT().IsOneToOne().Return(false).Maybe()
	return space
}

// newWidgetAdapterFixture builds the space root as the real WidgetObject
// editor over a smarttest smartblock, and the personal root as the real
// VirtualWidgetObject over an in-memory favorites store, so a personal
// mutation is tested through the Apply override that projects it onto the
// store — the part of the personal path that a plain widget editor would
// skip.
func newWidgetAdapterFixture(t *testing.T) *widgetAdapterFixture {
	spaceRoot := widgetTestRoot(widgetTestRootId)
	favorites := newFakeFavorites()
	personalRoot := widgetTestRoot(domain.NewPersonalWidgetsId(widgetTestSpaceId))
	personal := pfeditor.NewVirtualWidget(personalRoot, nil, favorites, nil)
	require.NoError(t, personal.Init(&smartblock.InitContext{Ctx: context.Background(), State: personalRoot.NewState()}))
	t.Cleanup(func() { _ = personal.Close() })

	getter := widgetTestGetter{roots: map[string]smartblock.SmartBlock{
		spaceRoot.Id():    &editor.WidgetObject{SmartBlock: spaceRoot, Widget: widget.NewWidget(spaceRoot)},
		personalRoot.Id(): personal,
	}}
	return &widgetAdapterFixture{
		adapter:   newWidgetAdapter(getter, widgetTestSpaces{spc: permissiveSpace(t)}),
		getter:    getter,
		spaceRoot: spaceRoot,
		favorites: favorites,
	}
}

func (fx *widgetAdapterFixture) create(t *testing.T, scope apicore.WidgetScope, target string, placement apicore.WidgetPlacement) apicore.WidgetEntry {
	t.Helper()
	entry, err := fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, scope, apicore.WidgetCreate{
		Target: target, Layout: model.BlockContentWidget_Tree, Limit: 6, Placement: placement,
	})
	require.NoError(t, err)
	return entry
}

// deleteWidget removes entry through the adapter, asserting success and
// that the removed entry is answered.
func deleteWidget(t *testing.T, fx *widgetAdapterFixture, scope apicore.WidgetScope, entry apicore.WidgetEntry) {
	t.Helper()
	removed, err := fx.adapter.DeleteWidget(context.Background(), widgetTestSpaceId, scope, entry.Id, entry.Target)
	require.NoError(t, err)
	assert.Equal(t, entry.Id, removed.Id)
}

func assertDeleteErr(t *testing.T, fx *widgetAdapterFixture, scope apicore.WidgetScope, entry apicore.WidgetEntry, want error) {
	t.Helper()
	_, err := fx.adapter.DeleteWidget(context.Background(), widgetTestSpaceId, scope, entry.Id, entry.Target)
	assert.ErrorIs(t, err, want)
}

// planOf is a plan that ignores the current entry: the adapter tests
// exercise the mechanics, the service tests the planning.
func planOf(update apicore.WidgetUpdate) func(apicore.WidgetEntry) (apicore.WidgetUpdate, error) {
	return func(apicore.WidgetEntry) (apicore.WidgetUpdate, error) { return update, nil }
}

func (fx *widgetAdapterFixture) targets(t *testing.T, scope apicore.WidgetScope) []string {
	t.Helper()
	entries, err := fx.adapter.ListWidgets(context.Background(), widgetTestSpaceId, scope)
	require.NoError(t, err)
	targets := make([]string, len(entries))
	for i, entry := range entries {
		targets[i] = entry.Target
	}
	return targets
}

func TestWidgetAdapter(t *testing.T) {
	t.Run("create appends a wrapper with one link child and answers both ids", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)

		// when
		entry := fx.create(t, apicore.WidgetScopeSpace, "obj1", apicore.WidgetPlacement{})

		// then
		assert.NotEmpty(t, entry.LinkId)
		assert.Equal(t, entry.LinkId+"-wrapper", entry.Id, "the wrapper id is derived from the minted link id, the shape both roots keep")
		assert.Equal(t, apicore.WidgetEntry{Id: entry.Id, LinkId: entry.LinkId, Scope: apicore.WidgetScopeSpace, Target: "obj1", Layout: model.BlockContentWidget_Tree, Limit: 6, Placed: &apicore.WidgetPlacement{}}, entry)
		st := fx.spaceRoot.NewState()
		require.Equal(t, []string{entry.Id}, st.Pick(widgetTestRootId).Model().ChildrenIds)
		assert.Equal(t, []string{entry.LinkId}, st.Pick(entry.Id).Model().ChildrenIds)
		assert.Equal(t, "obj1", st.Pick(entry.LinkId).Model().GetLink().TargetBlockId)
	})

	t.Run("a second widget for the same target is refused under the lock", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)
		fx.create(t, apicore.WidgetScopeSpace, "obj1", apicore.WidgetPlacement{})

		// when
		_, err := fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{Target: "obj1"})

		// then
		assert.ErrorIs(t, err, apicore.ErrWidgetExists)
		assert.Equal(t, []string{"obj1"}, fx.targets(t, apicore.WidgetScopeSpace))
		// the other root is its own namespace
		fx.create(t, apicore.WidgetScopePersonal, "obj1", apicore.WidgetPlacement{})
	})

	t.Run("placement: last by default, first, after and before", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		fx.create(t, apicore.WidgetScopeSpace, "b", apicore.WidgetPlacement{})
		fx.create(t, apicore.WidgetScopeSpace, "first", apicore.WidgetPlacement{First: true})
		fx.create(t, apicore.WidgetScopeSpace, "after-a", apicore.WidgetPlacement{AfterId: a.Id})
		fx.create(t, apicore.WidgetScopeSpace, "before-a", apicore.WidgetPlacement{BeforeId: a.Id})

		// then
		assert.Equal(t, []string{"first", "before-a", "a", "after-a", "b"}, fx.targets(t, apicore.WidgetScopeSpace))

		// and a placement naming no widget is the not-found sentinel
		_, err := fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace,
			apicore.WidgetCreate{Target: "x", Placement: apicore.WidgetPlacement{AfterId: "nope"}})
		assert.ErrorIs(t, err, apicore.ErrWidgetNotFound)
	})

	t.Run("the personal root projects every mutation onto the favorites store", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)

		// when: create two
		a := fx.create(t, apicore.WidgetScopePersonal, "a", apicore.WidgetPlacement{})
		b := fx.create(t, apicore.WidgetScopePersonal, "b", apicore.WidgetPlacement{})

		// then: the store holds them by link id, chained in order
		assert.Equal(t, []string{"a", "b"}, fx.favorites.targets(t))
		assert.Equal(t, personalfavorites.WidgetEntry{Id: b.LinkId, SpaceId: widgetTestSpaceId, TargetId: "b", Layout: model.BlockContentWidget_Tree, Limit: 6, AfterId: a.LinkId}, fx.favorites.entries[b.LinkId])
		assert.Equal(t, b.LinkId+"-wrapper", b.Id, "the wrapper id survives the store's rebuild, which derives it from the link id")

		// when: move the second first and change its members in one update
		layout := model.BlockContentWidget_CompactList
		limit := int32(10)
		updated, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal, b.Id, planOf(apicore.WidgetUpdate{Layout: &layout, Limit: &limit, Placement: &apicore.WidgetPlacement{First: true}}))

		// then
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentWidget_CompactList, updated.Layout)
		assert.Equal(t, []string{"b", "a"}, fx.targets(t, apicore.WidgetScopePersonal))
		assert.Equal(t, []string{"b", "a"}, fx.favorites.targets(t))
		assert.Equal(t, int32(10), fx.favorites.entries[b.LinkId].Limit)
		assert.Equal(t, model.BlockContentWidget_CompactList, fx.favorites.entries[b.LinkId].Layout)

		// when: delete the first
		deleteWidget(t, fx, apicore.WidgetScopePersonal, a)

		// then
		assert.Equal(t, []string{"b"}, fx.favorites.targets(t))
		assert.Equal(t, []string{"b"}, fx.targets(t, apicore.WidgetScopePersonal))
		// and the space root never saw any of it
		assert.Empty(t, fx.targets(t, apicore.WidgetScopeSpace))
	})

	t.Run("update changes the wrapper's members and moves it in one apply", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		b := fx.create(t, apicore.WidgetScopeSpace, "b", apicore.WidgetPlacement{})
		layout := model.BlockContentWidget_View
		limit := int32(14)
		viewId := "view1"

		// when
		updated, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, b.Id, planOf(apicore.WidgetUpdate{Layout: &layout, Limit: &limit, ViewId: &viewId, Placement: &apicore.WidgetPlacement{BeforeId: a.Id}}))

		// then
		require.NoError(t, err)
		assert.Equal(t, apicore.WidgetEntry{Id: b.Id, LinkId: b.LinkId, Scope: apicore.WidgetScopeSpace, Target: "b", Layout: model.BlockContentWidget_View, Limit: 14, ViewId: "view1", Placed: &apicore.WidgetPlacement{BeforeId: a.Id}}, updated)
		assert.Equal(t, []string{"b", "a"}, fx.targets(t, apicore.WidgetScopeSpace))

		// and a nil member is left alone
		again, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id, planOf(apicore.WidgetUpdate{Placement: &apicore.WidgetPlacement{First: true}}))
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentWidget_Tree, again.Layout)
		assert.Equal(t, []string{"a", "b"}, fx.targets(t, apicore.WidgetScopeSpace))

		_, err = fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, "nope", planOf(apicore.WidgetUpdate{Limit: &limit}))
		assert.ErrorIs(t, err, apicore.ErrWidgetNotFound)
	})

	t.Run("delete removes the wrapper and its link", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		b := fx.create(t, apicore.WidgetScopeSpace, "b", apicore.WidgetPlacement{})

		// when
		deleteWidget(t, fx, apicore.WidgetScopeSpace, a)

		// then
		assert.Equal(t, []string{"b"}, fx.targets(t, apicore.WidgetScopeSpace))
		st := fx.spaceRoot.NewState()
		assert.Nil(t, st.Pick(a.Id))
		assert.Nil(t, st.Pick(a.LinkId))
		assert.NotNil(t, st.Pick(b.LinkId))
		assertDeleteErr(t, fx, apicore.WidgetScopeSpace, a, apicore.ErrWidgetNotFound)
	})

	t.Run("list skips a child that is not a widget pair", func(t *testing.T) {
		// given: a stray text block among the wrappers, as a corrupt root holds
		fx := newWidgetAdapterFixture(t)
		fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		st := fx.spaceRoot.NewState()
		st.Add(simple.New(&model.Block{Id: "stray", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "stray"}}}))
		require.NoError(t, st.InsertTo("", model.Block_Inner, "stray"))
		require.NoError(t, fx.spaceRoot.Apply(st))

		// then
		assert.Equal(t, []string{"a"}, fx.targets(t, apicore.WidgetScopeSpace))
	})

	t.Run("a write is refused under the lock when the role changed since the check", func(t *testing.T) {
		// given: a root with one widget, then a space whose verdicts say no
		fx := newWidgetAdapterFixture(t)
		fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
		space.EXPECT().CanManageSpace().Return(false).Maybe()
		space.EXPECT().IsReadOnly().Return(true).Maybe()
		refused := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})

		// when
		_, err := refused.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{Target: "b"})
		assert.ErrorIs(t, err, restriction.ErrRestricted)
		_, err = refused.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal, apicore.WidgetCreate{Target: "b"})
		assert.ErrorIs(t, err, restriction.ErrRestricted)

		// then: nothing was written, and the verdicts read as the service sees them
		assert.Equal(t, []string{"a"}, fx.targets(t, apicore.WidgetScopeSpace))
		assert.Empty(t, fx.favorites.targets(t))
		ok, err := refused.CanEditWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace)
		require.NoError(t, err)
		assert.False(t, ok, "a plain writer cannot touch the space root")
		ok, err = refused.CanEditWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal)
		require.NoError(t, err)
		assert.False(t, ok, "a reader keeps no personal root either")
	})

	t.Run("a root in another space is not served", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		_, err := fx.adapter.ListWidgets(context.Background(), "other.space", apicore.WidgetScopeSpace)
		require.Error(t, err)
	})

	t.Run("the plan sees the widget as stored under the lock", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		var seen apicore.WidgetEntry
		limit := int32(30)
		_, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id,
			func(current apicore.WidgetEntry) (apicore.WidgetUpdate, error) {
				seen = current
				return apicore.WidgetUpdate{Limit: &limit}, nil
			})
		require.NoError(t, err)
		a.Placed = nil
		assert.Equal(t, a, seen, "the plan sees the stored entry, without a placement receipt")
		// and a refusing plan writes nothing
		_, err = fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id,
			func(apicore.WidgetEntry) (apicore.WidgetUpdate, error) {
				return apicore.WidgetUpdate{}, fmt.Errorf("refused by the plan")
			})
		require.EqualError(t, err, "refused by the plan")
	})

	t.Run("concurrent creates for one target leave exactly one widget, in each root", func(t *testing.T) {
		for _, scope := range []apicore.WidgetScope{apicore.WidgetScopeSpace, apicore.WidgetScopePersonal} {
			fx := newWidgetAdapterFixture(t)
			// an unrelated head, so the duplicate is never the first entry
			fx.create(t, scope, "head", apicore.WidgetPlacement{})
			const racers = 8
			var wg sync.WaitGroup
			var successes, duplicates atomic.Int32
			for i := 0; i < racers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, scope, apicore.WidgetCreate{Target: "raced", Layout: model.BlockContentWidget_Tree, Limit: 6})
					switch {
					case err == nil:
						successes.Add(1)
					case errors.Is(err, apicore.ErrWidgetExists):
						duplicates.Add(1)
					default:
						t.Errorf("unexpected error: %v", err)
					}
				}()
			}
			wg.Wait()
			assert.Equal(t, int32(1), successes.Load(), scope)
			assert.Equal(t, int32(racers-1), duplicates.Load(), scope)
			assert.Equal(t, []string{"head", "raced"}, fx.targets(t, scope), scope)
		}
	})

	t.Run("a plain writer keeps a personal root and cannot touch the space root; a reader neither", func(t *testing.T) {
		type verdicts struct {
			manage, readOnly, oneToOne bool
			space, personal            bool
		}
		for name, v := range map[string]verdicts{
			"owner":      {manage: true, space: true, personal: true},
			"writer":     {manage: false, space: false, personal: true},
			"reader":     {manage: false, readOnly: true, space: false, personal: false},
			"one-to-one": {manage: true, oneToOne: true, space: false, personal: true},
		} {
			t.Run(name, func(t *testing.T) {
				fx := newWidgetAdapterFixture(t)
				space := mock_clientspace.NewMockSpace(t)
				space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
				space.EXPECT().CanManageSpace().Return(v.manage).Maybe()
				space.EXPECT().IsReadOnly().Return(v.readOnly).Maybe()
				space.EXPECT().IsOneToOne().Return(v.oneToOne).Maybe()
				adapter := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})
				for scope, want := range map[apicore.WidgetScope]bool{apicore.WidgetScopeSpace: v.space, apicore.WidgetScopePersonal: v.personal} {
					ok, err := adapter.CanEditWidgets(context.Background(), widgetTestSpaceId, scope)
					require.NoError(t, err)
					assert.Equal(t, want, ok, scope)
					_, err = adapter.CreateWidget(context.Background(), widgetTestSpaceId, scope, apicore.WidgetCreate{Target: "x", Layout: model.BlockContentWidget_Tree, Limit: 6})
					assert.Equal(t, want, err == nil, "%s create: %v", scope, err)
				}
			})
		}
	})

	t.Run("a denied update and delete write nothing, in either root", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		p := fx.create(t, apicore.WidgetScopePersonal, "p", apicore.WidgetPlacement{})
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
		space.EXPECT().CanManageSpace().Return(false).Maybe()
		space.EXPECT().IsReadOnly().Return(true).Maybe()
		space.EXPECT().IsOneToOne().Return(false).Maybe()
		denied := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})
		limit := int32(30)
		for scope, entry := range map[apicore.WidgetScope]apicore.WidgetEntry{apicore.WidgetScopeSpace: a, apicore.WidgetScopePersonal: p} {
			id := entry.Id
			_, err := denied.UpdateWidget(context.Background(), widgetTestSpaceId, scope, id, planOf(apicore.WidgetUpdate{Limit: &limit}))
			assert.ErrorIs(t, err, restriction.ErrRestricted, scope)
			if _, err := denied.DeleteWidget(context.Background(), widgetTestSpaceId, scope, id, entry.Target); true {
				assert.ErrorIs(t, err, restriction.ErrRestricted, scope)
			}
		}
		assert.Equal(t, []string{"a"}, fx.targets(t, apicore.WidgetScopeSpace))
		assert.Equal(t, []string{"p"}, fx.targets(t, apicore.WidgetScopePersonal))
		assert.Equal(t, int32(6), fx.favorites.entries[p.LinkId].Limit, "the store never saw the denied update")
	})

	t.Run("a personal root reopened from the store shows the same widgets, ids and order", func(t *testing.T) {
		// given: a root mutated through the adapter
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopePersonal, "a", apicore.WidgetPlacement{})
		b := fx.create(t, apicore.WidgetScopePersonal, "b", apicore.WidgetPlacement{})
		c := fx.create(t, apicore.WidgetScopePersonal, "c", apicore.WidgetPlacement{First: true})
		layout := model.BlockContentWidget_CompactList
		_, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal, b.Id, planOf(apicore.WidgetUpdate{Layout: &layout, Placement: &apicore.WidgetPlacement{BeforeId: a.Id}}))
		require.NoError(t, err)
		deleteWidget(t, fx, apicore.WidgetScopePersonal, c)
		before, err := fx.adapter.ListWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal)
		require.NoError(t, err)

		// when: a second editor is built over the same store, as a restart does
		reopenedRoot := widgetTestRoot(domain.NewPersonalWidgetsId(widgetTestSpaceId))
		reopened := pfeditor.NewVirtualWidget(reopenedRoot, nil, fx.favorites, nil)
		require.NoError(t, reopened.Init(&smartblock.InitContext{Ctx: context.Background(), State: reopenedRoot.NewState()}))
		t.Cleanup(func() { _ = reopened.Close() })
		fx.getter.roots[reopenedRoot.Id()] = reopened
		after, err := fx.adapter.ListWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal)
		require.NoError(t, err)

		// then
		assert.Equal(t, before, after)
		assert.Equal(t, []string{"b", "a"}, fx.targets(t, apicore.WidgetScopePersonal))
		assert.Equal(t, b.Id, after[0].Id)
	})

	t.Run("a role revoked while an operation waits for the lock is seen before Apply", func(t *testing.T) {
		// given: the space answers from a variable the test flips while it
		// holds the root's lock
		fx := newWidgetAdapterFixture(t)
		fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		var manage atomic.Bool
		manage.Store(true)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
		space.EXPECT().CanManageSpace().RunAndReturn(manage.Load).Maybe()
		space.EXPECT().IsOneToOne().Return(false).Maybe()
		space.EXPECT().IsReadOnly().Return(false).Maybe()
		adapter := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})
		ok, err := adapter.CanEditWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace)
		require.NoError(t, err)
		require.True(t, ok, "the up-front check passes")

		// when: the operation starts while the root is locked and is let run
		// until it has fetched the root (the step before it blocks on the
		// lock); only then is the role revoked and the lock released, so a
		// verdict captured before the lock would have been "yes"
		fetched := make(chan string, 1)
		fx.getter.fetched = fetched
		adapter = newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})
		fx.spaceRoot.Lock()
		done := make(chan error, 1)
		go func() {
			_, err := adapter.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{Target: "b", Layout: model.BlockContentWidget_Tree, Limit: 6})
			done <- err
		}()
		require.Equal(t, widgetTestRootId, <-fetched)
		manage.Store(false)
		fx.spaceRoot.Unlock()
		fx.getter.fetched = nil

		// then
		assert.ErrorIs(t, <-done, restriction.ErrRestricted)
		assert.Equal(t, []string{"a"}, fx.targets(t, apicore.WidgetScopeSpace))
	})

	t.Run("a plan waiting for the lock sees the pair another write stored meanwhile", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})

		// when: the update starts while the root is locked and is let run
		// until it has fetched the root; the test then changes the stored
		// pair through the locked root and releases it, so a pair read
		// before the lock would have been the old one
		fetched := make(chan string, 1)
		fx.getter.fetched = fetched
		fx.adapter = newWidgetAdapter(fx.getter, widgetTestSpaces{spc: permissiveSpace(t)})
		fx.spaceRoot.Lock()
		seen := make(chan apicore.WidgetEntry, 1)
		done := make(chan error, 1)
		go func() {
			_, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id,
				func(current apicore.WidgetEntry) (apicore.WidgetUpdate, error) {
					seen <- current
					limit := int32(30)
					return apicore.WidgetUpdate{Limit: &limit}, nil
				})
			done <- err
		}()
		require.Equal(t, widgetTestRootId, <-fetched)
		st := fx.spaceRoot.NewState()
		st.Get(a.Id).Model().GetWidget().Layout = model.BlockContentWidget_Link
		require.NoError(t, fx.spaceRoot.Apply(st))
		fx.spaceRoot.Unlock()

		// then
		require.NoError(t, <-done)
		assert.Equal(t, model.BlockContentWidget_Link, (<-seen).Layout, "the plan ran against the pair stored under the lock, not the one read before it")
		fx.getter.fetched = nil
	})

	t.Run("a delete of a widget retargeted meanwhile is refused, not applied under the old name", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		// heart's own RPC retargets the wrapper's link
		st := fx.spaceRoot.NewState()
		st.Get(a.LinkId).Model().GetLink().TargetBlockId = "b"
		require.NoError(t, fx.spaceRoot.Apply(st))

		_, err := fx.adapter.DeleteWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id, "a")
		assert.ErrorIs(t, err, apicore.ErrWidgetRetargeted)
		assert.Equal(t, []string{"b"}, fx.targets(t, apicore.WidgetScopeSpace))
		removed, err := fx.adapter.DeleteWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id, "b")
		require.NoError(t, err)
		assert.Equal(t, "b", removed.Target)
	})

	t.Run("the bin stays last: default and last placements land before it, after it is refused", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		bin := fx.create(t, apicore.WidgetScopeSpace, widget.DefaultWidgetBin, apicore.WidgetPlacement{})
		// a default create with a trailing bin, reported as before the bin
		b := fx.create(t, apicore.WidgetScopeSpace, "b", apicore.WidgetPlacement{})
		assert.Equal(t, []string{"a", "b", "bin"}, fx.targets(t, apicore.WidgetScopeSpace))
		assert.Equal(t, &apicore.WidgetPlacement{BeforeId: bin.Id}, b.Placed)
		// a move to last, reported the same way
		moved, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id, planOf(apicore.WidgetUpdate{Placement: &apicore.WidgetPlacement{}}))
		require.NoError(t, err)
		assert.Equal(t, []string{"b", "a", "bin"}, fx.targets(t, apicore.WidgetScopeSpace))
		assert.Equal(t, &apicore.WidgetPlacement{BeforeId: bin.Id}, moved.Placed)
		// a member-only update carries no placement
		limit := int32(30)
		untouched, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, a.Id, planOf(apicore.WidgetUpdate{Limit: &limit}))
		require.NoError(t, err)
		assert.Nil(t, untouched.Placed)
		// an anchor that is the bin, under the lock
		_, err = fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{Target: "c", Placement: apicore.WidgetPlacement{AfterId: bin.Id}})
		assert.ErrorIs(t, err, apicore.ErrBinPlacement)
		assert.Equal(t, []string{"b", "a", "bin"}, fx.targets(t, apicore.WidgetScopeSpace))
	})

	t.Run("a combined update is one apply, and a failing anchor leaves nothing behind", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		a := fx.create(t, apicore.WidgetScopeSpace, "a", apicore.WidgetPlacement{})
		b := fx.create(t, apicore.WidgetScopeSpace, "b", apicore.WidgetPlacement{})
		before := len(fx.spaceRoot.Results.Applies)
		layout := model.BlockContentWidget_Link
		limit := int32(30)

		// when: members and a move in one update
		_, err := fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, b.Id,
			planOf(apicore.WidgetUpdate{Layout: &layout, Limit: &limit, Placement: &apicore.WidgetPlacement{BeforeId: a.Id}}))
		require.NoError(t, err)
		assert.Equal(t, before+1, len(fx.spaceRoot.Results.Applies), "one apply for the members and the move together")

		// and: members with an anchor that does not exist — no apply at all
		tree := model.BlockContentWidget_Tree
		_, err = fx.adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, b.Id,
			planOf(apicore.WidgetUpdate{Layout: &tree, Placement: &apicore.WidgetPlacement{AfterId: "nope"}}))
		assert.ErrorIs(t, err, apicore.ErrWidgetNotFound)
		assert.Equal(t, before+1, len(fx.spaceRoot.Results.Applies))
		entries, err := fx.adapter.ListWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace)
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentWidget_Link, entries[0].Layout, "the layout change did not leak")
		assert.Equal(t, []string{"b", "a"}, fx.targets(t, apicore.WidgetScopeSpace))
	})

	t.Run("a personal writer revoked while waiting for the lock cannot update or delete", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		p := fx.create(t, apicore.WidgetScopePersonal, "p", apicore.WidgetPlacement{})
		var readOnly atomic.Bool
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Widgets: widgetTestRootId}).Maybe()
		space.EXPECT().CanManageSpace().Return(true).Maybe()
		space.EXPECT().IsOneToOne().Return(false).Maybe()
		space.EXPECT().IsReadOnly().RunAndReturn(readOnly.Load).Maybe()
		fetched := make(chan string, 1)
		fx.getter.fetched = fetched
		adapter := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: space})
		personal := fx.getter.roots[domain.NewPersonalWidgetsId(widgetTestSpaceId)]
		limit := int32(30)

		for name, op := range map[string]func() error{
			"update": func() error {
				_, err := adapter.UpdateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal, p.Id, planOf(apicore.WidgetUpdate{Limit: &limit}))
				return err
			},
			"delete": func() error {
				_, err := adapter.DeleteWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal, p.Id, "p")
				return err
			},
		} {
			readOnly.Store(false)
			personal.Lock()
			done := make(chan error, 1)
			go func() { done <- op() }()
			require.Equal(t, personal.Id(), <-fetched, name)
			readOnly.Store(true)
			personal.Unlock()
			assert.ErrorIs(t, <-done, restriction.ErrRestricted, name)
		}
		fx.getter.fetched = nil
		assert.Equal(t, []string{"p"}, fx.favorites.targets(t))
		assert.Equal(t, int32(6), fx.favorites.entries[p.LinkId].Limit)
	})

	t.Run("a create stores the members it was given, on both roots, across a reopen", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		for _, tc := range []apicore.WidgetCreate{
			{Target: "l", Layout: model.BlockContentWidget_Link, Limit: 6},
			{Target: "s", Layout: model.BlockContentWidget_List, Limit: 4},
			{Target: "v", Layout: model.BlockContentWidget_View, Limit: 50, ViewId: "view-1"},
		} {
			for _, scope := range []apicore.WidgetScope{apicore.WidgetScopeSpace, apicore.WidgetScopePersonal} {
				entry, err := fx.adapter.CreateWidget(context.Background(), widgetTestSpaceId, scope, tc)
				require.NoError(t, err)
				assert.Equal(t, tc.Layout, entry.Layout, "%s %s", scope, tc.Target)
				assert.Equal(t, tc.Limit, entry.Limit, "%s %s", scope, tc.Target)
				assert.Equal(t, tc.ViewId, entry.ViewId, "%s %s", scope, tc.Target)
			}
			stored := fx.favorites.entries
			var found bool
			for _, e := range stored {
				if e.TargetId == tc.Target {
					found = true
					assert.Equal(t, tc.Layout, e.Layout)
					assert.Equal(t, tc.Limit, e.Limit)
					assert.Equal(t, tc.ViewId, e.ViewId)
				}
			}
			require.True(t, found, "the store holds %s", tc.Target)
		}
		// and a reopened personal root shows the same members
		reopenedRoot := widgetTestRoot(domain.NewPersonalWidgetsId(widgetTestSpaceId))
		reopened := pfeditor.NewVirtualWidget(reopenedRoot, nil, fx.favorites, nil)
		require.NoError(t, reopened.Init(&smartblock.InitContext{Ctx: context.Background(), State: reopenedRoot.NewState()}))
		t.Cleanup(func() { _ = reopened.Close() })
		fx.getter.roots[reopenedRoot.Id()] = reopened
		entries, err := fx.adapter.ListWidgets(context.Background(), widgetTestSpaceId, apicore.WidgetScopePersonal)
		require.NoError(t, err)
		require.Len(t, entries, 3)
		assert.Equal(t, model.BlockContentWidget_View, entries[2].Layout)
		assert.Equal(t, int32(50), entries[2].Limit)
		assert.Equal(t, "view-1", entries[2].ViewId)
	})

	t.Run("a duplicate that appears while a create waits for the lock is refused", func(t *testing.T) {
		fx := newWidgetAdapterFixture(t)
		fx.create(t, apicore.WidgetScopeSpace, "head", apicore.WidgetPlacement{})
		fetched := make(chan string, 1)
		fx.getter.fetched = fetched
		adapter := newWidgetAdapter(fx.getter, widgetTestSpaces{spc: permissiveSpace(t)})
		applies := len(fx.spaceRoot.Results.Applies)

		// when: the competing create has fetched the root and is about to
		// take the lock; the test inserts its target through the locked root
		fx.spaceRoot.Lock()
		done := make(chan error, 1)
		go func() {
			_, err := adapter.CreateWidget(context.Background(), widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{Target: "raced", Layout: model.BlockContentWidget_Tree, Limit: 6})
			done <- err
		}()
		require.Equal(t, widgetTestRootId, <-fetched)
		st := fx.spaceRoot.NewState()
		_, err := widget.NewWidget(fx.spaceRoot).CreateBlock(st, &pb.RpcBlockCreateWidgetRequest{
			ContextId: widgetTestRootId, Position: model.Block_Inner, WidgetLayout: model.BlockContentWidget_Tree, ObjectLimit: 6,
			Block: &model.Block{Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "raced"}}},
		})
		require.NoError(t, err)
		require.NoError(t, fx.spaceRoot.Apply(st))
		fx.spaceRoot.Unlock()

		// then
		assert.ErrorIs(t, <-done, apicore.ErrWidgetExists)
		assert.Equal(t, []string{"head", "raced"}, fx.targets(t, apicore.WidgetScopeSpace))
		assert.Equal(t, applies+1, len(fx.spaceRoot.Results.Applies), "only the test's own insert applied")
		fx.getter.fetched = nil
	})
}

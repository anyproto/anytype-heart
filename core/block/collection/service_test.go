package collection

import (
	"context"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type testPicker struct {
	sb smartblock.SmartBlock
}

func (t *testPicker) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) Init(a *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

type fixture struct {
	picker *testPicker
	*Service
}

func newFixture(t *testing.T) *fixture {
	picker := &testPicker{}
	a := &app.App{}
	objectStore := mock_objectstore.NewMockObjectStore(t)
	objectStore.EXPECT().Name().Return("objectStore")

	systemObjectService := mock_system_object.NewMockService(t)
	systemObjectService.EXPECT().Name().Return("systemObjectService")

	a.Register(picker)
	a.Register(objectStore)
	a.Register(systemObjectService)
	s := New()

	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{picker: picker, Service: s}
}

func TestBroadcast(t *testing.T) {
	const collectionID = "collectionID"
	sb := smarttest.New(collectionID)

	s := newFixture(t)
	s.picker.sb = sb

	_, subCh1, err := s.SubscribeForCollection(collectionID, "sub1")
	require.NoError(t, err)
	_, subCh2, err := s.SubscribeForCollection(collectionID, "sub2")
	require.NoError(t, err)

	var wg sync.WaitGroup

	var sub1Results [][]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		for c := range subCh1 {
			sub1Results = append(sub1Results, c)
		}

	}()

	var sub2Results [][]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		for c := range subCh2 {
			sub2Results = append(sub2Results, c)
		}
	}()

	changeCollection := func(ids []string) {
		st := sb.NewState()
		st.UpdateStoreSlice(template.CollectionStoreKey, ids)
		err := sb.Apply(st)

		require.NoError(t, err)
	}

	changeCollection([]string{"1", "2", "3"})
	changeCollection([]string{"3", "2"})
	changeCollection([]string{"1", "2", "3", "4"})
	s.UnsubscribeFromCollection(collectionID, "sub1")
	changeCollection([]string{"1", "4"})
	s.UnsubscribeFromCollection(collectionID, "sub2")
	changeCollection([]string{"5"})

	wg.Wait()

	assert.Equal(t, [][]string{
		{"1", "2", "3"},
		{"3", "2"},
		{"1", "2", "3", "4"},
	}, sub1Results)
	assert.Equal(t, [][]string{
		{"1", "2", "3"},
		{"3", "2"},
		{"1", "2", "3", "4"},
		{"1", "4"},
	}, sub2Results)
}

func TestSetObjectTypeToViews(t *testing.T) {
	var (
		viewID1 = "view1"
		viewID2 = "view2"

		generateState = func(objectType domain.TypeKey, setOf string) *state.State {
			parent := state.NewDoc("root", nil).(*state.State)
			parent.SetObjectTypeKey(objectType)
			parent.Set(dataview.NewDataview(&model.Block{
				Id: state.DataviewBlockID,
				Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
					Views: []*model.BlockContentDataviewView{{Id: viewID1}, {Id: viewID2}},
				}},
			}))
			parent.SetDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList([]string{setOf}))
			return parent.NewState()
		}

		assertViews = func(st *state.State, defaultObjectType string) {
			block := st.Get(state.DataviewBlockID)
			dataviewBlock, _ := block.(dataview.Block)
			view1, _ := dataviewBlock.GetView(viewID1)
			view2, _ := dataviewBlock.GetView(viewID2)
			assert.Equal(t, defaultObjectType, view1.DefaultObjectTypeId)
			assert.Equal(t, defaultObjectType, view2.DefaultObjectTypeId)
		}
	)

	t.Run("object is not a set", func(t *testing.T) {
		// given
		st := generateState(bundle.TypeKeyPage, bundle.TypeKeySet.URL())

		// when
		setDefaultObjectTypeToViews(st)

		// then
		assertViews(st, "")
	})

	t.Run("object is a set by relation", func(t *testing.T) {
		// given
		st := generateState(bundle.TypeKeySet, bundle.RelationKeyDescription.URL())

		// when
		setDefaultObjectTypeToViews(st)

		// then
		assertViews(st, "")
	})

	t.Run("object is a set by object type", func(t *testing.T) {
		// given
		st := generateState(bundle.TypeKeySet, bundle.TypeKeyBook.URL())

		// when
		setDefaultObjectTypeToViews(st)

		// then
		assertViews(st, bundle.TypeKeyBook.URL())
	})

	t.Run("object is a set by internal type", func(t *testing.T) {
		// given
		st := generateState(bundle.TypeKeySet, bundle.TypeKeyFile.URL())

		// when
		setDefaultObjectTypeToViews(st)

		// then
		assertViews(st, "")
	})

	t.Run("object is a set by not creatable type", func(t *testing.T) {
		// given
		st := generateState(bundle.TypeKeySet, bundle.TypeKeyCollection.URL())

		// when
		setDefaultObjectTypeToViews(st)

		// then
		assertViews(st, "")
	})
}

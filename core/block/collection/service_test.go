package collection

import (
	"context"
	"fmt"
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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const collectionID = "collectionID"

type testPicker struct {
	sbMap map[string]smartblock.SmartBlock
}

func (t *testPicker) GetObject(ctx context.Context, id string) (smartblock.SmartBlock, error) {
	if t.sbMap == nil {
		return nil, fmt.Errorf("not found")
	}
	sb, found := t.sbMap[id]
	if !found {
		return nil, fmt.Errorf("not found")
	}
	return sb, nil
}

func (t *testPicker) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return t.GetObject(ctx, id.ObjectID)
}

func (t *testPicker) Init(a *app.App) error { return nil }

func (t *testPicker) Name() string { return "test.picker" }

type testFlusher struct{}

func (tf *testFlusher) Name() string { return "test.flusher" }

func (tf *testFlusher) Init(*app.App) error { return nil }

func (tf *testFlusher) Run(context.Context) error { return nil }

func (tf *testFlusher) Close(context.Context) error { return nil }

func (tf *testFlusher) FlushUpdates() {}

type fixture struct {
	picker *testPicker
	*Service
	objectStore *objectstore.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	a := &app.App{}
	picker := &testPicker{}
	flusher := &testFlusher{}
	objectStore := objectstore.NewStoreFixture(t)

	a.Register(picker)
	a.Register(objectStore)
	a.Register(flusher)
	s := New()

	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{picker: picker, Service: s, objectStore: objectStore}
}

func TestBroadcast(t *testing.T) {
	sb := smarttest.New(collectionID)

	s := newFixture(t)
	s.picker.sbMap = map[string]smartblock.SmartBlock{
		collectionID: sb,
	}

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
		viewID1    = "view1"
		viewID2    = "view2"
		setOfValue = "randomId"

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
		s := Service{}
		st := generateState(bundle.TypeKeyPage, setOfValue)

		// when
		s.setDefaultObjectTypeToViews("space1", st)

		// then
		assertViews(st, "")
	})

	for _, testCase := range []struct {
		name, key string
		sbType    coresb.SmartBlockType
		expected  string
	}{
		{name: "relation", key: bundle.RelationKeyDescription.String(), sbType: coresb.SmartBlockTypeRelation, expected: ""},
		{name: "object type", key: bundle.TypeKeyBook.String(), sbType: coresb.SmartBlockTypeObjectType, expected: setOfValue},
		{name: "internal type", key: bundle.TypeKeyFile.String(), sbType: coresb.SmartBlockTypeObjectType, expected: ""},
		{name: "not creatable type", key: bundle.TypeKeyObjectType.String(), sbType: coresb.SmartBlockTypeObjectType, expected: ""},
	} {
		t.Run("object is a set by "+testCase.name, func(t *testing.T) {
			// given
			s := newFixture(t)
			s.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
				{
					bundle.RelationKeyId:        pbtypes.String(setOfValue),
					bundle.RelationKeyUniqueKey: pbtypes.String(domain.MustUniqueKey(testCase.sbType, testCase.key).Marshal()),
				},
			})

			st := generateState(bundle.TypeKeySet, setOfValue)

			// when
			s.setDefaultObjectTypeToViews("space1", st)

			// then
			assertViews(st, testCase.expected)
		})
	}
}

func TestService_Add(t *testing.T) {
	t.Run("add new objects to collection", func(t *testing.T) {
		// given
		coll := smarttest.New(collectionID)
		obj1 := smarttest.New("obj1")
		obj2 := smarttest.New("obj2")

		s := newFixture(t)
		s.picker.sbMap = map[string]smartblock.SmartBlock{
			collectionID: coll,
			"obj1":       obj1,
			"obj2":       obj2,
		}

		// when
		err := s.Add(nil, &pb.RpcObjectCollectionAddRequest{
			ContextId: collectionID,
			ObjectIds: []string{"obj1", "obj2"},
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"obj1", "obj2"}, coll.NewState().GetStoreSlice(template.CollectionStoreKey))
	})
}

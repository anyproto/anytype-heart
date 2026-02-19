package order

import (
	"context"
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestReorder(t *testing.T) {
	for _, tc := range []struct {
		name             string
		originalOrderIds map[string]string
		objectIds        []string
	}{
		{
			name: "no changes",
			originalOrderIds: map[string]string{
				"obj1": "aaaaaa",
				"obj2": "bbbbbb",
				"obj3": "cccccc",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "drag to the first position #1",
			originalOrderIds: map[string]string{
				"obj1": "aaaaaa",
				"obj2": "bbbbbb",
				"obj3": "cccccc",
			},
			objectIds: []string{"obj2", "obj1", "obj3"},
		},
		{
			name: "drag to the first position #2",
			originalOrderIds: map[string]string{
				"obj1": "aaaaaa",
				"obj2": "bbbbbb",
				"obj3": "cccccc",
			},
			objectIds: []string{"obj3", "obj1", "obj2"},
		},
		{
			name: "drag to the last position #3",
			originalOrderIds: map[string]string{
				"obj1": "aaaaaa",
				"obj2": "bbbbbb",
				"obj3": "cccccc",
			},
			objectIds: []string{"obj2", "obj3", "obj1"},
		},
		{
			name: "client sends an incomplete list #1",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "bbb",
				"obj3": "ccc",
				"obj4": "ddd",
				"obj5": "eee",
			},
			objectIds: []string{"obj3", "obj1", "obj2"},
		},
		{
			name: "client sends an incomplete list #2",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "bbb",
				"obj3": "ccc",
				"obj4": "ddd",
				"obj5": "eee",
			},
			objectIds: []string{"obj5", "obj1", "obj3"},
		},
		{
			name: "client sends an incomplete list #3",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "bbb",
				"obj3": "ccc",
				"obj4": "ddd",
				"obj5": "eee",
			},
			objectIds: []string{"obj3", "obj1", "obj5"},
		},
		{
			name: "empty orders #1",
			originalOrderIds: map[string]string{
				"obj1": "",
			},
			objectIds: []string{"obj1"},
		},
		{
			name: "empty orders #2",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "",
				"obj3": "",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #1",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "aaa",
				"obj3": "bbb",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #2",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "",
				"obj3": "bbb",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #3",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "bbb",
				"obj3": "",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #4",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "",
				"obj3": "aaa",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #5",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "aaa",
				"obj3": "",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "some orders are empty #6",
			originalOrderIds: map[string]string{
				"obj1": "aaa",
				"obj2": "",
				"obj3": "",
			},
			objectIds: []string{"obj1", "obj2", "obj3"},
		},
		{
			name: "empty orders + incomplete list",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "",
				"obj3": "",
				"obj4": "",
				"obj5": "",
			},
			objectIds: []string{"obj3", "obj2", "obj5"},
		},
		{
			name: "some orders are empty + incomplete list",
			originalOrderIds: map[string]string{
				"obj1": "",
				"obj2": "xxx",
				"obj3": "",
				"obj4": "bbb",
				"obj5": "aaa",
			},
			objectIds: []string{"obj3", "obj2", "obj5"},
		},
		{
			name: "4 elements #1",
			originalOrderIds: map[string]string{
				"a": "AAA001",
				"b": "BBB002",
				"c": "CCC003",
				"d": "DDD004",
			},
			objectIds: []string{"a", "c", "b", "d"},
		},
		{
			name: "4 elements #2",
			originalOrderIds: map[string]string{
				"a": "AAA001",
				"b": "BBB002",
				"c": "CCC003",
				"d": "DDD004",
			},
			objectIds: []string{"a", "c", "d", "b"},
		},
		{
			name: "5 elements #1",
			originalOrderIds: map[string]string{
				"a": "A001",
				"b": "B002",
				"c": "C003",
				"d": "D004",
				"e": "E005",
			},
			objectIds: []string{"a", "d", "b", "e", "c"},
		},
		{
			name: "5 elements #2",
			originalOrderIds: map[string]string{
				"a": "A001",
				"b": "B002",
				"c": "C003",
				"d": "D004",
				"e": "E005",
			},
			objectIds: []string{"b", "d", "a", "e", "c"},
		},
		{
			name: "drag to top",
			originalOrderIds: map[string]string{
				"a": "A001",
				"b": "B002",
				"c": "C003",
				"d": "D004",
				"e": "E005",
			},
			objectIds: []string{"a", "d", "b", "c", "e"},
		},
		{
			name: "drag to bottom",
			originalOrderIds: map[string]string{
				"a": "A001",
				"b": "B002",
				"c": "C003",
				"d": "D004",
				"e": "E005",
			},
			objectIds: []string{"a", "c", "d", "e", "b"},
		},
		{
			name: "real world data - drag tag 57 to first position",
			originalOrderIds: map[string]string{
				"tag14": "XeOt",
				"tag60": "XfOO",
				"tag55": "XgNs",
				"tag56": "XhNN",
				"tag57": "XiMr",
				"tag58": "XjMM",
				"tag59": "XkLq",
				"tag6":  "XlLL",
				"tag61": "XmKp",
				"tag62": "XnKK",
				"tag63": "XoJo",
				"tag64": "XpJJ",
				"tag9":  "XqIn",
				"tag65": "XrII",
			},
			objectIds: []string{"tag57", "tag14", "tag60", "tag55", "tag56", "tag58", "tag59", "tag6", "tag61", "tag62", "tag63", "tag64", "tag9", "tag65"},
		},
		{
			name: "real world data - drag tag 14 to other position",
			originalOrderIds: map[string]string{
				"tag14": "XeOt",
				"tag60": "XfOO",
				"tag55": "XgNs",
				"tag56": "XhNN",
				"tag57": "XiMr",
				"tag58": "XjMM",
				"tag59": "XkLq",
				"tag6":  "XlLL",
				"tag61": "XmKp",
				"tag62": "XnKK",
				"tag63": "XoJo",
				"tag64": "XpJJ",
				"tag9":  "XqIn",
				"tag65": "XrII",
			},
			objectIds: []string{"tag60", "tag55", "tag56", "tag57", "tag58", "tag59", "tag6", "tag61", "tag14", "tag62", "tag63", "tag64", "tag9", "tag65"},
		},
		{
			name: "reverse",
			originalOrderIds: map[string]string{
				"a": "A001",
				"b": "B002",
				"c": "C003",
				"d": "D004",
			},
			objectIds: []string{"d", "c", "b", "a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testReorder(t, tc.objectIds, tc.originalOrderIds)
		})
	}
}

func testReorder(t *testing.T, objectIds []string, originalOrderIds map[string]string) {
	s := &orderSetter{}
	gotNewOrder, gotOps, err := s.reorder(objectIds, originalOrderIds, true)
	require.NoError(t, err)

	t.Log(gotOps)

	gotObjectIdsWithOrder := make([]idAndOrderId, len(gotNewOrder))
	for i := range gotObjectIdsWithOrder {
		gotObjectIdsWithOrder[i] = idAndOrderId{
			id:      objectIds[i],
			orderId: gotNewOrder[i],
		}
	}
	sort.Slice(gotObjectIdsWithOrder, func(i, j int) bool {
		return gotObjectIdsWithOrder[i].orderId < gotObjectIdsWithOrder[j].orderId
	})

	gotObjectIds := lo.Map(gotObjectIdsWithOrder, func(x idAndOrderId, _ int) string {
		return x.id
	})

	assert.Equal(t, objectIds, gotObjectIds)
}

func TestOrderSetter_rebuildIfNeeded(t *testing.T) {
	t.Run("empty views returns error", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		o := &orderSetter{objectGetter: objGetter}

		// when
		_, err := o.SetSpaceViewOrder([]string{})

		// then
		assert.Error(t, err)
	})

	t.Run("single new view", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb := smarttest.New("view1")
		mockSpaceView := &editor.SpaceView{SmartBlock: sb, OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder)}
		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		lexids, err := o.rebuildIfNeeded([]string{"view1"}, map[string]string{}, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 1)
		assert.NotEmpty(t, lexids[0])
		// The first lexid should be generated using Middle() for better padding
		assert.NotEmpty(t, lexids[0])
		// Middle() should give us a lexid roughly in the middle of the space
	})

	t.Run("multiple new views", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb1 := smarttest.New("view1")
		mockView1 := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}
		sb2 := smarttest.New("view2")
		mockView2 := &editor.SpaceView{SmartBlock: sb2, OrderSettable: order.NewOrderSettable(sb2, bundle.RelationKeySpaceOrder)}
		sb3 := smarttest.New("view3")
		mockView3 := &editor.SpaceView{SmartBlock: sb3, OrderSettable: order.NewOrderSettable(sb3, bundle.RelationKeySpaceOrder)}

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockView1, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockView2, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, map[string]string{}, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)
		// Check order
		assert.True(t, lexids[0] < lexids[1])
		assert.True(t, lexids[1] < lexids[2])
		// The first lexid should be generated using Middle() for better padding
		assert.NotEmpty(t, lexids[0])
		// Middle() should give us a lexid roughly in the middle of the space
	})

	t.Run("mix of existing and new views - simple", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb1 := smarttest.New("view1")
		mockView1 := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}
		sb3 := smarttest.New("view1")
		mockView3 := &editor.SpaceView{SmartBlock: sb3, OrderSettable: order.NewOrderSettable(sb3, bundle.RelationKeySpaceOrder)}

		// We expect view1 and view3 to be fetched to set their orders
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView1, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView3, nil).Maybe()

		o := &orderSetter{objectGetter: objGetter}

		// Existing lexids - view2 already has an order
		currentLexIds := map[string]string{
			"view2": "MMMM5000",
		}

		// when
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)
		// Final order should be correct
		assert.True(t, lexids[0] < lexids[1])
		assert.True(t, lexids[1] < lexids[2])
	})

	t.Run("all views already have lexids in correct order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		o := &orderSetter{objectGetter: objGetter}

		// All views already have lexids in the correct order
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0002",
			"view3": "MMMM0003",
		}

		// when
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)
		// Should keep existing lexids
		assert.Equal(t, "MMMM0001", lexids[0])
		assert.Equal(t, "MMMM0002", lexids[1])
		assert.Equal(t, "MMMM0003", lexids[2])
	})

	t.Run("reorder needed", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb := smarttest.New("view2")
		mockView2 := &editor.SpaceView{SmartBlock: sb, OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder)}

		// Expect view2 to be fetched for reordering
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView2, nil).Maybe()

		o := &orderSetter{objectGetter: objGetter}

		// Current order has view2 after view3, but we want it between view1 and view3
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0003", // Out of order
			"view3": "MMMM0002",
		}

		// when - desired order: view1, view2, view3
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)
		// Final order should be correct
		assert.True(t, lexids[0] < lexids[1])
		assert.True(t, lexids[1] < lexids[2])
	})

	t.Run("multiple new views pinned before first existing view", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb1 := smarttest.New("view1")
		mockView1 := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}
		sb2 := smarttest.New("view2")
		mockView2 := &editor.SpaceView{SmartBlock: sb2, OrderSettable: order.NewOrderSettable(sb2, bundle.RelationKeySpaceOrder)}
		sb3 := smarttest.New("view3")
		mockView3 := &editor.SpaceView{SmartBlock: sb3, OrderSettable: order.NewOrderSettable(sb3, bundle.RelationKeySpaceOrder)}

		// Expect all new views to be fetched for lexid assignment
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView1, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView2, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView3, nil).Maybe()

		o := &orderSetter{objectGetter: objGetter}

		// Scenario: view4 already exists with lexid, we want to pin view1, view2, view3 before it
		currentLexIds := map[string]string{
			"view4": "MMMM5000", // Existing view with lexid
		}

		// when - desired order: view1, view2, view3, view4 (first 3 are new, need to be inserted before view4)
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3", "view4"}, currentLexIds, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// All new views should have lexids less than the existing view4
		assert.True(t, lexids[0] < "MMMM5000", "view1 should be before view4")
		assert.True(t, lexids[1] < "MMMM5000", "view2 should be before view4")
		assert.True(t, lexids[2] < "MMMM5000", "view3 should be before view4")
		assert.Equal(t, "MMMM5000", lexids[3], "view4 should keep its existing lexid")

		// New views should be in correct order relative to each other
		assert.True(t, lexids[0] < lexids[1], "view1 should be before view2")
		assert.True(t, lexids[1] < lexids[2], "view2 should be before view3")
		assert.True(t, lexids[2] < lexids[3], "view3 should be before view4")
	})

	t.Run("reorder to first position", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb3 := smarttest.New("view3")
		mockView3 := &editor.SpaceView{SmartBlock: sb3, OrderSettable: order.NewOrderSettable(sb3, bundle.RelationKeySpaceOrder)}

		// STRICT expectations: only view3 needs fetching
		// Position 0 (view3): curr="MMMM0003" > next="MMMM0001" → receives new lexid
		// Position 1 (view1): curr="MMMM0001" > prev="<new lexid less than MMMM0001>" → keeps old lexid
		// Position 2 (view2): curr="MMMM0002" > prev="MMMM0001" → keeps old lexid
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockView3, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Current order: view3 is last, but we want it first
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0002",
			"view3": "MMMM0003", // Should be moved to first position
		}

		// when - desired order: view3, view1, view2 (view3 moves to first)
		lexids, err := o.rebuildIfNeeded([]string{"view3", "view1", "view2"}, currentLexIds, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)

		// The final order should be correct (view3 < view1 < view2)
		assert.True(t, lexids[0] < lexids[1], "view3 should be before view1")
		assert.True(t, lexids[1] < lexids[2], "view1 should be before view2")
	})
}

func TestOrderSetter_UnsetOrder(t *testing.T) {
	t.Run("unset order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb1 := smarttest.New("view1")
		mockSpaceView := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}

		// Pre-set an order
		err := mockSpaceView.SetOrder("aaaaaaaaaa")
		require.NoError(t, err)
		assert.NotEmpty(t, mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder))

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.UnsetOrder("view1")

		// then
		assert.NoError(t, err)
		assert.Empty(t, mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder))
	})
}

func TestCalculateFullList(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ids             []string
		fullOriginalIds []string
		orders          map[string]string
		want            []string
	}{
		{
			name:            "everything is empty",
			ids:             []string{},
			fullOriginalIds: []string{},
			orders:          map[string]string{},
			want:            []string{},
		},
		{
			name:            "ids is empty",
			ids:             []string{},
			fullOriginalIds: []string{"a", "b", "c"},
			orders: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:            "insert to the front",
			ids:             []string{"b", "a"},
			fullOriginalIds: []string{"a", "b", "c"},
			orders: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
			},
			want: []string{"b", "a", "c"},
		},
		{
			name:            "insert to the back",
			ids:             []string{"c", "b"},
			fullOriginalIds: []string{"a", "b", "c"},
			orders: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
			},
			want: []string{"a", "c", "b"},
		},
		{
			name:            "insert inside the list",
			ids:             []string{"c", "b"},
			fullOriginalIds: []string{"a", "b", "c", "d"},
			orders: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
				"d": "d",
			},
			want: []string{"a", "c", "b", "d"},
		},
		{
			name:            "complex",
			ids:             []string{"c", "a", "b"},
			fullOriginalIds: []string{"a", "b", "c", "d"},
			orders: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
				"d": "d",
			},
			want: []string{"c", "a", "b", "d"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateFullList(tc.ids, tc.fullOriginalIds, tc.orders)
			assert.Equal(t, tc.want, got)
		})
	}
}

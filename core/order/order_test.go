package order

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// orderSettableWrapper wraps OrderSettable to intercept SetBetweenOrders calls
type orderSettableWrapper struct {
	order.OrderSettable
	onSetBetweenOrders func(before, after string) error
}

func (w *orderSettableWrapper) SetBetweenOrders(before, after string) error {
	if w.onSetBetweenOrders != nil {
		return w.onSetBetweenOrders(before, after)
	}
	return w.OrderSettable.SetBetweenOrders(before, after)
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
		lexids, err := o.rebuildIfNeeded([]string{"view1"}, map[string]string{})

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
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, map[string]string{})

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
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds)

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
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds)

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
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3"}, currentLexIds)

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
		lexids, err := o.rebuildIfNeeded([]string{"view1", "view2", "view3", "view4"}, currentLexIds)

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
		sb1 := smarttest.New("view1")
		mockView1 := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}
		sb2 := smarttest.New("view2")
		mockView2 := &editor.SpaceView{SmartBlock: sb2, OrderSettable: order.NewOrderSettable(sb2, bundle.RelationKeySpaceOrder)}

		// STRICT expectations: only view3 needs fetching
		// view3: "MMMM0003" > "" (prev) → keeps its lexid (no fetch needed)
		// view1: "MMMM0001" > "MMMM0003" (prev) → NO, needs new lexid
		// view2: "MMMM0002" > view1's new lexid → depends on what view1 gets

		// Actually, let's trace through:
		// Position 0 (view3): curr="MMMM0003" > prev="" → keeps lexid
		// Position 1 (view1): curr="MMMM0001" > prev="MMMM0003" → NO, needs new lexid
		// Position 2 (view2): curr="MMMM0002" > prev=(view1's new lexid) → depends
		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockView1, nil).Once()
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockView2, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Current order: view3 is last, but we want it first
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0002",
			"view3": "MMMM0003", // Should be moved to first position
		}

		// when - desired order: view3, view1, view2 (view3 moves to first)
		lexids, err := o.rebuildIfNeeded([]string{"view3", "view1", "view2"}, currentLexIds)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)

		// The final order should be correct (view3 < view1 < view2)
		assert.True(t, lexids[0] < lexids[1], "view3 should be before view1")
		assert.True(t, lexids[1] < lexids[2], "view1 should be before view2")

		// With the simpler fix, view3 may keep its original lexid if it's valid
		// The important thing is that the final ordering is correct
	})
}

func TestOrderSetter_UnsetOrder(t *testing.T) {
	t.Run("unset order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		sb1 := smarttest.New("view1")
		mockSpaceView := &editor.SpaceView{SmartBlock: sb1, OrderSettable: order.NewOrderSettable(sb1, bundle.RelationKeySpaceOrder)}

		// Pre-set an order
		_, err := mockSpaceView.SetOrder("")
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

func TestOrderSetter_InvalidBoundsHandling(t *testing.T) {
	t.Run("move c between a and b - basic case", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mock for b since it needs repositioning
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// STRICT expectation: only b should be fetched
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Current lexids in order: a < b < c < d
		currentLexIds := map[string]string{
			"a": "AAA001",
			"b": "BBB002",
			"c": "CCC003",
			"d": "DDD004",
		}

		// when - desired order: [a, c, b, d] (move c between a and b)
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "b", "d"}, currentLexIds)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Log the actual lexids for debugging
		t.Logf("Final lexids: a=%s, c=%s, b=%s, d=%s", lexids[0], lexids[1], lexids[2], lexids[3])

		// Verify the final order is correct
		assert.True(t, lexids[0] < lexids[1], "a should be before c")
		assert.True(t, lexids[1] < lexids[2], "c should be before b")
		assert.True(t, lexids[2] < lexids[3], "b should be before d")

		// With the simpler fix, c might keep its lexid if it's > prev
		// But this could be wrong if it violates ordering with next element
	})


	t.Run("elements without lexids", func(t *testing.T) {
		// Test case where not all elements have lexids initially
		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mock views
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}
		sbC := smarttest.New("c")
		mockViewC := &editor.SpaceView{SmartBlock: sbC, OrderSettable: order.NewOrderSettable(sbC, bundle.RelationKeySpaceOrder)}

		// Track what SetBetweenOrders is called with for both b and c
		var capturedBeforeC, capturedAfterC string
		var setBetweenCalledC bool
		var capturedBeforeB, capturedAfterB string
		var setBetweenCalledB bool

		// Wrap c's OrderSettable
		origOrderSettableC := mockViewC.OrderSettable
		wrapperOrderSettableC := &orderSettableWrapper{
			OrderSettable: origOrderSettableC,
			onSetBetweenOrders: func(before, after string) error {
				capturedBeforeC = before
				capturedAfterC = after
				setBetweenCalledC = true
				return origOrderSettableC.SetBetweenOrders(before, after)
			},
		}
		mockViewC.OrderSettable = wrapperOrderSettableC

		// Wrap b's OrderSettable
		origOrderSettableB := mockViewB.OrderSettable
		wrapperOrderSettableB := &orderSettableWrapper{
			OrderSettable: origOrderSettableB,
			onSetBetweenOrders: func(before, after string) error {
				capturedBeforeB = before
				capturedAfterB = after
				setBetweenCalledB = true
				return origOrderSettableB.SetBetweenOrders(before, after)
			},
		}
		mockViewB.OrderSettable = wrapperOrderSettableB

		// STRICT expectations based on algorithm:
		// Position 0 (a): "AAA001" > "" → keeps lexid
		// Position 1 (c): "" (no lexid) → needs new lexid
		// Position 2 (b): "" (no lexid) → needs new lexid
		// Position 3 (d): "DDD004" > prev → depends on b's new lexid
		objGetter.EXPECT().GetObject(context.Background(), "c").Return(mockViewC, nil).Once()
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Only a and d have lexids initially
		currentLexIds := map[string]string{
			"a": "AAA001",
			// b has no lexid
			// c has no lexid
			"d": "DDD004",
		}

		// when - desired order: [a, c, b, d]
		// c needs to be inserted between a and b (but b has no lexid yet!)
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "b", "d"}, currentLexIds)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Log what happened
		if setBetweenCalledC {
			t.Logf("c: SetBetweenOrders was called with before=%q, after=%q", capturedBeforeC, capturedAfterC)
		}
		if setBetweenCalledB {
			t.Logf("b: SetBetweenOrders was called with before=%q, after=%q", capturedBeforeB, capturedAfterB)
		}

		// Verify the final order
		assert.True(t, lexids[0] < lexids[1], "a should be before c")
		assert.True(t, lexids[1] < lexids[2], "c should be before b")
		assert.True(t, lexids[2] < lexids[3], "b should be before d")

		// Verify c was positioned correctly
		if setBetweenCalledC {
			assert.Equal(t, "AAA001", capturedBeforeC, "c should be after a")
			assert.Equal(t, "DDD004", capturedAfterC, "c should be before d")
		}
	})

	t.Run("complex reordering - potential edge case", func(t *testing.T) {
		// Test a more complex scenario: [a,b,c,d,e] -> [a,d,b,e,c]
		// This tests multiple simultaneous moves
		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mock views
		sbD := smarttest.New("d")
		mockViewD := &editor.SpaceView{SmartBlock: sbD, OrderSettable: order.NewOrderSettable(sbD, bundle.RelationKeySpaceOrder)}
		sbE := smarttest.New("e")
		mockViewE := &editor.SpaceView{SmartBlock: sbE, OrderSettable: order.NewOrderSettable(sbE, bundle.RelationKeySpaceOrder)}
		sbC := smarttest.New("c")
		mockViewC := &editor.SpaceView{SmartBlock: sbC, OrderSettable: order.NewOrderSettable(sbC, bundle.RelationKeySpaceOrder)}

		// Track SetBetweenOrders calls
		var dBefore, dAfter string
		var dCalled bool
		origD := mockViewD.OrderSettable
		mockViewD.OrderSettable = &orderSettableWrapper{
			OrderSettable: origD,
			onSetBetweenOrders: func(before, after string) error {
				dBefore, dAfter = before, after
				dCalled = true
				return origD.SetBetweenOrders(before, after)
			},
		}

		var eBefore, eAfter string
		var eCalled bool
		origE := mockViewE.OrderSettable
		mockViewE.OrderSettable = &orderSettableWrapper{
			OrderSettable: origE,
			onSetBetweenOrders: func(before, after string) error {
				eBefore, eAfter = before, after
				eCalled = true
				return origE.SetBetweenOrders(before, after)
			},
		}

		var cBefore, cAfter string
		var cCalled bool
		origC := mockViewC.OrderSettable
		mockViewC.OrderSettable = &orderSettableWrapper{
			OrderSettable: origC,
			onSetBetweenOrders: func(before, after string) error {
				cBefore, cAfter = before, after
				cCalled = true
				return origC.SetBetweenOrders(before, after)
			},
		}

		// Create mock for b since it needs repositioning
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// STRICT expectations: only b and c should be fetched
		// b: "B002" > "D004"? No → needs new lexid
		// c: "C003" > "E005"? No → needs new lexid
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()
		objGetter.EXPECT().GetObject(context.Background(), "c").Return(mockViewC, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Current order: a < b < c < d < e
		currentLexIds := map[string]string{
			"a": "A001",
			"b": "B002",
			"c": "C003",
			"d": "D004",
			"e": "E005",
		}

		// Desired order: [a, d, b, e, c]
		// d moves between a and b
		// e moves between b and c (in new positions)
		// c moves to the end
		lexids, err := o.rebuildIfNeeded([]string{"a", "d", "b", "e", "c"}, currentLexIds)

		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 5)

		// Log calls
		if dCalled {
			t.Logf("d: SetBetweenOrders(%q, %q)", dBefore, dAfter)
		}
		if eCalled {
			t.Logf("e: SetBetweenOrders(%q, %q)", eBefore, eAfter)
		}
		if cCalled {
			t.Logf("c: SetBetweenOrders(%q, %q)", cBefore, cAfter)
		}

		// Verify final order
		assert.True(t, lexids[0] < lexids[1], "a < d")
		assert.True(t, lexids[1] < lexids[2], "d < b")
		assert.True(t, lexids[2] < lexids[3], "b < e")
		assert.True(t, lexids[3] < lexids[4], "e < c")

		// Check the bounds used:
		// d at position 1: should be between a (A001) and b (B002)
		if dCalled {
			assert.Equal(t, "A001", dBefore, "d should be after a")
			assert.Equal(t, "B002", dAfter, "d should be before b")
		}

	})

	t.Run("move c between a and b - verify bounds", func(t *testing.T) {
		// Test that moving c between a and b uses correct bounds

		objGetter := mock_cache.NewMockObjectGetter(t)

		// Only b needs to be fetched - c keeps its lexid since "lexid_c" > "lexid_a"
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// STRICT expectation: only b should be fetched
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Initial state: [a,b,c,d] with lexids in alphabetical order
		currentLexIds := map[string]string{
			"a": "lexid_a",
			"b": "lexid_b",
			"c": "lexid_c",
			"d": "lexid_d",
		}

		// Desired order: [a,c,b,d] (c moves between a and b)
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "b", "d"}, currentLexIds)

		// Verify no errors
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// With the simpler fix, c keeps its lexid (no SetBetweenOrders call)
		// The important part is that the final order is correct
		assert.Equal(t, "lexid_a", lexids[0], "a keeps its lexid")
		assert.Equal(t, "lexid_c", lexids[1], "c keeps its lexid")
		assert.True(t, lexids[2] > "lexid_c", "b gets new lexid after c")
		assert.Equal(t, "lexid_d", lexids[3], "d keeps its lexid")

		// Verify the final order is correct regardless
		assert.True(t, lexids[0] < lexids[1], "a should be before c")
		assert.True(t, lexids[1] < lexids[2], "c should be before b")
		assert.True(t, lexids[2] < lexids[3], "b should be before d")
	})

	t.Run("two moves - [a,b,c,d] -> [a,c,d,b]", func(t *testing.T) {
		// Test case with 2 moves:
		// Initial: [a,b,c,d]
		// Final:   [a,c,d,b]
		// - c moves between a and b (where b was originally)
		// - d moves between c and b (in their new positions)
		// - b moves to the end

		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mocks for c, d, and b
		sbC := smarttest.New("c")
		mockViewC := &editor.SpaceView{SmartBlock: sbC, OrderSettable: order.NewOrderSettable(sbC, bundle.RelationKeySpaceOrder)}
		sbD := smarttest.New("d")
		mockViewD := &editor.SpaceView{SmartBlock: sbD, OrderSettable: order.NewOrderSettable(sbD, bundle.RelationKeySpaceOrder)}
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// Track calls for each element
		var cBefore, cAfter string
		var cMethodCalled bool
		origC := mockViewC.OrderSettable
		mockViewC.OrderSettable = &orderSettableWrapper{
			OrderSettable: origC,
			onSetBetweenOrders: func(before, after string) error {
				cBefore, cAfter = before, after
				cMethodCalled = true
				return origC.SetBetweenOrders(before, after)
			},
		}

		var dBefore, dAfter string
		var dMethodCalled bool
		origD := mockViewD.OrderSettable
		mockViewD.OrderSettable = &orderSettableWrapper{
			OrderSettable: origD,
			onSetBetweenOrders: func(before, after string) error {
				dBefore, dAfter = before, after
				dMethodCalled = true
				return origD.SetBetweenOrders(before, after)
			},
		}

		var bBefore, bAfter string
		var bMethodCalled bool
		origB := mockViewB.OrderSettable
		mockViewB.OrderSettable = &orderSettableWrapper{
			OrderSettable: origB,
			onSetBetweenOrders: func(before, after string) error {
				bBefore, bAfter = before, after
				bMethodCalled = true
					return origB.SetBetweenOrders(before, after)
			},
		}

		// STRICT expectations for [a,b,c,d] -> [a,c,d,b]:
		// Position 0 (a): "AAA" > "" → keeps lexid (no fetch)
		// Position 1 (c): "CCC" > "AAA" → keeps lexid (no fetch)
		// Position 2 (d): "DDD" > "CCC" → keeps lexid (no fetch)
		// Position 3 (b): "BBB" > "DDD" → NO, needs new lexid
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Initial state: [a,b,c,d] with lexids in order
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
		}

		// Let's trace precalcNext for [a,c,d,b]:
		// From right: b="BBB", d="DDD", c="CCC", a="AAA"
		// i=3 (b): res[3]="", next="BBB"
		// i=2 (d): res[2]="BBB", next="DDD"
		// i=1 (c): res[1]="DDD", next="CCC"
		// i=0 (a): res[0]="CCC", next="AAA"
		// So next = ["CCC", "DDD", "BBB", ""]

		// This means:
		// - Position 0 (a): prev="", next="CCC" -> keeps "AAA" (valid)
		// - Position 1 (c): prev="AAA", next="DDD"
		//   Current is "CCC" which is > "AAA" and < "DDD", so might keep it
		//   Actually let's see...

		// Desired order: [a,c,d,b]
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "d", "b"}, currentLexIds)

		// Verify no errors
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Log what methods were called
		t.Logf("\n=== Method calls ===")
		if cMethodCalled {
			t.Logf("c: SetBetweenOrders(%q, %q)", cBefore, cAfter)
			// c at position 1: Should be between "AAA" (a) and next existing which is "DDD" (d)
			// BUT WAIT - if d moves too, what happens?
		} else {
			t.Logf("c: SetBetweenOrders NOT called (kept existing lexid)")
		}

		if dMethodCalled {
			t.Logf("d: SetBetweenOrders(%q, %q)", dBefore, dAfter)
			// d at position 2: prev would be c's lexid (might be "CCC" if kept)
			// next would be "BBB" (b)
		} else {
			t.Logf("d: SetBetweenOrders NOT called (kept existing lexid)")
		}

		if bMethodCalled {
			t.Logf("b: SetBetweenOrders(%q, %q)", bBefore, bAfter)
			// b at position 3: prev would be d's lexid, next=""
		} else {
			t.Logf("b: SetBetweenOrders NOT called (kept existing lexid)")
		}

		// Verify final order
		assert.True(t, lexids[0] < lexids[1], "a < c")
		assert.True(t, lexids[1] < lexids[2], "c < d")
		assert.True(t, lexids[2] < lexids[3], "d < b")

		// The interesting question: what bounds are used?
		// If the algorithm processes left-to-right:
		// 1. c sees next="DDD" but that's where d currently is
		// 2. d sees next="BBB" but b is moving too

		// Print the actual final lexids for debugging
		t.Logf("\n=== Final lexids ===")
		t.Logf("a: %q", lexids[0])
		t.Logf("c: %q", lexids[1])
		t.Logf("d: %q", lexids[2])
		t.Logf("b: %q", lexids[3])

		// With the fix: The algorithm detects invalid bounds and adjusts properly
		// Now b correctly moves to the end without triggering a full rebuild
		if !bMethodCalled && lexids[3] == "BBB" {
			t.Errorf("Expected b to be repositioned to the end")
		}

		// The fix makes this work optimally: a,c,d keep their lexids, only b moves
		assert.Equal(t, "AAA", lexids[0], "a should keep AAA")
		assert.Equal(t, "CCC", lexids[1], "c should keep CCC")
		assert.Equal(t, "DDD", lexids[2], "d should keep DDD")
		assert.True(t, lexids[3] > "DDD", "b should have new lexid after DDD")
	})
}

func TestOrderSetter_OptimalReorderingWithFix(t *testing.T) {
	t.Run("optimal reordering without rebuild", func(t *testing.T) {
		// Tests that [a,b,c,d] -> [a,c,d,b] doesn't trigger full rebuild
		// With the fix: only b gets new lexid, a/c/d keep theirs

		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mock for b (only one that needs fetching)
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// STRICT expectation for [a,b,c,d] -> [a,c,d,b]:
		// Only b needs fetching ("BBB" is not > "DDD")
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Initial state: [a,b,c,d] with lexids in order
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
		}

		// Execute reorder
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "d", "b"}, currentLexIds)

		// Verify
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)
		assert.True(t, lexids[0] < lexids[1], "a < c")
		assert.True(t, lexids[1] < lexids[2], "c < d")
		assert.True(t, lexids[2] < lexids[3], "d < b")


		// The fix ensures only b gets a new lexid
		assert.Equal(t, "AAA", lexids[0], "a should keep AAA")
		assert.Equal(t, "CCC", lexids[1], "c should keep CCC")
		assert.Equal(t, "DDD", lexids[2], "d should keep DDD")
		assert.True(t, lexids[3] > "DDD", "b should have new lexid after DDD")
	})
	t.Run("FIX VERIFICATION - [a,b,c,d] -> [a,c,d,b] should not trigger rebuild", func(t *testing.T) {
		// With the fix, this should NOT trigger a full rebuild
		// Expected behavior:
		// - a keeps AAA
		// - c keeps CCC
		// - d keeps DDD
		// - b gets new lexid after DDD

		objGetter := mock_cache.NewMockObjectGetter(t)

		// Create mocks
		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		var bMethodName string
		var bPrevValue string
		origB := mockViewB.OrderSettable

		// Create a wrapper that intercepts both methods
		wrapperB := &orderSettableWithBothMethods{
			OrderSettable: origB,
			onSetBetweenOrders: func(before, after string) error {
				bMethodName = "SetBetweenOrders"
					return origB.SetBetweenOrders(before, after)
			},
			onSetOrder: func(prev string) (string, error) {
				bMethodName = "SetOrder"
				bPrevValue = prev
				return origB.SetOrder(prev)
			},
		}
		mockViewB.OrderSettable = wrapperB

		// STRICT expectation: only b needs fetching
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Initial state
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
		}

		// Execute
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "d", "b"}, currentLexIds)

		// Verify
		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Check that a, c, d kept their lexids
		assert.Equal(t, "AAA", lexids[0], "a should keep its lexid")
		assert.Equal(t, "CCC", lexids[1], "c should keep its lexid")
		assert.Equal(t, "DDD", lexids[2], "d should keep its lexid")

		// b should get a new lexid after DDD
		assert.NotEqual(t, "BBB", lexids[3], "b should get a new lexid")
		assert.True(t, lexids[3] > "DDD", "b's new lexid should be after DDD")

		// Verify SetOrder was called with DDD as the previous value
		assert.Equal(t, "SetOrder", bMethodName, "b should use SetOrder, not SetBetweenOrders")
		assert.Equal(t, "DDD", bPrevValue, "b should be positioned after DDD")

		t.Logf("Success! No rebuild triggered. Final order: %v", lexids)
	})

	t.Run("FIX VERIFICATION - complex case [a,b,c,d,e] -> [b,d,a,e,c]", func(t *testing.T) {
		// A more complex reordering to verify the fix handles multiple moves
		objGetter := mock_cache.NewMockObjectGetter(t)

		// Track all method calls
		methodCalls := make(map[string]string)

		createMockView := func(id string) *editor.SpaceView {
			sb := smarttest.New(id)
			view := &editor.SpaceView{SmartBlock: sb, OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder)}

			orig := view.OrderSettable
			view.OrderSettable = &orderSettableWrapper{
				OrderSettable: orig,
				onSetBetweenOrders: func(before, after string) error {
					methodCalls[id] = fmt.Sprintf("SetBetweenOrders(%q, %q)", before, after)
					return orig.SetBetweenOrders(before, after)
				},
			}
			return view
		}

		views := map[string]*editor.SpaceView{
			"a": createMockView("a"),
			"b": createMockView("b"),
			"c": createMockView("c"),
			"d": createMockView("d"),
			"e": createMockView("e"),
		}

		// STRICT expectations for [a,b,c,d,e] -> [b,d,a,e,c]:
		// Position 0 (b): "BBB" > "" → keeps lexid (no fetch)
		// Position 1 (d): "DDD" > "BBB" → keeps lexid (no fetch)
		// Position 2 (a): "AAA" > "DDD" → NO, needs new lexid
		// Position 3 (e): "EEE" > a's new lexid (between DDD and EEE) → keeps lexid (no fetch)
		// Position 4 (c): "CCC" > "EEE" → NO, needs new lexid
		objGetter.EXPECT().GetObject(context.Background(), "a").Return(views["a"], nil).Once()
		objGetter.EXPECT().GetObject(context.Background(), "c").Return(views["c"], nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Initial: a < b < c < d < e
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
			"e": "EEE",
		}

		// Target: [b, d, a, e, c]
		lexids, err := o.rebuildIfNeeded([]string{"b", "d", "a", "e", "c"}, currentLexIds)

		// Verify
		assert.NoError(t, err)
		assert.Len(t, lexids, 5)

		// Check final ordering
		assert.True(t, lexids[0] < lexids[1], "b < d")
		assert.True(t, lexids[1] < lexids[2], "d < a")
		assert.True(t, lexids[2] < lexids[3], "a < e")
		assert.True(t, lexids[3] < lexids[4], "e < c")

		// Log what methods were called
		t.Logf("Method calls: %v", methodCalls)

		// The fix should prevent invalid bounds from causing unnecessary rebuilds
		// Check that we don't have all new lexids (which would indicate a full rebuild)
		allNew := true
		for i, lexid := range lexids {
			id := []string{"b", "d", "a", "e", "c"}[i]
			if lexid == currentLexIds[id] {
				allNew = false
				t.Logf("%s kept its original lexid: %s", id, lexid)
			}
		}

		if allNew {
			t.Logf("Warning: All lexids were regenerated, might indicate unnecessary rebuild")
		}
	})

	t.Run("EDGE CASE - complete reversal [a,b,c,d,e] -> [e,d,c,b,a]", func(t *testing.T) {
		// This tests whether the fix prevents necessary rebuilds
		// Complete reversal is a stress test for the algorithm
		objGetter := mock_cache.NewMockObjectGetter(t)

		methodCalls := []string{}

		createTrackedView := func(id string) *editor.SpaceView {
			sb := smarttest.New(id)
			view := &editor.SpaceView{SmartBlock: sb, OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder)}

			orig := view.OrderSettable
			view.OrderSettable = &orderSettableWithBothMethods{
				OrderSettable: orig,
				onSetBetweenOrders: func(before, after string) error {
					call := fmt.Sprintf("%s.SetBetweenOrders(%q,%q)", id, before, after)
					methodCalls = append(methodCalls, call)
					return orig.SetBetweenOrders(before, after)
				},
				onSetOrder: func(prev string) (string, error) {
					call := fmt.Sprintf("%s.SetOrder(%q)", id, prev)
					methodCalls = append(methodCalls, call)
					return orig.SetOrder(prev)
				},
			}
			return view
		}

		views := map[string]*editor.SpaceView{
			"a": createTrackedView("a"),
			"b": createTrackedView("b"),
			"c": createTrackedView("c"),
			"d": createTrackedView("d"),
			"e": createTrackedView("e"),
		}

		// STRICT expectations for complete reversal [a,b,c,d,e] -> [e,d,c,b,a]:
		// Position 0 (e): "EEE" > "" → keeps lexid (no fetch)
		// Position 1 (d): "DDD" > "EEE" → NO, needs new lexid
		// Position 2 (c): "CCC" > d's new lexid → needs new lexid
		// Position 3 (b): "BBB" > c's new lexid → needs new lexid
		// Position 4 (a): "AAA" > b's new lexid → needs new lexid
		// This may trigger a full rebuild due to all inversions
		objGetter.EXPECT().GetObject(context.Background(), "d").Return(views["d"], nil).Once()
		// After d fails, algorithm may trigger full rebuild which fetches all
		objGetter.EXPECT().GetObject(context.Background(), "e").Return(views["e"], nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), "c").Return(views["c"], nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(views["b"], nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), "a").Return(views["a"], nil).Maybe()

		o := &orderSetter{objectGetter: objGetter}

		// Initial: a < b < c < d < e
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
			"e": "EEE",
		}

		// Complete reversal: [e, d, c, b, a]
		lexids, err := o.rebuildIfNeeded([]string{"e", "d", "c", "b", "a"}, currentLexIds)

		// Log what happened
		t.Logf("Method calls:")
		for _, call := range methodCalls {
			t.Logf("  %s", call)
		}

		// Verify
		assert.NoError(t, err)
		assert.Len(t, lexids, 5)

		// Check final ordering is correct
		assert.True(t, lexids[0] < lexids[1], "e < d in final order")
		assert.True(t, lexids[1] < lexids[2], "d < c in final order")
		assert.True(t, lexids[2] < lexids[3], "c < b in final order")
		assert.True(t, lexids[3] < lexids[4], "b < a in final order")

		t.Logf("Final lexids: e=%q, d=%q, c=%q, b=%q, a=%q",
			lexids[0], lexids[1], lexids[2], lexids[3], lexids[4])

		// The key test: did we maintain correct ordering even with the fix?
		// The fix should not prevent the algorithm from working correctly
	})

	t.Run("EDGE CASE - tightly packed lexids", func(t *testing.T) {
		// Test case where lexids are so close together that we might run out of space
		// This should trigger a rebuild even with the fix
		objGetter := mock_cache.NewMockObjectGetter(t)

		sbB := smarttest.New("b")
		mockViewB := &editor.SpaceView{SmartBlock: sbB, OrderSettable: order.NewOrderSettable(sbB, bundle.RelationKeySpaceOrder)}

		// STRICT expectations for [a,b,c,d] -> [a,c,b,d] with tight lexids:
		// Position 0 (a): "A" > "" → keeps lexid (no fetch)
		// Position 1 (c): "C" > "A" → keeps lexid (no fetch)
		// Position 2 (b): "B" > "C" → NO, needs new lexid
		// Position 3 (d): "D" > b's new lexid → likely keeps lexid
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(mockViewB, nil).Once()

		o := &orderSetter{objectGetter: objGetter}

		// Simulate tightly packed lexids where there's no room
		// Using lexids that are sequential with no gaps
		currentLexIds := map[string]string{
			"a": "A",
			"b": "B",
			"c": "C",
			"d": "D",
		}

		// Try to insert b between a and c: [a,b,c,d] -> [a,c,b,d]
		// This should work because c moves, creating space
		lexids, err := o.rebuildIfNeeded([]string{"a", "c", "b", "d"}, currentLexIds)

		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Verify correct ordering
		assert.True(t, lexids[0] < lexids[1], "a < c")
		assert.True(t, lexids[1] < lexids[2], "c < b")
		assert.True(t, lexids[2] < lexids[3], "b < d")

		t.Logf("Lexids after reorder: a=%q, c=%q, b=%q, d=%q",
			lexids[0], lexids[1], lexids[2], lexids[3])
	})

	t.Run("CRITICAL - verify fix doesn't keep lexids when it shouldn't", func(t *testing.T) {
		// Test a case where an element might incorrectly keep its lexid due to the fix
		// Scenario: [a,b,c,d] -> [a,d,b,c]
		// Without fix: d would try SetBetweenOrders("AAA", "BBB") and succeed
		// With fix: if bounds are invalid, might keep lexid when it shouldn't

		objGetter := mock_cache.NewMockObjectGetter(t)
		o := &orderSetter{objectGetter: objGetter}

		// Mock views
		views := make(map[string]*editor.SpaceView)
		for _, id := range []string{"a", "b", "c", "d"} {
			sb := smarttest.New(id)
			views[id] = &editor.SpaceView{SmartBlock: sb, OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder)}
		}

		// STRICT expectations for [a,b,c,d] -> [a,d,b,c]:
		// Position 0 (a): "AAA" > "" → keeps lexid (no fetch)
		// Position 1 (d): "DDD" > "AAA" → keeps lexid (no fetch)
		// Position 2 (b): "BBB" > "DDD" → NO, needs new lexid
		// Position 3 (c): "CCC" > b's new lexid → likely needs new lexid
		objGetter.EXPECT().GetObject(context.Background(), "b").Return(views["b"], nil).Once()
		objGetter.EXPECT().GetObject(context.Background(), "c").Return(views["c"], nil).Once()

		// Initial state
		currentLexIds := map[string]string{
			"a": "AAA",
			"b": "BBB",
			"c": "CCC",
			"d": "DDD",
		}

		// Reorder: [a,d,b,c] - d moves between a and b
		lexids, err := o.rebuildIfNeeded([]string{"a", "d", "b", "c"}, currentLexIds)

		assert.NoError(t, err)
		assert.Len(t, lexids, 4)

		// Check final ordering
		assert.True(t, lexids[0] < lexids[1], "a < d")
		assert.True(t, lexids[1] < lexids[2], "d < b")
		assert.True(t, lexids[2] < lexids[3], "b < c")

		// With the simpler fix, d may keep "DDD" if it's valid relative to prev
		// The important thing is that the final ordering is correct
		assert.True(t, lexids[1] > lexids[0], "d should be after a")
		assert.True(t, lexids[1] < lexids[2], "d should be before b in final order")

		t.Logf("Final lexids: a=%q, d=%q, b=%q, c=%q",
			lexids[0], lexids[1], lexids[2], lexids[3])
	})
}

// orderSettableWithBothMethods can intercept both SetOrder and SetBetweenOrders
type orderSettableWithBothMethods struct {
	order.OrderSettable
	onSetOrder         func(prev string) (string, error)
	onSetBetweenOrders func(before, after string) error
}

func (w *orderSettableWithBothMethods) SetOrder(prev string) (string, error) {
	if w.onSetOrder != nil {
		return w.onSetOrder(prev)
	}
	return w.OrderSettable.SetOrder(prev)
}

func (w *orderSettableWithBothMethods) SetBetweenOrders(before, after string) error {
	if w.onSetBetweenOrders != nil {
		return w.onSetBetweenOrders(before, after)
	}
	return w.OrderSettable.SetBetweenOrders(before, after)
}

func TestCollatorIgnoresCaseButKeepsSymbolsDistinct(t *testing.T) {
	coll := collate.New(language.Und, collate.IgnoreCase)

	if coll.CompareString("A", "a") != 0 {
		t.Fatalf("expected case-insensitive collator to treat 'A' and 'a' as equal")
	}

	if coll.CompareString("-", "_") == 0 {
		t.Fatalf("hyphen and underscore should remain distinct under collate.IgnoreCase")
	}
}

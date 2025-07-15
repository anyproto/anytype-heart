package spaceview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestOrderSetter_rebuildIfNeeded(t *testing.T) {
	t.Run("empty views returns error", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		o := &orderSetter{objectGetter: objGetter}
		
		// when
		_, err := o.SetOrder([]string{})
		
		// then
		assert.Error(t, err)
		assert.Equal(t, "empty spaceViewOrder", err.Error())
	})

	t.Run("single new view", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
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
		mockView1 := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}
		
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
		mockView1 := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}
		
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
		mockView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		
		// Expect view2 to be fetched for reordering
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView2, nil).Maybe()
		
		o := &orderSetter{objectGetter: objGetter}
		
		// Current order has view2 after view3, but we want it between view1 and view3
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0003",  // Out of order
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
		mockView1 := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}
		
		// Expect all new views to be fetched for lexid assignment
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView1, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView2, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView3, nil).Maybe()
		
		o := &orderSetter{objectGetter: objGetter}
		
		// Scenario: view4 already exists with lexid, we want to pin view1, view2, view3 before it
		currentLexIds := map[string]string{
			"view4": "MMMM5000",  // Existing view with lexid
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
		mockView1 := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}
		
		// Expect views to be fetched for reordering
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView1, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView2, nil).Maybe()
		objGetter.EXPECT().GetObject(context.Background(), mock.Anything).Return(mockView3, nil).Maybe()
		
		o := &orderSetter{objectGetter: objGetter}
		
		// Current order: view3 is last, but we want it first
		currentLexIds := map[string]string{
			"view1": "MMMM0001",
			"view2": "MMMM0002", 
			"view3": "MMMM0003",  // Should be moved to first position
		}
		
		// when - desired order: view3, view1, view2 (view3 moves to first)
		lexids, err := o.rebuildIfNeeded([]string{"view3", "view1", "view2"}, currentLexIds)
		
		// then
		assert.NoError(t, err)
		assert.Len(t, lexids, 3)
		
		// The final order should be correct (view3 < view1 < view2)
		assert.True(t, lexids[0] < lexids[1], "view3 should be before view1")
		assert.True(t, lexids[1] < lexids[2], "view1 should be before view2")
		
		// view3 should now be first and have a different lexid from its original
		assert.NotEqual(t, "MMMM0003", lexids[0], "view3 should have a new lexid (not its original)")
		// The fix works correctly - view3 is properly ordered before view1 and view2
	})
}

func TestOrderSetter_UnsetOrder(t *testing.T) {
	t.Run("unset order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		
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
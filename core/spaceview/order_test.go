package spaceview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestOrderSetter_SetSpaceViewOrder(t *testing.T) {
	t.Run("insufficient view", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		o := &orderSetter{objectGetter: objGetter}

		// when
		err := o.SetOrder("view1", []string{})

		// then
		assert.NotNil(t, err)
		assert.Equal(t, "insufficient space views for reordering", err.Error())
	})
	t.Run("single view is pinned", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		o := &orderSetter{objectGetter: objGetter}

		// when
		err := o.SetOrder("view1", []string{"view1"})

		// then
		assert.Nil(t, err)
		assert.NotEmpty(t, mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder))
	})
	t.Run("move view at the beginning", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err := o.SetOrder("view1", []string{"view1", "view2"})

		// then
		assert.Nil(t, err)
		assert.NotEmpty(t, mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder))
	})

	t.Run("move view at the beginning, order exists", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		view2Order, err := mockSpaceView2.SetOrder("")
		assert.Nil(t, err)

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.SetOrder("view1", []string{"view1", "view2", "view3"})

		// then
		assert.Nil(t, err)
		assert.NotEmpty(t, mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder))

		view1Order := mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.True(t, view1Order < view2Order)
	})

	t.Run("move view at the end", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockSpaceView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockSpaceView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err := o.SetOrder("view3", []string{"view1", "view2", "view3"})

		// then
		assert.Nil(t, err)
		assert.NotEmpty(t, mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder))

		view1Order := mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder)
		view2Order := mockSpaceView2.Details().GetString(bundle.RelationKeySpaceOrder)
		view3Order := mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.True(t, view1Order < view2Order)
		assert.True(t, view2Order < view3Order)
	})
	t.Run("move view at the end, order exists", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockSpaceView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}

		view1Order, err := mockSpaceView.SetOrder("")
		view2Order, err := mockSpaceView2.SetOrder(view1Order)
		assert.Nil(t, err)

		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockSpaceView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.SetOrder("view3", []string{"view1", "view2", "view3"})

		// then
		assert.Nil(t, err)
		view3Order := mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.True(t, view2Order < view3Order)
	})
	t.Run("set view between: no order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockSpaceView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockSpaceView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err := o.SetOrder("view3", []string{"view1", "view3", "view2"})

		// then
		assert.Nil(t, err)
		view1Order := mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder)
		view2Order := mockSpaceView2.Details().GetString(bundle.RelationKeySpaceOrder)
		view3Order := mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder)

		assert.True(t, view1Order < view3Order)
		assert.Empty(t, view2Order)
	})
	t.Run("set view between: next view doesn't have order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockSpaceView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}

		view1Order, err := mockSpaceView.SetOrder("")
		assert.Nil(t, err)

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockSpaceView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.SetOrder("view3", []string{"view1", "view3", "view2"})

		// then
		assert.Nil(t, err)
		view3Order := mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder)
		view2Order := mockSpaceView2.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.True(t, view1Order < view3Order)
		assert.Empty(t, view2Order)
	})
	t.Run("set view between: order exists", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		mockSpaceView2 := &editor.SpaceView{SmartBlock: smarttest.New("view2")}
		mockSpaceView3 := &editor.SpaceView{SmartBlock: smarttest.New("view3")}

		view1Order, err := mockSpaceView.SetOrder("")
		view2Order, err := mockSpaceView2.SetOrder(view1Order)
		assert.Nil(t, err)

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view2").Return(mockSpaceView2, nil)
		objGetter.EXPECT().GetObject(context.Background(), "view3").Return(mockSpaceView3, nil)

		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.SetOrder("view3", []string{"view1", "view3", "view2"})

		// then
		assert.Nil(t, err)
		view3Order := mockSpaceView3.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.True(t, view1Order < view3Order)
		assert.True(t, view3Order < view2Order)
	})
}

func TestOrderSetter_UnsetOrder(t *testing.T) {
	t.Run("unset order", func(t *testing.T) {
		// given
		objGetter := mock_cache.NewMockObjectGetter(t)
		mockSpaceView := &editor.SpaceView{SmartBlock: smarttest.New("view1")}
		view1Order, err := mockSpaceView.SetOrder("")
		assert.Nil(t, err)

		objGetter.EXPECT().GetObject(context.Background(), "view1").Return(mockSpaceView, nil)
		o := &orderSetter{objectGetter: objGetter}

		// when
		err = o.UnsetOrder("view1")

		// then
		assert.Nil(t, err)
		view1Order = mockSpaceView.Details().GetString(bundle.RelationKeySpaceOrder)
		assert.Empty(t, view1Order)
	})
}

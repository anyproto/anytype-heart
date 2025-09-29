package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
)

const orderKey = "key"

func newTestObject(id string) *orderSettable {
	sb := smarttest.New(id)
	return &orderSettable{
		SmartBlock: sb,
		orderKey:   orderKey,
	}
}

func TestRelationOption_SetOrder(t *testing.T) {
	t.Run("set order without previous order id creates middle lexid", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		// when
		orderId, err := obj.SetOrder("")

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, orderId, savedOrderId)

		expectedMiddle := lx.Middle()
		assert.Equal(t, expectedMiddle, orderId)
	})

	t.Run("set order with previous order id creates next lexid", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")
		previousOrderId := "prev-order-id"

		// when
		orderId, err := obj.SetOrder(previousOrderId)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, orderId, savedOrderId)

		expectedNext := lx.Next(previousOrderId)
		assert.Equal(t, expectedNext, orderId)
	})

	t.Run("set order updates existing order", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		st := obj.NewState()
		st.SetDetail(orderKey, domain.String("initial-order"))
		err := obj.Apply(st)
		require.NoError(t, err)

		previousOrderId := "new-prev-order"

		// when
		orderId, err := obj.SetOrder(previousOrderId)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, orderId, savedOrderId)
		assert.NotEqual(t, "initial-order", savedOrderId)
	})
}

func TestRelationOption_SetAfterOrder(t *testing.T) {
	t.Run("sets order after given order id when current order is smaller", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		initialOrder := "A"
		st := obj.NewState()
		st.SetDetail(orderKey, domain.String(initialOrder))
		err := obj.Apply(st)
		require.NoError(t, err)

		targetOrderId := "Z"

		// when
		err = obj.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)
		savedOrderId := obj.Details().GetString(orderKey)
		expectedNext := lx.Next(targetOrderId)
		assert.Equal(t, expectedNext, savedOrderId)
	})

	t.Run("does not update order when current order is greater than or equal to target", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		initialOrder := "Z"
		st := obj.NewState()
		st.SetDetail(orderKey, domain.String(initialOrder))
		err := obj.Apply(st)
		require.NoError(t, err)

		targetOrderId := "A"

		// when
		err = obj.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, initialOrder, savedOrderId)
	})

	t.Run("handles empty current order", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")
		targetOrderId := "target-order"

		// when
		err := obj.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := obj.Details().GetString(orderKey)
		expectedNext := lx.Next(targetOrderId)
		assert.Equal(t, expectedNext, savedOrderId)
	})
}

func TestRelationOption_SetBetweenOrders(t *testing.T) {
	t.Run("sets order between two existing orders", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		previousOrderId := "A"
		afterOrderId := "Z"

		// when
		got, err := obj.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.NoError(t, err)
		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, got, savedOrderId)
		assert.NotEmpty(t, savedOrderId)
		assert.True(t, savedOrderId > previousOrderId)
		assert.True(t, savedOrderId < afterOrderId)
	})

	t.Run("sets order before first element when previous order is empty", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		previousOrderId := ""
		afterOrderId := "M"

		// when
		got, err := obj.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := obj.Details().GetString(orderKey)
		assert.Equal(t, got, savedOrderId)
		expectedPrev := lx.Prev(afterOrderId)
		assert.Equal(t, expectedPrev, savedOrderId)
		assert.True(t, savedOrderId < afterOrderId)
	})

	t.Run("returns error when lexid insertion fails", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")
		previousOrderId := "A"
		afterOrderId := "A"

		// when
		_, err := obj.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrLexidInsertionFailed)
	})
}

func TestRelationOption_UnsetOrder(t *testing.T) {
	t.Run("removes order detail", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		st := obj.NewState()
		st.SetDetail(orderKey, domain.String("some-order"))
		err := obj.Apply(st)
		require.NoError(t, err)
		require.NotEmpty(t, obj.Details().GetString(orderKey))

		// when
		err = obj.UnsetOrder()

		// then
		assert.NoError(t, err)
		savedOrderId := obj.Details().GetString(orderKey)
		assert.Empty(t, savedOrderId)
	})

	t.Run("succeeds when order is already empty", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		require.Empty(t, obj.Details().GetString(orderKey))

		// when
		err := obj.UnsetOrder()

		// then
		assert.NoError(t, err)
		savedOrderId := obj.Details().GetString(orderKey)
		assert.Empty(t, savedOrderId)
	})
}

func TestRelationOption_OrderOperationsIntegration(t *testing.T) {
	t.Run("complete order management workflow", func(t *testing.T) {
		// given
		obj := newTestObject("test-relation-option")

		// Test 1: Set initial order
		orderId1, err := obj.SetOrder("")
		require.NoError(t, err)
		assert.Equal(t, orderId1, obj.GetOrder())

		// Test 2: Set order after another
		err = obj.SetAfterOrder("A")
		require.NoError(t, err)
		newOrder := obj.GetOrder()
		assert.True(t, newOrder > "A")

		// Test 3: Set order between two orders
		_, err = obj.SetBetweenOrders("B", "Z")
		require.NoError(t, err)
		betweenOrder := obj.GetOrder()
		assert.True(t, betweenOrder > "B" && betweenOrder < "Z")

		// Test 4: Unset order
		err = obj.UnsetOrder()
		require.NoError(t, err)
		assert.Empty(t, obj.GetOrder())
	})
}

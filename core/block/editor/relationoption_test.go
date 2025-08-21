package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func newTestRelationOption(id string) *RelationOption {
	sb := smarttest.New(id)
	sb.SetType(smartblock.SmartBlockTypeRelationOption)
	return &RelationOption{
		SmartBlock: sb,
	}
}

func TestRelationOption_SetOrder(t *testing.T) {
	t.Run("set order without previous order id creates middle lexid", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// when
		orderId, err := ro.SetOrder("")

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)

		expectedMiddle := lx.Middle()
		assert.Equal(t, expectedMiddle, orderId)
	})

	t.Run("set order with previous order id creates next lexid", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")
		previousOrderId := "prev-order-id"

		// when
		orderId, err := ro.SetOrder(previousOrderId)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)

		expectedNext := lx.Next(previousOrderId)
		assert.Equal(t, expectedNext, orderId)
	})

	t.Run("set order updates existing order", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		st := ro.NewState()
		st.SetDetail(bundle.RelationKeyOptionOrder, domain.String("initial-order"))
		err := ro.Apply(st)
		require.NoError(t, err)

		previousOrderId := "new-prev-order"

		// when
		orderId, err := ro.SetOrder(previousOrderId)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)
		assert.NotEqual(t, "initial-order", savedOrderId)
	})
}

func TestRelationOption_SetAfterOrder(t *testing.T) {
	t.Run("sets order after given order id when current order is smaller", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		initialOrder := "A"
		st := ro.NewState()
		st.SetDetail(bundle.RelationKeyOptionOrder, domain.String(initialOrder))
		err := ro.Apply(st)
		require.NoError(t, err)

		targetOrderId := "Z"

		// when
		err = ro.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedNext := lx.Next(targetOrderId)
		assert.Equal(t, expectedNext, savedOrderId)
	})

	t.Run("does not update order when current order is greater than or equal to target", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		initialOrder := "Z"
		st := ro.NewState()
		st.SetDetail(bundle.RelationKeyOptionOrder, domain.String(initialOrder))
		err := ro.Apply(st)
		require.NoError(t, err)

		targetOrderId := "A"

		// when
		err = ro.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, initialOrder, savedOrderId)
	})

	t.Run("handles empty current order", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")
		targetOrderId := "target-order"

		// when
		err := ro.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedNext := lx.Next(targetOrderId)
		assert.Equal(t, expectedNext, savedOrderId)
	})
}

func TestRelationOption_SetBetweenOrders(t *testing.T) {
	t.Run("sets order between two existing orders", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		previousOrderId := "A"
		afterOrderId := "Z"

		// when
		err := ro.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.NoError(t, err)
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.NotEmpty(t, savedOrderId)
		assert.True(t, savedOrderId > previousOrderId)
		assert.True(t, savedOrderId < afterOrderId)
	})

	t.Run("sets order before first element when previous order is empty", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		previousOrderId := ""
		afterOrderId := "M"

		// when
		err := ro.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.NoError(t, err)

		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedPrev := lx.Prev(afterOrderId)
		assert.Equal(t, expectedPrev, savedOrderId)
		assert.True(t, savedOrderId < afterOrderId)
	})

	t.Run("returns error when lexid insertion fails", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")
		previousOrderId := "A"
		afterOrderId := "A"

		// when
		err := ro.SetBetweenOrders(previousOrderId, afterOrderId)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrLexidInsertionFailed)
	})
}

func TestRelationOption_UnsetOrder(t *testing.T) {
	t.Run("removes order detail", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		st := ro.NewState()
		st.SetDetail(bundle.RelationKeyOptionOrder, domain.String("some-order"))
		err := ro.Apply(st)
		require.NoError(t, err)
		require.NotEmpty(t, ro.Details().GetString(bundle.RelationKeyOptionOrder))

		// when
		err = ro.UnsetOrder()

		// then
		assert.NoError(t, err)
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Empty(t, savedOrderId)
	})

	t.Run("succeeds when order is already empty", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		require.Empty(t, ro.Details().GetString(bundle.RelationKeyOptionOrder))

		// when
		err := ro.UnsetOrder()

		// then
		assert.NoError(t, err)
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Empty(t, savedOrderId)
	})
}

func TestRelationOption_OrderOperationsIntegration(t *testing.T) {
	t.Run("complete order management workflow", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Test 1: Set initial order
		orderId1, err := ro.SetOrder("")
		require.NoError(t, err)
		assert.Equal(t, orderId1, ro.GetOrder())

		// Test 2: Set order after another
		err = ro.SetAfterOrder("A")
		require.NoError(t, err)
		newOrder := ro.GetOrder()
		assert.True(t, newOrder > "A")

		// Test 3: Set order between two orders
		err = ro.SetBetweenOrders("B", "Z")
		require.NoError(t, err)
		betweenOrder := ro.GetOrder()
		assert.True(t, betweenOrder > "B" && betweenOrder < "Z")

		// Test 4: Unset order
		err = ro.UnsetOrder()
		require.NoError(t, err)
		assert.Empty(t, ro.GetOrder())
	})
}

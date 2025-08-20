package editor

import (
	"testing"

	"github.com/anyproto/lexid"
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

		// Verify the order is set in details
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)

		// Verify it's a valid lexid middle value
		expectedMiddle := optionOrderLX.Middle()
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

		// Verify the order is set in details
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)

		// Verify it's the next lexid after previous
		expectedNext := optionOrderLX.Next(previousOrderId)
		assert.Equal(t, expectedNext, orderId)
	})

	t.Run("set order updates existing order", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Set initial order
		initialState := ro.NewState()
		initialState.SetDetail(bundle.RelationKeyOptionOrder, domain.String("initial-order"))
		ro.Apply(initialState)

		previousOrderId := "new-prev-order"

		// when
		orderId, err := ro.SetOrder(previousOrderId)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, orderId)

		// Verify the order is updated
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Equal(t, orderId, savedOrderId)
		assert.NotEqual(t, "initial-order", savedOrderId)
	})
}

func TestRelationOption_SetAfterOrder(t *testing.T) {
	t.Run("sets order after given order id when current order is smaller", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Set initial order smaller than target
		initialOrder := "A"
		initialState := ro.NewState()
		initialState.SetDetail(bundle.RelationKeyOptionOrder, domain.String(initialOrder))
		ro.Apply(initialState)

		targetOrderId := "Z" // Lexicographically after "A"

		// when
		err := ro.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		// Verify the order is updated to next after target
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedNext := optionOrderLX.Next(targetOrderId)
		assert.Equal(t, expectedNext, savedOrderId)
	})

	t.Run("does not update order when current order is greater than or equal to target", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Set initial order greater than target
		initialOrder := "Z"
		initialState := ro.NewState()
		initialState.SetDetail(bundle.RelationKeyOptionOrder, domain.String(initialOrder))
		ro.Apply(initialState)

		targetOrderId := "A" // Lexicographically before "Z"

		// when
		err := ro.SetAfterOrder(targetOrderId)

		// then
		assert.NoError(t, err)

		// Verify the order remains unchanged
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

		// Verify the order is set to next after target
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedNext := optionOrderLX.Next(targetOrderId)
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

		// Verify order is set between the two orders
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.NotEmpty(t, savedOrderId)

		// The order should be between previous and after
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

		// Verify order is set before the after order
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		expectedPrev := optionOrderLX.Prev(afterOrderId)
		assert.Equal(t, expectedPrev, savedOrderId)
		assert.True(t, savedOrderId < afterOrderId)
	})

	t.Run("returns error when lexid insertion fails", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Use orders that would cause lexid insertion to fail
		// This happens when the orders are consecutive and can't have anything between them
		previousOrderId := "A"
		afterOrderId := "A" // Same as previous - should cause conflict

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

		// Set initial order
		initialState := ro.NewState()
		initialState.SetDetail(bundle.RelationKeyOptionOrder, domain.String("some-order"))
		ro.Apply(initialState)

		// Verify order exists initially
		assert.NotEmpty(t, ro.Details().GetString(bundle.RelationKeyOptionOrder))

		// when
		err := ro.UnsetOrder()

		// then
		assert.NoError(t, err)

		// Verify order is removed
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Empty(t, savedOrderId)
	})

	t.Run("succeeds when order is already empty", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// Verify order is empty initially
		assert.Empty(t, ro.Details().GetString(bundle.RelationKeyOptionOrder))

		// when
		err := ro.UnsetOrder()

		// then
		assert.NoError(t, err)

		// Verify order remains empty
		savedOrderId := ro.Details().GetString(bundle.RelationKeyOptionOrder)
		assert.Empty(t, savedOrderId)
	})
}

func TestRelationOption_GetOrder(t *testing.T) {
	t.Run("returns empty string when no order is set", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		// when
		order := ro.GetOrder()

		// then
		assert.Empty(t, order)
	})

	t.Run("returns order when set", func(t *testing.T) {
		// given
		ro := newTestRelationOption("test-relation-option")

		expectedOrder := "test-order-123"
		initialState := ro.NewState()
		initialState.SetDetail(bundle.RelationKeyOptionOrder, domain.String(expectedOrder))
		ro.Apply(initialState)

		// when
		order := ro.GetOrder()

		// then
		assert.Equal(t, expectedOrder, order)
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

func TestLexidOrdering(t *testing.T) {
	t.Run("lexid ordering properties", func(t *testing.T) {
		// Test the lexid library behavior that our methods depend on
		lx := lexid.Must(lexid.CharsBase64, 4, 4000)

		// Test middle generation
		middle := lx.Middle()
		assert.NotEmpty(t, middle)

		// Test next generation
		next := lx.Next(middle)
		assert.True(t, next > middle)

		// Test prev generation
		prev := lx.Prev(middle)
		assert.True(t, prev < middle)

		// Test ordering between elements
		between, err := lx.NextBefore(prev, next)
		assert.NoError(t, err)
		assert.True(t, between > prev && between < next)
	})
}

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

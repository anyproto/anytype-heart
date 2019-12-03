package base

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIcon_Diff(t *testing.T) {
	newIcon := func() IconBlock {
		return NewIcon(&model.Block{
			Id:           "1",
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfIcon{Icon: &model.BlockContentIcon{Name: "testicon"}},
		}).(IconBlock)
	}
	t.Run("equals", func(t *testing.T) {
		a := newIcon()
		b := newIcon()
		d, e := a.Diff(b)
		require.NoError(t, e)
		assert.Len(t, d, 0)
	})
	t.Run("diff", func(t *testing.T) {
		a := newIcon()
		b := newIcon()
		b.Model().Restrictions.Read = true
		b.Model().GetIcon().Name = "othername"
		d, e := a.Diff(b)
		require.NoError(t, e)
		assert.Len(t, d, 2)
	})
}

package relation

import (
	"github.com/anyproto/anytype-heart/core/block/simple/test"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelation_Diff(t *testing.T) {
	testBlock := func() *Relation {
		return NewRelation(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}},
		}).(*Relation)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff(b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.content.Key = "1"
		b2.content.Key = "1"
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = true
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Key = "42"

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetRelation{
			BlockSetRelation: &pb.EventBlockSetRelation{
				Id:  b1.Id,
				Key: &pb.EventBlockSetRelationKey{Value: "42"},
			},
		}), diff)
	})
}

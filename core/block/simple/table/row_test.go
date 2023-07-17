package table

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiff(t *testing.T) {
	testBlock := func() *rowBlock {
		return NewRowBlock(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}},
		}).(*rowBlock)
	}
	t.Run("layout", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.IsHeader = true

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetTableRow).BlockSetTableRow.IsHeader
		assert.Equal(t, true, change.Value)
	})
}

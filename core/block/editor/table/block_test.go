package table

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source *state.State
		want   *state.State
	}{
		{
			name:   "empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			want:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
		},
		{
			name: "invalid ids",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-c11", "row1-col2"},
				{"row2-col3"},
			}),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col2"},
				{},
			}),
		},
		{
			name: "wrong order",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col3", "row1-col1", "row1-col2"},
				{"row2-col3", "row2-c1", "row2-col1"},
			}),
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2", "row1-col3"},
				{"row2-col1", "row2-col3"},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb, err := NewTable(tc.source, "table")

			require.NoError(t, err)

			st := tc.source.Copy()
			err = tb.block.(Block).Normalize(st)
			require.NoError(t, err)

			assert.Equal(t, tc.want.Blocks(), st.Blocks())
		})
	}
}

func TestDuplicate(t *testing.T) {
	s := mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
		[][]string{
			{"row1-col1", "row1-col3"},
			{"row2-col1", "row2-col2"},
		}, withBlockContents(map[string]*model.Block{
			"row1-col1": mkTextBlock("11"),
			"row1-col3": mkTextBlock("13"),
			"row2-col1": mkTextBlock("21"),
			"row2-col2": mkTextBlock("22"),
		}))
	old, err := NewTable(s, "table")
	require.NoError(t, err)

	b := block{
		Base: base.NewBase(&model.Block{Id: "table"}).(*base.Base),
	}

	newId, err := b.Duplicate(s)

	require.NoError(t, err)

	got, err := NewTable(s, newId)

	require.NoError(t, err)

	assertNotEqual := func(old, new *model.Block) {
		assert.NotEmpty(t, new.Id)
		assert.NotEqual(t, old.Id, new.Id)
		assert.Equal(t, len(old.ChildrenIds), len(new.ChildrenIds))
		assert.NotEqual(t, old.ChildrenIds, new.ChildrenIds)
	}
	assertNotEqual(old.block.Model(), got.block.Model())
	assertNotEqual(old.Columns(), got.Columns())
	assertNotEqual(old.Rows(), got.Rows())
	for i, oldId := range old.Rows().ChildrenIds {
		newId := got.Rows().ChildrenIds[i]

		oldRow := s.Pick(oldId)
		newRow := s.Pick(newId)

		assertNotEqual(oldRow.Model(), newRow.Model())
	}
}

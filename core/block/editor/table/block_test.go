package table

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source *state.State
		want   *state.State
	}{
		{
			name:   "empty table should remain empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			want:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
		},
		{
			name: "cells with invalid ids are moved under the table",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-c11", "row1-col2"},
				{"row2-col3", "cell"},
			}),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-c11", "row1-col2"},
				{"row2-col3", "cell"},
			}, withChangedChildren(map[string][]string{
				"root": {"table", "row2-col3", "cell", "row1-c11"},
				"row1": {"row1-col2"},
				"row2": {},
			})),
		},
		{
			name: "wrong cells order -> do sorting and move invalid cells under the table",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col3", "row1-col1", "row1-col2"},
				{"row2-col3", "row2-c1", "row2-col1"},
			}),
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2", "row1-col3"},
				{"row2-col3", "row2-c1", "row2-col1"},
			}, withChangedChildren(map[string][]string{
				"root": {"table", "row2-c1"},
				"row2": {"row2-col1", "row2-col3"},
			})),
		},
		{
			name: "wrong place for header rows -> do sorting",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2", "row3"}, nil,
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row3": {IsHeader: true},
				})),
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row3", "row1", "row2"}, nil,
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row3": {IsHeader: true},
				})),
		},
		{
			name: "cell is a child of rows, not row -> move under the table",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"},
			}, withChangedChildren(map[string][]string{
				"rows": {"row1", "row1-col2", "row2"},
				"row1": {"row1-col1"},
			})),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"},
			}, withChangedChildren(map[string][]string{
				"root": {"table", "row1-col2"},
				"row1": {"row1-col1"},
			})),
		},
		{
			name: "columns contain invalid children -> move under the table",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"},
			}, withChangedChildren(map[string][]string{
				"columns": {"col1", "col2", "row1-col2"},
				"row1":    {"row1-col1"},
			})),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"},
			}, withChangedChildren(map[string][]string{
				"root": {"table", "row1-col2"},
				"row1": {"row1-col1"},
			})),
		},
		{
			name: "table block contains invalid children -> table is dropped",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}, withChangedChildren(map[string][]string{
				"table": {"columns"},
			})),
			want: state.NewDoc("root", map[string]simple.Block{"root": simple.New(&model.Block{Id: "root"})}).NewState(),
		},
		{
			name: "missed column is recreated",
			source: mkTestTable([]string{"col1"}, []string{"row1", "row2"}, [][]string{}, withChangedChildren(map[string][]string{
				"columns": {"col1", "col2"},
			})),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
		},
		{
			name: "missed row is recreated",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1"}, [][]string{}, withChangedChildren(map[string][]string{
				"rows": {"row1", "row2"},
			})),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			st := tc.source.Copy()
			tb := st.Pick("table")
			require.NotNil(t, tb)

			// when
			err := tb.(Block).Normalize(st)

			// then
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

	newId, visitedId, blocks, err := b.Duplicate(s)
	require.NoError(t, err)
	for _, b := range blocks {
		s.Add(b)
	}
	assert.ElementsMatch(t, []string{"table", "columns", "rows", "col1", "col2", "col3", "row1", "row2", "row1-col1", "row1-col3", "row2-col1", "row2-col2"}, visitedId)

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
	for i, oldID := range old.RowIDs() {
		newID := got.RowIDs()[i]

		oldRow := s.Pick(oldID)
		newRow := s.Pick(newID)

		assertNotEqual(oldRow.Model(), newRow.Model())
	}
}

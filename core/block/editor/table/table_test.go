package table

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestPickTableRootBlock(t *testing.T) {
	t.Run("block is not in state", func(t *testing.T) {
		// given
		s := state.NewDoc("root", nil).NewState()

		// when
		root := PickTableRootBlock(s, "id")

		// then
		assert.Nil(t, root)
	})
}

func TestDestructureDivs(t *testing.T) {
	t.Run("remove divs", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col1, col2"}, []string{"row1", "row2"}, nil)
		s = modifyState(s, func(s *state.State) *state.State {
			rows := s.Pick("rows")
			rows.Model().ChildrenIds = []string{"div1", "div2"}
			s.Set(rows)
			s.Set(simple.New(&model.Block{
				Id:          "div1",
				ChildrenIds: []string{"row1"},
				Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div}},
			}))
			s.Set(simple.New(&model.Block{
				Id:          "div2",
				ChildrenIds: []string{"row2"},
				Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div}},
			}))
			return s
		})

		// when
		destructureDivs(s, "rows")

		// then
		assert.Equal(t, []string{"row1", "row2"}, s.Pick("rows").Model().ChildrenIds)
	})
}

func TestTable_Iterate(t *testing.T) {
	t.Run("paint it black", func(t *testing.T) {
		// given
		colIDs := []string{"col1", "col2"}
		rowIDs := []string{"row1", "row2"}
		s := mkTestTable(colIDs, rowIDs, [][]string{{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"}})
		tb, err := NewTable(s, "rows")
		require.NoError(t, err)

		// when
		err = tb.Iterate(func(b simple.Block, _ CellPosition) bool {
			b.Model().BackgroundColor = "black"
			return true
		})

		// then
		require.NoError(t, err)
		for _, rowId := range rowIDs {
			for _, colId := range colIDs {
				assert.Equal(t, "black", s.Pick(MakeCellID(rowId, colId)).Model().BackgroundColor)
			}
		}
	})

	t.Run("failed to get a row", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col1", "row1-col2"}, {"row2-col1", "row2-col2"}})
		s = modifyState(s, func(s *state.State) *state.State {
			rows := s.Pick("rows")
			rows.Model().ChildrenIds = []string{"row0"}
			s.Set(rows)
			return s
		})
		tb, err := NewTable(s, "rows")
		require.NoError(t, err)

		// when
		err = tb.Iterate(func(b simple.Block, pos CellPosition) bool {
			return true
		})

		// then
		assert.Error(t, err)
	})

	t.Run("invalid cell id", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil)
		s = modifyState(s, func(s *state.State) *state.State {
			row := s.Pick("row1")
			row.Model().ChildrenIds = []string{"cell"}
			s.Set(row)
			return s
		})
		tb, err := NewTable(s, "rows")
		require.NoError(t, err)

		// when
		err = tb.Iterate(func(b simple.Block, pos CellPosition) bool {
			return true
		})

		// then
		assert.Error(t, err)
	})

	t.Run("no iteration", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col1"}})
		tb, err := NewTable(s, "rows")
		require.NoError(t, err)

		// when
		err = tb.Iterate(func(b simple.Block, pos CellPosition) bool {
			return false
		})

		// then
		assert.NoError(t, err)
	})
}

func TestCheckTableBlocksMove(t *testing.T) {
	for _, tc := range []struct {
		name      string
		source    *state.State
		target    string
		pos       model.BlockPosition
		blockIds  []string
		resPos    model.BlockPosition
		resTarget string
		shouldErr bool
	}{
		{
			name:      "no table - no error",
			source:    state.NewDoc("root", nil).NewState(),
			shouldErr: false,
		},
		{
			name:      "moving rows between each other",
			source:    mkTestTable([]string{"col"}, []string{"row1", "row2", "row3"}, nil),
			target:    "row2",
			pos:       model.Block_Bottom,
			blockIds:  []string{"row3", "row1"},
			resTarget: "row2",
			resPos:    model.Block_Bottom,
			shouldErr: false,
		},
		{
			name:      "moving rows between each other with invalid position",
			source:    mkTestTable([]string{"col"}, []string{"row1", "row2", "row3"}, nil),
			target:    "row2",
			pos:       model.Block_Replace,
			blockIds:  []string{"row3", "row1"},
			shouldErr: true,
		},
		{
			name:      "moving inner table blocks is prohibited",
			source:    mkTestTable([]string{"col"}, []string{"row1", "row2", "row3"}, nil),
			target:    "root",
			pos:       model.Block_Bottom,
			blockIds:  []string{"row3", "cols"},
			shouldErr: true,
		},
		{
			name: "place blocks under the table",
			source: modifyState(mkTestTable([]string{"col"}, []string{"row1", "row2", "row3"}, nil),
				func(s *state.State) *state.State {
					root := s.Pick("root")
					root.Model().ChildrenIds = []string{"table", "text"}
					s.Set(root)
					s.Add(simple.New(&model.Block{Id: "text"}))
					return s
				}),
			target:    "col",
			pos:       model.Block_Inner,
			blockIds:  []string{"text"},
			resTarget: "table",
			resPos:    model.Block_Bottom,
			shouldErr: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resTarget, resPos, err := CheckTableBlocksMove(tc.source, tc.target, tc.pos, tc.blockIds)
			if tc.shouldErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.resTarget, resTarget)
			assert.Equal(t, tc.resPos, resPos)
		})
	}
}

type testTableOptions struct {
	blocks    map[string]*model.Block
	rowBlocks map[string]*model.BlockContentTableRow
	children  map[string][]string
}

type testTableOption func(o *testTableOptions)

func withBlockContents(blocks map[string]*model.Block) testTableOption {
	return func(o *testTableOptions) {
		o.blocks = blocks
	}
}

func withRowBlockContents(blocks map[string]*model.BlockContentTableRow) testTableOption {
	return func(o *testTableOptions) {
		o.rowBlocks = blocks
	}
}

func withChangedChildren(children map[string][]string) testTableOption {
	return func(o *testTableOptions) {
		o.children = children
	}
}

func mkTestTable(columns []string, rows []string, cells [][]string, opts ...testTableOption) *state.State {
	blocks := mkTestTableBlocks(columns, rows, cells, opts...)
	o := testTableOptions{}
	for _, apply := range opts {
		apply(&o)
	}
	s := state.NewDoc("root", nil).NewState()
	for _, b := range blocks {
		if children, found := o.children[b.Id]; found {
			b.ChildrenIds = children
		}
		s.Add(simple.New(b))
	}
	return s
}

func mkTestTableSb(columns []string, rows []string, cells [][]string, opts ...testTableOption) *smarttest.SmartTest {
	blocks := mkTestTableBlocks(columns, rows, cells, opts...)
	o := testTableOptions{}
	for _, apply := range opts {
		apply(&o)
	}
	sb := smarttest.New("root")
	for _, b := range blocks {
		if children, found := o.children[b.Id]; found {
			b.ChildrenIds = children
		}
		sb.AddBlock(simple.New(b))
	}
	return sb
}

func mkTestTableBlocks(columns []string, rows []string, cells [][]string, opts ...testTableOption) []*model.Block {
	o := testTableOptions{}
	for _, apply := range opts {
		apply(&o)
	}

	blocks := []*model.Block{
		{
			Id:          "root",
			ChildrenIds: []string{"table"},
		},
		{
			Id:          "table",
			ChildrenIds: []string{"columns", "rows"},
			Content:     &model.BlockContentOfTable{Table: &model.BlockContentTable{}},
		},
		{
			Id:          "columns",
			ChildrenIds: columns,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableColumns,
				},
			},
		},
		{
			Id:          "rows",
			ChildrenIds: rows,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableRows,
				},
			},
		},
	}

	for _, c := range columns {
		blocks = append(blocks, &model.Block{
			Id:      c,
			Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}},
		})
	}

	cellsByRow := map[string][]string{}
	for _, cc := range cells {
		if len(cc) == 0 {
			continue
		}
		rowID, _, err := ParseCellID(cc[0])
		if err != nil {
			panic(err)
		}
		cellsByRow[rowID] = cc

		for _, c := range cc {
			proto, ok := o.blocks[c]
			if !ok {
				proto = &model.Block{
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
				}
			}
			proto.Id = c
			blocks = append(blocks, proto)
		}
	}

	for _, r := range rows {
		content := &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}
		if c, ok := o.rowBlocks[r]; ok {
			content.TableRow = c
		}
		blocks = append(blocks, &model.Block{
			Id:          r,
			ChildrenIds: cellsByRow[r],
			Content:     content,
		})
	}

	return blocks
}

func mkTextBlock(txt string) *model.Block {
	return &model.Block{
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: txt,
		}},
	}
}

func idFromSlice(ids []string) func() string {
	var i int
	return func() string {
		id := ids[i]
		i++
		return id
	}
}

func modifyState(s *state.State, modifier func(s *state.State) *state.State) *state.State {
	return modifier(s)
}

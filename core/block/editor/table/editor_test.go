package table

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestEditor_TableCreate(t *testing.T) {
	t.Run("table create - no error", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root"}))
		editor := NewEditor(sb)

		s := sb.NewState()

		// when
		id, err := editor.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId: "root",
			Position: model.Block_Inner,
			Columns:  3,
			Rows:     4,
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		tb, err := NewTable(s, id)

		require.NoError(t, err)

		assert.Len(t, tb.ColumnIDs(), 3)
		assert.Len(t, tb.RowIDs(), 4)

		for _, rowID := range tb.RowIDs() {
			row, err := pickRow(s, rowID)

			require.NoError(t, err)
			assert.Empty(t, row.Model().ChildrenIds)
		}
	})

	t.Run("table create - in object with Blocks restriction", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root"}))
		sb.TestRestrictions = restriction.Restrictions{Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}}}
		e := NewEditor(sb)

		s := sb.NewState()

		// when
		_, err := e.TableCreate(s, pb.RpcBlockTableCreateRequest{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, restriction.ErrRestricted))
	})

	t.Run("table create - error on insertion", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root"}))
		e := NewEditor(sb)

		s := sb.NewState()

		// when
		_, err := e.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId: "no_such_block",
			Position: model.Block_Inner,
			Columns:  3,
			Rows:     4,
		})

		// then
		assert.Error(t, err)
	})

	t.Run("table create - error on column creation", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"random_column_id"}})).AddBlock(simple.New(&model.Block{Id: "random_column_id"}))
		e := editor{sb: sb, generateColID: func() string {
			return "random_column_id"
		}}

		s := sb.NewState()

		// when
		_, err := e.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId: "root",
			Position: model.Block_Inner,
			Columns:  3,
			Rows:     2,
		})

		// then
		assert.Error(t, err)
	})

	t.Run("table create - error on row creation", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"random_row_id"}})).AddBlock(simple.New(&model.Block{Id: "random_row_id"}))
		e := editor{
			sb:            sb,
			generateColID: func() string { return "random_col_id" },
			generateRowID: func() string { return "random_row_id" },
		}

		s := sb.NewState()

		// when
		_, err := e.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId: "root",
			Position: model.Block_Inner,
			Columns:  1,
			Rows:     3,
		})

		// then
		assert.Error(t, err)
	})

	t.Run("table create - with header row", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root"}))
		editor := NewEditor(sb)

		s := sb.NewState()

		// when
		id, err := editor.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId:      "root",
			Position:      model.Block_Inner,
			Columns:       3,
			Rows:          4,
			WithHeaderRow: true,
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		tb, err := NewTable(s, id)
		require.NoError(t, err)

		assert.Len(t, tb.ColumnIDs(), 3)
		assert.Len(t, tb.RowIDs(), 4)

		row, err := tb.PickRow(tb.RowIDs()[0])
		require.NoError(t, err)
		headerRowId := row.Model().Id

		headerRow := row.Model().GetTableRow()
		require.NotNil(t, headerRow)
		assert.True(t, headerRow.IsHeader)

		cells := row.Model().ChildrenIds
		assert.Len(t, cells, 3)

		for _, cellID := range cells {
			rowID, _, err := ParseCellID(cellID)
			require.NoError(t, err)
			require.Equal(t, headerRowId, rowID)

			cell := s.Get(cellID)
			require.NotNil(t, cell)

			assert.Equal(t, "grey", cell.Model().BackgroundColor)
		}
	})

	t.Run("table create - with 0 rows and header row", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root"}))
		editor := NewEditor(sb)

		s := sb.NewState()

		// when
		_, err := editor.TableCreate(s, pb.RpcBlockTableCreateRequest{
			TargetId:      "root",
			Position:      model.Block_Inner,
			Columns:       3,
			Rows:          0,
			WithHeaderRow: true,
		})

		// then
		assert.Error(t, err)
	})
}

func TestEditor_CellCreate(t *testing.T) {
	for _, tc := range []struct {
		name         string
		source       *state.State
		colID, rowID string
		block        *model.Block
	}{
		{
			name:   "no table in state",
			source: state.NewDoc("root", nil).NewState(),
		},
		{
			name:   "failed to find row",
			source: mkTestTable([]string{"col1"}, []string{"row1", "row2"}, nil),
			colID:  "col1",
			rowID:  "row3",
		},
		{
			name:   "failed to find column",
			source: mkTestTable([]string{"col1"}, []string{"row1", "row2"}, nil),
			colID:  "col2",
			rowID:  "row2",
		},
		{
			name:   "failed to add a cell",
			source: mkTestTable([]string{"col1"}, []string{"row1", "row2"}, [][]string{{"row1-col1"}}),
			colID:  "col1",
			rowID:  "row1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := editor{}
			_, err := e.CellCreate(tc.source, tc.rowID, tc.colID, tc.block)
			assert.Error(t, err)
		})
	}
}

func TestEditor_RowCreate(t *testing.T) {
	type testCase struct {
		name     string
		source   *state.State
		newRowId string
		req      pb.RpcBlockTableRowCreateRequest
		want     *state.State
	}

	for _, tc := range []testCase{
		{
			name:     "cells are not affected",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newRowId: "row3",
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2"}, [][]string{{"row1-col2"}}),
		},
		{
			name:     "between, bottom position",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row3",
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2"}, nil),
		},
		{
			name:     "between, top position",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row3",
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row2",
				Position: model.Block_Top,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2"}, nil),
		},
		{
			name:     "at the beginning",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row3",
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row1",
				Position: model.Block_Top,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2"}, nil),
		},
		{
			name:     "at the end",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row3",
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row2",
				Position: model.Block_Bottom,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateRowID: idFromSlice([]string{tc.newRowId}),
			}
			id, err := tb.RowCreate(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.newRowId, id)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "no table in state",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row",
				Position: model.Block_Bottom,
			},
		},
		{
			name:   "no table in state on inner creation",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row",
				Position: model.Block_Inner,
			},
		},
		{
			name:   "invalid position",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row",
				Position: model.Block_Replace,
			},
		},
		{
			name:   "failed to add row",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableRowCreateRequest{
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
			newRowId: "row2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateRowID: idFromSlice([]string{tc.newRowId}),
			}
			_, err := tb.RowCreate(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_RowDelete(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col"}, []string{"row1", "row2"}, [][]string{{"row1-col"}})
		e := editor{}

		// when
		err := e.RowDelete(s, pb.RpcBlockTableRowDeleteRequest{TargetId: "row1"})

		// then
		require.NoError(t, err)
		tb, err := NewTable(s, "col")
		require.NoError(t, err)
		assert.Len(t, tb.RowIDs(), 1)
		assert.Equal(t, "row2", tb.RowIDs()[0])
	})

	t.Run("no such row", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col"}, []string{"row1", "row2"}, [][]string{{"row1-col"}})
		e := editor{}

		// when
		err := e.RowDelete(s, pb.RpcBlockTableRowDeleteRequest{TargetId: "row4"})

		// then
		assert.Error(t, err)
	})

	t.Run("invalid table", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col"}, []string{"row1", "row2"}, [][]string{{"row1-col"}})
		s.Unlink("row1")
		e := editor{}

		// when
		err := e.RowDelete(s, pb.RpcBlockTableRowDeleteRequest{TargetId: "row1"})

		// then
		assert.Error(t, err)
	})
}

func TestEditor_RowDuplicate(t *testing.T) {
	type testCase struct {
		name     string
		source   *state.State
		newRowId string
		req      pb.RpcBlockTableRowDuplicateRequest
		want     *state.State
	}
	for _, tc := range []testCase{
		{
			name: "fully filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row1-col2": mkTextBlock("test12"),
					"row2-col1": mkTextBlock("test21"),
				})),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row1",
				TargetId: "row2",
				Position: model.Block_Bottom,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row1-col2": mkTextBlock("test12"),
					"row2-col1": mkTextBlock("test21"),
					"row3-col1": mkTextBlock("test11"),
					"row3-col2": mkTextBlock("test12"),
				})),
		},
		{
			name: "partially filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1"},
					{"row2-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row2-col2": mkTextBlock("test22"),
				})),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row2",
				TargetId: "row1",
				Position: model.Block_Top,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2"},
				[][]string{
					{"row3-col2"},
					{"row1-col1"},
					{"row2-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row2-col2": mkTextBlock("test22"),
					"row3-col2": mkTextBlock("test22"),
				})),
		},
		{
			name: "empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{},
					{},
				}),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row2",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2"},
				[][]string{
					{},
					{},
					{},
				}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateRowID: idFromSlice([]string{tc.newRowId}),
			}
			id, err := tb.RowDuplicate(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.newRowId, id)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "invalid position",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row1",
				TargetId: "row1",
				Position: model.Block_Inner,
			},
		},
		{
			name:     "no former row",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row4",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row3",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
		},
		{
			name:     "target block is not a row",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row4",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row1",
				TargetId: "rows",
				Position: model.Block_Bottom,
			},
		},
		{
			name:     "failed to add new row",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			newRowId: "row2",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row2",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
		},
		{
			name: "cell is not found",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row2")
					row.Model().ChildrenIds = []string{"row2-col2"}
					s.Set(row)
					return s
				}),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row2",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
		},
		{
			name: "cell has invalid name",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"cell"}
					s.Set(row)
					s.Add(simple.New(&model.Block{Id: "cell"}))
					return s
				}),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row1",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
		},
		{
			name: "failed to add new cell",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col1"}}),
				func(s *state.State) *state.State {
					s.Add(simple.New(&model.Block{Id: "row3-col1"}))
					return s
				}),
			newRowId: "row3",
			req: pb.RpcBlockTableRowDuplicateRequest{
				BlockId:  "row1",
				TargetId: "row1",
				Position: model.Block_Bottom,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateRowID: idFromSlice([]string{tc.newRowId}),
			}
			_, err := tb.RowDuplicate(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_RowListFill(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableRowListFillRequest
		want   *state.State
	}
	for _, tc := range []testCase{
		{
			name:   "empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"row1", "row2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
		},
		{
			name: "fully filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"row1", "row2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
		},
		{
			name: "partially filled",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2", "row3", "row4", "row5"},
				[][]string{
					{"row1-col1"},
					{"row2-col2"},
					{"row3-col3"},
					{"row4-col1", "row4-col3"},
					{"row5-col2", "row4-col3"},
				}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"row1", "row2", "row3", "row4", "row5"},
			},
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2", "row3", "row4", "row5"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col2", "row2-col3"},
					{"row3-col1", "row3-col2", "row3-col3"},
					{"row4-col1", "row4-col2", "row4-col3"},
					{"row5-col1", "row5-col2", "row5-col3"},
				}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.RowListFill(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "bo block ids",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: nil,
			},
		},
		{
			name:   "no such row",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"row3"},
			},
		},
		{
			name:   "ids do not belong to rows",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"col1", "row1", "root"},
			},
		},
		{
			name:   "no table in state",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableRowListFillRequest{
				BlockIds: []string{"row1"},
			},
		},
	} {
		tb := editor{}
		err := tb.RowListFill(tc.source, tc.req)
		assert.Error(t, err)
	}
}

func TestEditor_RowListClean(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableRowListCleanRequest
		want   *state.State
	}

	for _, tc := range []testCase{
		{
			name: "empty rows",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{},
				{},
			}),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: []string{"row1", "row2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{},
				{},
			}),
		},
		{
			name: "rows with empty blocks",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"},
				{"row2-col2"},
			}),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: []string{"row1", "row2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{},
				{},
			}),
		},
		{
			name: "rows with not empty text block",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2"},
				{"row2-col2"},
			}, withBlockContents(map[string]*model.Block{
				"row1-col1": mkTextBlock("test11"),
				"row2-col1": mkTextBlock(""),
			})),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: []string{"row1", "row2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1"},
				{},
			}, withBlockContents(map[string]*model.Block{
				"row1-col1": mkTextBlock("test11"),
			})),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.RowListClean(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "block ids list is empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: nil,
			},
		},
		{
			name:   "no such row",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: []string{"row3"},
			},
		},
		{
			name:   "ids in list do not belong to rows",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableRowListCleanRequest{
				BlockIds: []string{"table", "col1", "row2"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.RowListClean(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_RowSetHeader(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableRowSetHeaderRequest
		want   *state.State
	}

	for _, tc := range []testCase{
		{
			name:   "header row moves up",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4"}, nil),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "row3",
				IsHeader: true,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2", "row4"}, nil,
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row3": {IsHeader: true},
				})),
		},
		{
			name: "non-header row moves down",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row2", "row3", "row1", "row4"}, nil,
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row2": {IsHeader: true},
					"row3": {IsHeader: true},
				})),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "row2",
				IsHeader: false,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row2", "row1", "row4"}, nil,
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row3": {IsHeader: true},
				})),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.RowSetHeader(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "no table in state",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "row2",
			},
		},
		{
			name:   "no such row",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row2", "row3", "row1", "row4"}, nil),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "row0",
				IsHeader: true,
			},
		},
		{
			name:   "target block is not a row",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row2", "row3", "row1", "row4"}, nil),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "col2",
				IsHeader: false,
			},
		},
		{
			name: "rows normalization error",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2", "row3"}
					s.Set(rows)
					return s
				}),
			req: pb.RpcBlockTableRowSetHeaderRequest{
				TargetId: "row2",
				IsHeader: true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.RowSetHeader(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_ColumnCreate(t *testing.T) {
	type testCase struct {
		name     string
		source   *state.State
		newColId string
		req      pb.RpcBlockTableColumnCreateRequest
		want     *state.State
	}

	for _, tc := range []struct {
		name     string
		source   *state.State
		newColId string
		req      pb.RpcBlockTableColumnCreateRequest
		want     *state.State
	}{
		{
			name:     "between, to the right",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col1",
				Position: model.Block_Right,
			},
			want: mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
		},
		{
			name:     "between, to the left",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col2",
				Position: model.Block_Left,
			},
			want: mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
		},
		{
			name:     "at the beginning",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col1",
				Position: model.Block_Left,
			},
			want: mkTestTable([]string{"col3", "col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
		},
		{
			name:     "at the end",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col2",
				Position: model.Block_Right,
			},
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice([]string{tc.newColId}),
			}
			id, err := tb.ColumnCreate(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.newColId, id)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "invalid position",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col2",
				Position: model.Block_Top,
			},
		},
		{
			name:   "failed to find target column - left",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col0",
				Position: model.Block_Left,
			},
		},
		{
			name: "failed to find target column - right",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
				func(s *state.State) *state.State {
					col := s.Pick("col1")
					col.Model().Content = nil
					s.Set(col)
					return s
				}),
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col1",
				Position: model.Block_Right,
			},
		},
		{
			name:   "no table in state - inner",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col1",
				Position: model.Block_Inner,
			},
		},
		{
			name:     "failed to add column",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col2"}}),
			newColId: "col2",
			req: pb.RpcBlockTableColumnCreateRequest{
				TargetId: "col1",
				Position: model.Block_Left,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice([]string{tc.newColId}),
			}
			_, err := tb.ColumnCreate(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_ColumnDelete(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableColumnDeleteRequest
		want   *state.State
	}
	for _, tc := range []testCase{
		{
			name: "partial table",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col3"},
				}),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col2",
			},
			want: mkTestTable([]string{"col1", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col3"},
					{"row2-col1", "row2-col3"},
				}),
		},
		{
			name: "filled table",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col2", "row2-col3"},
				}),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col3",
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnDelete(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "no such column",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col4",
			},
		},
		{
			name: "no table in state",
			source: state.NewDoc("root", map[string]simple.Block{
				"col1": simple.New(&model.Block{Id: "col", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}),
			}).NewState(),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col1",
			},
		},
		{
			name: "invalid cell id",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"cell"}
					s.Set(row)
					s.Set(simple.New(&model.Block{Id: "cell"}))
					return s
				}),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col1",
			},
		},
		{
			name: "cannot find row",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2"}
					s.Set(rows)
					return s
				}),
			req: pb.RpcBlockTableColumnDeleteRequest{
				TargetId: "col1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnDelete(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestColumnDuplicate(t *testing.T) {
	type testCase struct {
		name     string
		source   *state.State
		newColId string
		req      pb.RpcBlockTableColumnDuplicateRequest
		want     *state.State
	}
	for _, tc := range []testCase{
		{
			name: "fully filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row2-col1": mkTextBlock("test21"),
				})),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col1",
				TargetId: "col2",
				Position: model.Block_Right,
			},
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col2", "row2-col3"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col1": mkTextBlock("test11"),
					"row2-col1": mkTextBlock("test21"),
					"row1-col3": mkTextBlock("test11"),
					"row2-col3": mkTextBlock("test21"),
				})),
		},
		{
			name: "partially filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1"},
					{"row2-col2"},
					{},
				}, withBlockContents(map[string]*model.Block{
					"row2-col2": mkTextBlock("test22"),
				})),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col1",
				Position: model.Block_Left,
			},
			want: mkTestTable([]string{"col3", "col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1"},
					{"row2-col3", "row2-col2"},
					{},
				}, withBlockContents(map[string]*model.Block{
					"row2-col2": mkTextBlock("test22"),
					"row2-col3": mkTextBlock("test22"),
				})),
		},
		{
			name: "empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1"},
					{},
					{},
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col1",
				Position: model.Block_Left,
			},
			want: mkTestTable([]string{"col3", "col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1"},
					{},
					{},
				}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice([]string{tc.newColId}),
			}
			id, err := tb.ColumnDuplicate(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
			assert.Equal(t, tc.newColId, id)
		})
	}

	for _, tc := range []testCase{
		{
			name:     "invalid position",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col1",
				Position: model.Block_Top,
			},
		},
		{
			name:     "failed to find source column",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col4",
				TargetId: "col1",
				Position: model.Block_Left,
			},
		},
		{
			name:     "failed to find target column",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col4",
				Position: model.Block_Left,
			},
		},
		{
			name: "table is broken",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
				func(s *state.State) *state.State {
					table := s.Pick("table")
					table.Model().ChildrenIds = []string{"rows", "columns", "other"}
					s.Set(table)
					return s
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col2",
				Position: model.Block_Left,
			},
		},
		{
			name:     "failed to add new column",
			source:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
			newColId: "col1",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col2",
				Position: model.Block_Right,
			},
		},
		{
			name: "failed to find a row",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2"}
					s.Set(rows)
					return s
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col2",
				Position: model.Block_Right,
			},
		},
		{
			name: "cell with invalid id",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"cell"}
					s.Set(row)
					return s
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col2",
				Position: model.Block_Right,
			},
		},
		{
			name: "failed to find a cell",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"row1-col2"}
					s.Set(row)
					return s
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col2",
				TargetId: "col2",
				Position: model.Block_Right,
			},
		},
		{
			name: "failed to add new cell",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1"}, [][]string{{"row1-col1"}}),
				func(s *state.State) *state.State {
					s.Set(simple.New(&model.Block{Id: "row1-col3"}))
					return s
				}),
			newColId: "col3",
			req: pb.RpcBlockTableColumnDuplicateRequest{
				BlockId:  "col1",
				TargetId: "col1",
				Position: model.Block_Left,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice([]string{tc.newColId}),
			}
			_, err := tb.ColumnDuplicate(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_ColumnMove(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableColumnMoveRequest
		want   *state.State
	}
	for _, tc := range []testCase{
		{
			name: "partial table",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col3"},
				}),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col3",
				Position:     model.Block_Left,
			},
			want: mkTestTable([]string{"col2", "col1", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col2", "row1-col1", "row1-col3"},
					{"row2-col1", "row2-col3"},
				}),
		},
		{
			name: "filled table",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2", "row1-col3"},
					{"row2-col1", "row2-col2", "row2-col3"},
				}),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col3",
				DropTargetId: "col1",
				Position:     model.Block_Right,
			},
			want: mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col3", "row1-col2"},
					{"row2-col1", "row2-col3", "row2-col2"},
				}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnMove(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "invalid position of move",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col3",
				Position:     model.Block_Inner,
			},
		},
		{
			name:   "no such column to move",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col4",
				Position:     model.Block_Right,
			},
		},
		{
			name:   "no target column",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col5",
				DropTargetId: "col3",
				Position:     model.Block_Left,
			},
		},
		{
			name: "table is broken",
			source: modifyState(mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					s.Unlink("rows")
					return s
				}),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col3",
				Position:     model.Block_Left,
			},
		},
		{
			name: "failed to insert column",
			source: modifyState(mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					s.Unlink("col3")
					return s
				}),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col3",
				Position:     model.Block_Left,
			},
		},
		{
			name: "failed to find a row",
			source: modifyState(mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2", "row3"}
					s.Set(rows)
					return s
				}),
			req: pb.RpcBlockTableColumnMoveRequest{
				TargetId:     "col1",
				DropTargetId: "col3",
				Position:     model.Block_Left,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnMove(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_ColumnListFill(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableColumnListFillRequest
		want   *state.State
	}
	for _, tc := range []testCase{
		{
			name:   "empty",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: []string{"col2", "col1"},
			},
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
		},
		{
			name: "fully filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: []string{"col2", "col1"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
				}),
		},
		{
			name: "partially filled",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, [][]string{
				{"row1-col1"},
				{"row2-col2"},
				{"row3-col1", "row3-col2"},
			}),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: []string{"col1", "col2"},
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, [][]string{
				{"row1-col1", "row1-col2"},
				{"row2-col1", "row2-col2"},
				{"row3-col1", "row3-col2"},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnListFill(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "empty ids list",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{}),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: nil,
			},
		},
		{
			name:   "no table in state",
			source: state.NewDoc("root", nil).NewState(),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: []string{"col1"},
			},
		},
		{
			name: "failed to get a row",
			source: modifyState(mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{}),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2", "row3"}
					s.Set(rows)
					return s
				}),
			req: pb.RpcBlockTableColumnListFillRequest{
				BlockIds: []string{"col1"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.ColumnListFill(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestExpand(t *testing.T) {
	type testCase struct {
		name      string
		source    *state.State
		newColIds []string
		newRowIds []string
		req       pb.RpcBlockTableExpandRequest
		want      *state.State
	}
	for _, tc := range []testCase{
		{
			name:      "only rows",
			source:    mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
			newRowIds: []string{"row3", "row4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Rows:     2,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4"}, [][]string{{"row2-col2"}}),
		},
		{
			name:      "only columns",
			source:    mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
			newColIds: []string{"col3", "col4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Columns:  2,
			},
			want: mkTestTable([]string{"col1", "col2", "col3", "col4"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
		},
		{
			name:      "both columns and rows",
			source:    mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
			newRowIds: []string{"row3", "row4"},
			newColIds: []string{"col3", "col4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Rows:     2,
				Columns:  2,
			},
			want: mkTestTable([]string{"col1", "col2", "col3", "col4"}, []string{"row1", "row2", "row3", "row4"}, [][]string{{"row2-col2"}}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice(tc.newColIds),
				generateRowID: idFromSlice(tc.newRowIds),
			}
			err := tb.Expand(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:      "no table in state",
			source:    state.NewDoc("root", nil).NewState(),
			newRowIds: []string{"row3", "row4"},
			newColIds: []string{"col3", "col4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Rows:     2,
				Columns:  2,
			},
		},
		{
			name: "failed to create column",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
				func(s *state.State) *state.State {
					s.Set(simple.New(&model.Block{Id: "col3", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}))
					return s
				}),
			newRowIds: []string{"row3", "row4"},
			newColIds: []string{"col3", "col4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Rows:     2,
				Columns:  2,
			},
		},
		{
			name: "failed to create row",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row2-col2"}}),
				func(s *state.State) *state.State {
					s.Set(simple.New(&model.Block{Id: "row4", Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}))
					return s
				}),
			newRowIds: []string{"row3", "row4"},
			newColIds: []string{"col3", "col4"},
			req: pb.RpcBlockTableExpandRequest{
				TargetId: "table",
				Rows:     2,
				Columns:  2,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{
				generateColID: idFromSlice(tc.newColIds),
				generateRowID: idFromSlice(tc.newRowIds),
			}
			err := tb.Expand(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestSort(t *testing.T) {
	type testCase struct {
		name   string
		source *state.State
		req    pb.RpcBlockTableSortRequest
		want   *state.State
	}
	for _, tc := range []testCase{
		{
			name: "asc order",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("Abd"),
					"row2-col2": mkTextBlock("bsd"),
					"row3-col2": mkTextBlock("abc"),
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row3-col2": mkTextBlock("abc"),
					"row1-col2": mkTextBlock("Abd"),
					"row2-col2": mkTextBlock("bsd"),
				})),
		},
		{
			name: "desc order",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("234"),
					"row2-col2": mkTextBlock("323"),
					"row3-col2": mkTextBlock("123"),
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Desc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row2", "row1", "row3"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("234"),
					"row2-col2": mkTextBlock("323"),
					"row3-col2": mkTextBlock("123"),
				})),
		},
		{
			name: "asc order with header rows",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4", "row5"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
					{"row4-col1", "row4-col2"},
					{"row5-col1", "row5-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("555"),
					"row2-col2": mkTextBlock("444"),
					"row3-col2": mkTextBlock("333"),
					"row4-col2": mkTextBlock("222"),
					"row5-col2": mkTextBlock("111"),
				}),
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row1": {IsHeader: true},
					"row3": {IsHeader: true},
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row5", "row4", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row3-col1", "row3-col2"},
					{"row5-col1", "row5-col2"},
					{"row4-col1", "row4-col2"},
					{"row2-col1", "row2-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("555"),
					"row2-col2": mkTextBlock("444"),
					"row3-col2": mkTextBlock("333"),
					"row4-col2": mkTextBlock("222"),
					"row5-col2": mkTextBlock("111"),
				}),
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row1": {IsHeader: true},
					"row3": {IsHeader: true},
				})),
		},
		{
			name: "desc order with header rows",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4", "row5"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
					{"row4-col1", "row4-col2"},
					{"row5-col1", "row5-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("555"),
					"row2-col2": mkTextBlock("444"),
					"row3-col2": mkTextBlock("333"),
					"row4-col2": mkTextBlock("222"),
					"row5-col2": mkTextBlock("111"),
				}),
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row1": {IsHeader: true},
					"row3": {IsHeader: true},
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Desc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2", "row4", "row5"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row3-col1", "row3-col2"},
					{"row2-col1", "row2-col2"},
					{"row4-col1", "row4-col2"},
					{"row5-col1", "row5-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("555"),
					"row2-col2": mkTextBlock("444"),
					"row3-col2": mkTextBlock("333"),
					"row4-col2": mkTextBlock("222"),
					"row5-col2": mkTextBlock("111"),
				}),
				withRowBlockContents(map[string]*model.BlockContentTableRow{
					"row1": {IsHeader: true},
					"row3": {IsHeader: true},
				})),
		},
		{
			name: "alphabetical order",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("João"),
					"row2-col2": mkTextBlock("joz"),
					"row3-col2": mkTextBlock("Joao"),
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row3-col2": mkTextBlock("Joao"),
					"row1-col2": mkTextBlock("João"),
					"row2-col2": mkTextBlock("joz"),
				})),
		},
		{
			name: "numeric order with decimals",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
					{"row4-col1", "row4-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row1-col2": mkTextBlock("40.32"),
					"row2-col2": mkTextBlock("4321.89"),
					"row3-col2": mkTextBlock("10.32"),
					"row4-col2": mkTextBlock("55.00"),
				})),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
			want: mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row4", "row2"},
				[][]string{
					{"row1-col1", "row1-col2"},
					{"row2-col1", "row2-col2"},
					{"row3-col1", "row3-col2"},
					{"row4-col1", "row4-col2"},
				}, withBlockContents(map[string]*model.Block{
					"row3-col2": mkTextBlock("10.32"),
					"row1-col2": mkTextBlock("40.32"),
					"row4-col2": mkTextBlock("55.00"),
					"row2-col2": mkTextBlock("4321.89"),
				})),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.Sort(tc.source, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.want.Blocks(), tc.source.Blocks())
		})
	}

	for _, tc := range []testCase{
		{
			name:   "failed to find column",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4", "row5"}, nil),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col3",
				Type:     model.BlockContentDataviewSort_Desc,
			},
		},
		{
			name: "table is invalid",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3", "row4", "row5"}, nil),
				func(s *state.State) *state.State {
					table := s.Pick("table")
					table.Model().ChildrenIds = []string{"rows", "columns", "other"}
					s.Set(table)
					return s
				}),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Desc,
			},
		},
		{
			name: "failed to find a row",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2", "row3"}
					s.Set(rows)
					return s
				}),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
		},
		{
			name: "invalid cell id",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"cell"}
					s.Set(row)
					return s
				}),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
		},
		{
			name: "failed to find cell",
			source: modifyState(mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"row1-col2"}
					s.Set(row)
					return s
				}),
			req: pb.RpcBlockTableSortRequest{
				ColumnId: "col2",
				Type:     model.BlockContentDataviewSort_Asc,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := editor{}
			err := tb.Sort(tc.source, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestEditor_cleanupTables(t *testing.T) {
	t.Run("cannot do hook with nil smartblock", func(t *testing.T) {
		// given
		e := editor{}

		// when
		err := e.cleanupTables(smartblock.ApplyInfo{})

		// then
		assert.Error(t, err)
	})

	t.Run("no error", func(t *testing.T) {
		// given
		sb := mkTestTableSb([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{{"row1-col1"}, {"row2-col2"}}, withBlockContents(map[string]*model.Block{
			"row1-col1": mkTextBlock("test11"),
		}))
		e := editor{sb: sb}

		// when
		err := e.cleanupTables(smartblock.ApplyInfo{})

		// then
		require.NoError(t, err)
		assert.Len(t, sb.Pick("row1").Model().ChildrenIds, 1)
		assert.Empty(t, sb.Pick("row2").Model().ChildrenIds)
	})

	t.Run("broken table", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"table"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "table", ChildrenIds: []string{"columns", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "columns", Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}}))
		e := editor{sb: sb}

		// when
		err := e.cleanupTables(smartblock.ApplyInfo{})

		// then
		assert.NoError(t, err)
	})

	t.Run("iterate error", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"root"}}))
		e := editor{sb: sb}

		// when
		err := e.cleanupTables(smartblock.ApplyInfo{})

		// then
		assert.NoError(t, err)
	})

	t.Run("raw list clean error", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"table"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "table", ChildrenIds: []string{"columns", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "rows", ChildrenIds: []string{"row"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "columns", Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}}))
		e := editor{sb: sb}

		// when
		err := e.cleanupTables(smartblock.ApplyInfo{})

		// then
		assert.NoError(t, err)
	})
}

func TestEditor_cloneColumnStyles(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		source                *state.State
		srcColId, targetColId string
	}{
		{
			name:     "no table in state",
			source:   state.NewDoc("root", nil).NewState(),
			srcColId: "col1",
		},
		{
			name: "failed to find a row",
			source: modifyState(mkTestTable([]string{"col1"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					rows := s.Pick("rows")
					rows.Model().ChildrenIds = []string{"row1", "row2"}
					s.Set(rows)
					return s
				}),
			srcColId: "col1",
		},
		{
			name: "invalid cell id",
			source: modifyState(mkTestTable([]string{"col1"}, []string{"row1"}, nil),
				func(s *state.State) *state.State {
					row := s.Pick("row1")
					row.Model().ChildrenIds = []string{"cell"}
					s.Set(row)
					return s
				}),
			srcColId: "col1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := editor{}
			err := e.cloneColumnStyles(tc.source, tc.srcColId, tc.targetColId)
			assert.Error(t, err)
		})
	}

	t.Run("no error", func(t *testing.T) {
		// given
		s := mkTestTable([]string{"col1", "col2"}, []string{"row1"}, [][]string{{"row1-col1", "row1-col2"}}, withBlockContents(map[string]*model.Block{
			"row1-col1": {Id: "row1-col1", BackgroundColor: "red"},
		}))
		e := editor{}

		// when
		err := e.cloneColumnStyles(s, "col1", "col2")

		// then
		require.NoError(t, err)
		assert.Equal(t, "red", s.Pick("row1-col2").Model().BackgroundColor)
	})
}

func TestEditorAPI(t *testing.T) {
	rawTable := [][]string{
		{"c11", "c12", "c13"},
		{"c21", "c22", "c23"},
	}

	s := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		}),
	}).(*state.State)

	ed := editor{
		generateColID: idFromSlice([]string{"col1", "col2", "col3"}),
		generateRowID: idFromSlice([]string{"row1", "row2"}),
	}

	tableID, err := ed.TableCreate(s, pb.RpcBlockTableCreateRequest{
		TargetId: "root",
		Position: model.Block_Inner,
	})
	require.NoError(t, err)

	err = ed.Expand(s, pb.RpcBlockTableExpandRequest{
		TargetId: tableID,
		Columns:  3,
	})
	require.NoError(t, err)

	tb, err := NewTable(s, tableID)
	require.NoError(t, err)
	assert.Equal(t, tableID, tb.Block().Model().Id)

	columnIDs := tb.ColumnIDs()
	for _, row := range rawTable {
		rowID, err := ed.RowCreate(s, pb.RpcBlockTableRowCreateRequest{
			TargetId: tableID,
			Position: model.Block_Inner,
		})
		require.NoError(t, err)

		for colIdx, cellTxt := range row {
			colID := columnIDs[colIdx]

			_, err := ed.CellCreate(s, rowID, colID, &model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: cellTxt,
					},
				},
			})
			require.NoError(t, err)
		}
	}

	want := mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"},
		[][]string{
			{"row1-col1", "row1-col2", "row1-col3"},
			{"row2-col1", "row2-col2", "row2-col3"},
		}, withBlockContents(map[string]*model.Block{
			"row1-col1": mkTextBlock("c11"),
			"row1-col2": mkTextBlock("c12"),
			"row1-col3": mkTextBlock("c13"),
			"row2-col1": mkTextBlock("c21"),
			"row2-col2": mkTextBlock("c22"),
			"row2-col3": mkTextBlock("c23"),
		}))

	filter := func(bs []*model.Block) []*model.Block {
		var res []*model.Block
		for _, b := range bs {
			if b.GetTableRow() != nil || b.GetTableColumn() != nil || b.GetText() != nil {
				res = append(res, b)
			}
		}
		return res
	}
	assert.Equal(t, filter(want.Blocks()), filter(s.Blocks()))
}

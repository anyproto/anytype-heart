package table

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)

func TestTable_TableCreate(t *testing.T) {
	sb := smarttest.New("root")
	sb.AddBlock(simple.New(&model.Block{
		Id: "root",
	}))

	tb := New(sb)

	id, err := tb.TableCreate(nil, pb.RpcBlockTableCreateRequest{
		ContextId: "",
		TargetId:  "root",
		Position:  model.Block_Inner,
		Columns:   3,
		Rows:      2,
	})

	s := sb.NewState()

	assert.NoError(t, err)
	assert.True(t, s.Exists(id))

	want := mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, []string{"c11", "c12", "c13", "c21", "c22", "c23"})

	assertIsomorphic(t, want, s, map[string]string{}, map[string]string{})
}

func TestTable_TableRowCreate(t *testing.T) {
	ctx := newTableTestContext(t, 2, 2,
		mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"}))

	t.Run("to the top of the target", func(t *testing.T) {
		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		err = ctx.editor.RowCreate(nil, pb.RpcBlockTableRowCreateRequest{
			TargetId: tb.rows.Model().ChildrenIds[0],
			Position: model.Block_Top,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col2"}, []string{"row3", "row1", "row2"}, []string{"c31", "c32", "c11", "c12", "c21", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})

	t.Run("to the bottom of the target", func(t *testing.T) {
		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		err = ctx.editor.RowCreate(nil, pb.RpcBlockTableRowCreateRequest{
			TargetId: tb.rows.Model().ChildrenIds[0],
			Position: model.Block_Bottom,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col2"}, []string{"row3", "row4", "row1", "row2"}, []string{"c31", "c32", "c41", "c42", "c11", "c12", "c21", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})
}

func TestTable_TableRowDelete(t *testing.T) {
	ctx := newTableTestContext(t, 2, 2,
		mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"}))

	tb, err := newTableBlockFromState(ctx.s, ctx.id)
	require.NoError(t, err)

	err = ctx.editor.RowDelete(nil, pb.RpcBlockTableRowDeleteRequest{
		TargetId: tb.rows.Model().ChildrenIds[1],
	})

	require.NoError(t, err)

	want := mkTestTable([]string{"col1", "col2"}, []string{"row1"}, []string{"c11", "c12"})

	assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
}

func TestTable_TableRowMove(t *testing.T) {
	t.Run("to the top of the target", func(t *testing.T) {
		ctx := newTableTestContext(t, 2, 3,
			mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, []string{"c11", "c12", "c21", "c22", "c31", "c32"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		err = ctx.editor.RowMove(nil, pb.RpcBlockTableRowMoveRequest{
			TargetId:     tb.rows.Model().ChildrenIds[0],
			DropTargetId: tb.rows.Model().ChildrenIds[2],
			Position:     model.Block_Top,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col2"}, []string{"row2", "row1", "row3"}, []string{"c21", "c22", "c11", "c12", "c31", "c32"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})

	t.Run("to the bottom of the target", func(t *testing.T) {
		ctx := newTableTestContext(t, 2, 3,
			mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2", "row3"}, []string{"c11", "c12", "c21", "c22", "c31", "c32"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		err = ctx.editor.RowMove(nil, pb.RpcBlockTableRowMoveRequest{
			TargetId:     tb.rows.Model().ChildrenIds[2],
			DropTargetId: tb.rows.Model().ChildrenIds[0],
			Position:     model.Block_Bottom,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col2"}, []string{"row1", "row3", "row2"}, []string{"c11", "c12", "c31", "c32", "c21", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})
}

func TestTable_TableColumnCreate(t *testing.T) {
	ctx := newTableTestContext(t, 2, 2,
		mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"}))

	t.Run("to the right of target", func(t *testing.T) {
		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		target := tb.columns.Model().ChildrenIds[0]
		err = ctx.editor.ColumnCreate(nil, pb.RpcBlockTableColumnCreateRequest{
			TargetId: target,
			Position: model.Block_Right,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c11", "c13", "c12", "c21", "c23", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})

	t.Run("to the left of target", func(t *testing.T) {
		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		target := tb.columns.Model().ChildrenIds[0]
		err = ctx.editor.ColumnCreate(nil, pb.RpcBlockTableColumnCreateRequest{
			TargetId: target,
			Position: model.Block_Left,
		})

		require.NoError(t, err)

		// Remember that we operate under the same table, so previous modifications preserved
		want := mkTestTable([]string{"col4", "col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c14", "c11", "c13", "c12", "c24", "c21", "c23", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})
}

func TestTable_TableColumnDuplicate(t *testing.T) {
	t.Run("to the right of the target", func(t *testing.T) {
		ctx := newTableTestContext(t, 2, 2,
			mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		id, err := ctx.editor.ColumnDuplicate(nil, pb.RpcBlockTableColumnDuplicateRequest{
			BlockId:  tb.columns.Model().ChildrenIds[0],
			TargetId: tb.columns.Model().ChildrenIds[0],
			Position: model.Block_Right,
		})

		require.NoError(t, err)
		require.True(t, ctx.s.Exists(id))

		want := mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c11", "c13", "c12", "c21", "c23", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})

	t.Run("to the left of the target", func(t *testing.T) {
		ctx := newTableTestContext(t, 2, 2,
			mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		id, err := ctx.editor.ColumnDuplicate(nil, pb.RpcBlockTableColumnDuplicateRequest{
			BlockId:  tb.columns.Model().ChildrenIds[1],
			TargetId: tb.columns.Model().ChildrenIds[0],
			Position: model.Block_Left,
		})

		require.NoError(t, err)
		require.True(t, ctx.s.Exists(id))

		want := mkTestTable([]string{"col3", "col1", "col2"}, []string{"row1", "row2"}, []string{"c13", "c11", "c12", "c23", "c21", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})
}

func TestTable_TableColumnMove(t *testing.T) {
	t.Run("to the right of the drop target", func(t *testing.T) {
		ctx := newTableTestContext(t, 3, 2,
			mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, []string{"c11", "c12", "c13", "c21", "c22", "c23"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		target := tb.columns.Model().ChildrenIds[2]
		err = ctx.editor.ColumnMove(nil, pb.RpcBlockTableColumnMoveRequest{
			TargetId:     target,
			DropTargetId: tb.columns.Model().ChildrenIds[0],
			Position:     model.Block_Right,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c11", "c13", "c12", "c21", "c23", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})

	t.Run("to the left of the drop target", func(t *testing.T) {
		ctx := newTableTestContext(t, 3, 2,
			mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, []string{"c11", "c12", "c13", "c21", "c22", "c23"}))

		tb, err := newTableBlockFromState(ctx.s, ctx.id)
		require.NoError(t, err)

		err = ctx.editor.ColumnMove(nil, pb.RpcBlockTableColumnMoveRequest{
			TargetId:     tb.columns.Model().ChildrenIds[2],
			DropTargetId: tb.columns.Model().ChildrenIds[0],
			Position:     model.Block_Left,
		})

		require.NoError(t, err)

		want := mkTestTable([]string{"col3", "col1", "col2"}, []string{"row1", "row2"}, []string{"c13", "c11", "c12", "c23", "c21", "c22"})

		assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
	})
}

func TestTable_TableColumnDelete(t *testing.T) {
	ctx := newTableTestContext(t, 3, 2,
		mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, []string{"c11", "c12", "c13", "c21", "c22", "c23"}))

	tb, err := newTableBlockFromState(ctx.s, ctx.id)
	require.NoError(t, err)

	err = ctx.editor.ColumnDelete(nil, pb.RpcBlockTableColumnDeleteRequest{
		TargetId: tb.columns.Model().ChildrenIds[0],
	})
	require.NoError(t, err)

	want := mkTestTable([]string{"col2", "col3"}, []string{"row1", "row2"}, []string{"c12", "c13", "c22", "c23"})

	assertIsomorphic(t, want, ctx.s, ctx.wantMapping, ctx.gotMapping)
}

type tableTestContext struct {
	id          string
	editor      Table
	s           *state.State
	wantMapping map[string]string
	gotMapping  map[string]string
}

func newTableTestContext(t *testing.T, columnsCount, rowsCount uint32, wantTable *state.State) tableTestContext {
	sb := smarttest.New("root")
	sb.AddBlock(simple.New(&model.Block{
		Id: "root",
	}))

	ctx := tableTestContext{}

	ctx.editor = New(sb)

	id, err := ctx.editor.TableCreate(nil, pb.RpcBlockTableCreateRequest{
		ContextId: "",
		TargetId:  "root",
		Position:  model.Block_Inner,
		Columns:   columnsCount,
		Rows:      rowsCount,
	})
	ctx.id = id
	ctx.s = sb.NewState()

	assert.NoError(t, err)
	assert.True(t, ctx.s.Exists(id))

	ctx.wantMapping = map[string]string{}
	ctx.gotMapping = map[string]string{}
	assertIsomorphic(t, wantTable, ctx.s, ctx.wantMapping, ctx.gotMapping)

	return ctx
}

func idGenerator() func() string {
	var id int

	return func() string {
		id++
		return strconv.Itoa(id)
	}
}

func reassignIds(s *state.State, mapping map[string]string) *state.State {
	genId := idGenerator()

	var iter func(b simple.Block)
	iter = func(b simple.Block) {
		if b == nil {
			return
		}
		if _, ok := mapping[b.Model().Id]; !ok {
			id := genId()
			mapping[b.Model().Id] = id
		}

		for _, id := range b.Model().ChildrenIds {
			iter(s.Pick(id))
		}
	}
	iter(s.Pick(s.RootId()))

	res := state.NewDoc("", nil).NewState()
	iter = func(b simple.Block) {
		if b == nil {
			return
		}
		b = b.Copy()

		b.Model().Id = mapping[b.Model().Id]
		// Don't care about restrictions here
		b.Model().Restrictions = nil
		for i, id := range b.Model().ChildrenIds {
			iter(s.Pick(id))
			b.Model().ChildrenIds[i] = mapping[id]
		}
		res.Add(b)
	}
	iter(s.Pick(s.RootId()))

	return res
}

// assertIsomorphic checks that two states have same structure
// Preserves mappings for tracking structure changes
func assertIsomorphic(t *testing.T, want, got *state.State, wantMapping, gotMapping map[string]string) {
	want = reassignIds(want, wantMapping)
	got = reassignIds(got, gotMapping)

	var gotBlocks []simple.Block
	got.Iterate(func(b simple.Block) bool {
		gotBlocks = append(gotBlocks, b)
		return true
	})

	var wantBlocks []simple.Block
	want.Iterate(func(b simple.Block) bool {
		wantBlocks = append(wantBlocks, b)
		return true
	})

	assert.Equal(t, wantBlocks, gotBlocks)
}

func mkTestTable(columns []string, rows []string, cells []string) *state.State {
	s := state.NewDoc("root", nil).NewState()
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

	for i, r := range rows {
		from := i * len(columns)
		to := from + len(columns)
		cc := cells[from:to]

		blocks = append(blocks, &model.Block{
			Id:          r,
			ChildrenIds: cc,
			Content:     &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}},
		})
	}

	for _, c := range cells {
		blocks = append(blocks, &model.Block{
			Id:      c,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
		})
	}

	for _, b := range blocks {
		s.Add(simple.New(b))
	}
	return s
}

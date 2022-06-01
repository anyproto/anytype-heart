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

func TestTable_TableColumnCreate(t *testing.T) {
	sb := smarttest.New("root")
	sb.AddBlock(simple.New(&model.Block{
		Id: "root",
	}))

	editor := New(sb)

	id, err := editor.TableCreate(nil, pb.RpcBlockTableCreateRequest{
		ContextId: "",
		TargetId:  "root",
		Position:  model.Block_Inner,
		Columns:   2,
		Rows:      2,
	})

	s := sb.NewState()

	assert.NoError(t, err)
	assert.True(t, s.Exists(id))

	want := mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, []string{"c11", "c12", "c21", "c22"})

	wantMapping := map[string]string{}
	gotMapping := map[string]string{}
	assertIsomorphic(t, want, s, wantMapping, gotMapping)

	t.Run("to the right of target", func(t *testing.T) {
		tb, err := newTableBlockFromState(s, id)
		require.NoError(t, err)

		target := tb.columns.Model().ChildrenIds[0]
		err = editor.ColumnCreate(nil, pb.RpcBlockTableColumnCreateRequest{
			TargetId: target,
			Position: model.Block_Right,
		})

		require.NoError(t, err)

		want = mkTestTable([]string{"col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c11", "c13", "c12", "c21", "c23", "c22"})

		assertIsomorphic(t, want, s, wantMapping, gotMapping)
	})

	t.Run("to the left of target", func(t *testing.T) {
		tb, err := newTableBlockFromState(s, id)
		require.NoError(t, err)

		target := tb.columns.Model().ChildrenIds[0]
		err = editor.ColumnCreate(nil, pb.RpcBlockTableColumnCreateRequest{
			TargetId: target,
			Position: model.Block_Left,
		})

		require.NoError(t, err)

		// Remember that we operate under the same table, so previous modifications preserved
		want = mkTestTable([]string{"col4", "col1", "col3", "col2"}, []string{"row1", "row2"}, []string{"c14", "c11", "c13", "c12", "c24", "c21", "c23", "c22"})

		assertIsomorphic(t, want, s, wantMapping, gotMapping)
	})
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

package table

import (
	"fmt"
	"sort"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
)

var log = logging.Logger("anytype-simple-tables")

func NewEditor(sb smartblock.SmartBlock) Editor {
	genId := func() string {
		return bson.NewObjectId().Hex()
	}

	t := editor{
		SmartBlock:    sb,
		generateRowId: genId,
		generateColId: genId,
	}
	sb.AddHook(t.cleanupTables, smartblock.HookOnBlockClose)
	return t
}

type Editor interface {
	TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (id string, err error)
	Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error
	RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) error
	RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error
	RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) error
	RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error
	RowListClean(s *state.State, req pb.RpcBlockTableRowListCleanRequest) error
	ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) error
	ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error
	ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error
	ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error)
	Sort(s *state.State, req pb.RpcBlockTableSortRequest) error
}

type editor struct {
	smartblock.SmartBlock

	generateRowId func() string
	generateColId func() string
}

func (t editor) TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (id string, err error) {
	if err = t.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if t.Type() == model.SmartBlockType_Set {
		return "", basic.ErrNotSupported
	}

	id, err = basic.CreateBlock(s, "", pb.RpcBlockCreateRequest{
		ContextId: req.ContextId,
		TargetId:  req.TargetId,
		Position:  req.Position,
		Block: &model.Block{
			Content: &model.BlockContentOfTable{
				Table: &model.BlockContentTable{},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create block: %w", err)
	}

	columnIds := make([]string, 0, req.Columns)
	for i := uint32(0); i < req.Columns; i++ {
		id, err := t.addColumnHeader(s)
		if err != nil {
			return "", err
		}
		columnIds = append(columnIds, id)
	}
	columnsLayout := simple.New(&model.Block{
		ChildrenIds: columnIds,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns,
			},
		},
	})
	if !s.Add(columnsLayout) {
		return "", fmt.Errorf("add columns block")
	}

	rowIds := make([]string, 0, req.Rows)
	for i := uint32(0); i < req.Rows; i++ {
		id, err := t.addRow(s)
		if err != nil {
			return "", err
		}
		rowIds = append(rowIds, id)
	}
	rowsLayout := simple.New(&model.Block{
		ChildrenIds: rowIds,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows,
			},
		},
	})
	if !s.Add(rowsLayout) {
		return "", fmt.Errorf("add rows block")
	}

	table := s.Get(id)
	table.Model().ChildrenIds = []string{columnsLayout.Model().Id, rowsLayout.Model().Id}

	return id, nil
}

func (t editor) RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	rowId, err := t.addRow(s)
	if err != nil {
		return err
	}
	if err = s.InsertTo(req.TargetId, req.Position, rowId); err != nil {
		return fmt.Errorf("insert row: %w", err)
	}
	return nil
}

func (t editor) RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error {
	_, err := pickRow(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("pick target row: %w", err)
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("unlink row block")
	}
	return nil
}

func (t editor) ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error {
	_, err := pickColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("pick target column: %w", err)
	}

	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	for _, rowId := range tb.Rows().ChildrenIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}

		for _, cellId := range row.Model().ChildrenIds {
			_, colId, err := ParseCellId(cellId)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellId, err)
			}

			if colId == req.TargetId {
				if !s.Unlink(cellId) {
					return fmt.Errorf("unlink cell %s", cellId)
				}
				break
			}
		}
	}
	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("unlink column header")
	}

	return nil
}

func (t editor) ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error {
	switch req.Position {
	case model.Block_Left:
		req.Position = model.Block_Top
	case model.Block_Right:
		req.Position = model.Block_Bottom
	default:
		return fmt.Errorf("position is not supported")
	}
	_, err := pickColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get target column: %w", err)
	}
	_, err = pickColumn(s, req.DropTargetId)
	if err != nil {
		return fmt.Errorf("get drop target column: %w", err)
	}

	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("init table block: %w", err)
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("unlink target column")
	}
	if err = s.InsertTo(req.DropTargetId, req.Position, req.TargetId); err != nil {
		return fmt.Errorf("insert column: %w", err)
	}

	colIdx := map[string]int{}
	for i, c := range tb.Columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, id := range tb.Rows().ChildrenIds {
		row, err := getRow(s, id)
		if err != nil {
			return fmt.Errorf("get row %s: %w", id, err)
		}
		normalizeRow(colIdx, row)
	}

	return nil
}

func (t editor) RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) error {
	srcRow, err := pickRow(s, req.BlockId)
	if err != nil {
		return fmt.Errorf("pick source row: %w", err)
	}

	newRow := srcRow.Copy()
	newRow.Model().Id = t.generateRowId()
	if !s.Add(newRow) {
		return fmt.Errorf("add new row %s", newRow.Model().Id)
	}
	if err = s.InsertTo(req.TargetId, req.Position, newRow.Model().Id); err != nil {
		return fmt.Errorf("insert column: %w", err)
	}

	for i, srcId := range newRow.Model().ChildrenIds {
		cell := s.Pick(srcId)
		if cell == nil {
			return fmt.Errorf("cell %s is not found", srcId)
		}
		_, colId, err := ParseCellId(srcId)
		if err != nil {
			return fmt.Errorf("parse cell id %s: %w", srcId, err)
		}

		newCell := cell.Copy()
		newCell.Model().Id = makeCellId(newRow.Model().Id, colId)
		if !s.Add(newCell) {
			return fmt.Errorf("add new cell %s", newCell.Model().Id)
		}
		newRow.Model().ChildrenIds[i] = newCell.Model().Id
	}

	return nil
}

func (t editor) RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	tb, err := NewTable(s, req.BlockIds[0])
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	columns := tb.Columns().ChildrenIds

	for _, rowId := range req.BlockIds {
		row, err := getRow(s, rowId)
		if err != nil {
			return fmt.Errorf("get row %s: %w", rowId, err)
		}

		newIds := make([]string, 0, len(columns))
		for _, colId := range columns {
			id := makeCellId(rowId, colId)
			newIds = append(newIds, id)

			if !s.Exists(id) {
				_, err := addCell(s, rowId, colId)
				if err != nil {
					return fmt.Errorf("add cell %s: %w", id, err)
				}
			}
		}
		row.Model().ChildrenIds = newIds
	}
	return nil
}

func (t editor) RowListClean(s *state.State, req pb.RpcBlockTableRowListCleanRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	for _, rowId := range req.BlockIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row: %w", err)
		}

		for _, cellId := range row.Model().ChildrenIds {
			cell := s.Pick(cellId)
			if v, ok := cell.(text.Block); ok && v.IsEmpty() {
				s.Unlink(cellId)
			}
		}
	}
	return nil
}

func (t editor) cleanupTables() {
	s := t.NewState()

	err := s.Iterate(func(b simple.Block) bool {
		if b.Model().GetTable() == nil {
			return true
		}

		tb, err := NewTable(s, b.Model().Id)
		if err != nil {
			log.Errorf("cleanup init table %s: %s", b.Model().Id, err)
		}
		err = t.RowListClean(s, pb.RpcBlockTableRowListCleanRequest{
			BlockIds: tb.Rows().ChildrenIds,
		})
		if err != nil {
			log.Errorf("cleanup table %s: %s", b.Model().Id, err)
		}
		return true
	})
	if err != nil {
		log.Errorf("cleanup iterate: %s", err)
	}

	if err = t.Apply(s); err != nil {
		log.Errorf("cleanup apply: %s", err)
	}
}

func (t editor) ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) error {
	switch req.Position {
	case model.Block_Left:
		req.Position = model.Block_Top
	case model.Block_Right:
		req.Position = model.Block_Bottom
	default:
		return fmt.Errorf("position is not supported")
	}
	_, err := pickColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("pick column: %w", err)
	}

	colId, err := t.addColumnHeader(s)
	if err != nil {
		return err
	}
	if err = s.InsertTo(req.TargetId, req.Position, colId); err != nil {
		return fmt.Errorf("insert column header: %w", err)
	}
	return nil
}

func (t editor) ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error) {
	switch req.Position {
	case model.Block_Left:
		req.Position = model.Block_Top
	case model.Block_Right:
		req.Position = model.Block_Bottom
	default:
		return "", fmt.Errorf("position is not supported")
	}

	srcCol, err := pickColumn(s, req.BlockId)
	if err != nil {
		return "", fmt.Errorf("pick source column: %w", err)
	}

	_, err = pickColumn(s, req.TargetId)
	if err != nil {
		return "", fmt.Errorf("pick target column: %w", err)
	}

	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return "", fmt.Errorf("init table block: %w", err)
	}

	newCol := srcCol.Copy()
	newCol.Model().Id = t.generateColId()
	if !s.Add(newCol) {
		return "", fmt.Errorf("add column block")
	}
	if err = s.InsertTo(req.TargetId, req.Position, newCol.Model().Id); err != nil {
		return "", fmt.Errorf("insert column: %w", err)
	}

	colIdx := map[string]int{}
	for i, c := range tb.Columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, rowId := range tb.Rows().ChildrenIds {
		row, err := getRow(s, rowId)
		if err != nil {
			return "", fmt.Errorf("get row %s: %w", rowId, err)
		}

		var cellId string
		for _, id := range row.Model().ChildrenIds {
			_, colId, err := ParseCellId(id)
			if err != nil {
				return "", fmt.Errorf("parse cell %s in row %s: %w", cellId, rowId, err)
			}
			if colId == req.BlockId {
				cellId = id
				break
			}
		}
		if cellId == "" {
			continue
		}

		cell := s.Pick(cellId)
		if cell == nil {
			return "", fmt.Errorf("cell %s is not found", cellId)
		}
		cell = cell.Copy()
		cell.Model().Id = makeCellId(rowId, newCol.Model().Id)

		if !s.Add(cell) {
			return "", fmt.Errorf("add cell block")
		}

		row.Model().ChildrenIds = append(row.Model().ChildrenIds, cell.Model().Id)
		normalizeRow(colIdx, row)
	}

	return newCol.Model().Id, nil
}

func (t editor) Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error {
	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("init table block: %w", err)
	}

	for i := uint32(0); i < req.Columns; i++ {
		cols := tb.Columns()
		err := t.ColumnCreate(s, pb.RpcBlockTableColumnCreateRequest{
			TargetId: cols.ChildrenIds[len(cols.ChildrenIds)-1],
			Position: model.Block_Right,
		})
		if err != nil {
			return fmt.Errorf("create column: %w", err)
		}
	}

	for i := uint32(0); i < req.Rows; i++ {
		rows := tb.Rows()
		err := t.RowCreate(s, pb.RpcBlockTableRowCreateRequest{
			TargetId: rows.ChildrenIds[len(rows.ChildrenIds)-1],
			Position: model.Block_Bottom,
		})
		if err != nil {
			return fmt.Errorf("create row: %w", err)
		}
	}
	return nil
}

func (t editor) Sort(s *state.State, req pb.RpcBlockTableSortRequest) error {
	_, err := pickColumn(s, req.ColumnId)
	if err != nil {
		return fmt.Errorf("pick column: %w", err)
	}

	tb, err := NewTable(s, req.ColumnId)
	if err != nil {
		return fmt.Errorf("init table block: %w", err)
	}

	rows := s.Get(tb.Rows().Id)
	sorter := tableSorter{
		rowIds: rows.Model().ChildrenIds,
		values: make([]string, len(rows.Model().ChildrenIds)),
	}
	for i, rowId := range rows.Model().ChildrenIds {

		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}

		for _, cellId := range row.Model().ChildrenIds {
			_, colId, err := ParseCellId(cellId)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellId, err)
			}
			if colId == req.ColumnId {
				cell := s.Pick(cellId)
				if cell == nil {
					return fmt.Errorf("cell %s is not found", cellId)
				}
				sorter.values[i] = cell.Model().GetText().GetText()
			}
		}
	}

	if req.Type == model.BlockContentDataviewSort_Asc {
		sort.Stable(sorter)
	} else {
		sort.Stable(sort.Reverse(sorter))
	}

	return nil
}

type tableSorter struct {
	rowIds []string
	values []string
}

func (t tableSorter) Len() int {
	return len(t.rowIds)
}

func (t tableSorter) Less(i, j int) bool {
	return t.values[i] < t.values[j]
}

func (t tableSorter) Swap(i, j int) {
	t.values[i], t.values[j] = t.values[j], t.values[i]
	t.rowIds[i], t.rowIds[j] = t.rowIds[j], t.rowIds[i]
}

func (t editor) addColumnHeader(s *state.State) (string, error) {
	b := simple.New(&model.Block{
		Id: t.generateColId(),
		Content: &model.BlockContentOfTableColumn{
			TableColumn: &model.BlockContentTableColumn{},
		},
	})
	if !s.Add(b) {
		return "", fmt.Errorf("add column block")
	}
	return b.Model().Id, nil
}

func (t editor) addRow(s *state.State) (string, error) {
	row := simple.New(&model.Block{
		Id: t.generateRowId(),
		Content: &model.BlockContentOfTableRow{
			TableRow: &model.BlockContentTableRow{},
		},
	})
	if !s.Add(row) {
		return "", fmt.Errorf("add row block")
	}
	return row.Model().Id, nil
}

func getRow(s *state.State, id string) (simple.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, fmt.Errorf("row is not found")
	}
	if b.Model().GetTableRow() == nil {
		return nil, fmt.Errorf("block is not a row")
	}
	return b, nil
}

func pickRow(s *state.State, id string) (simple.Block, error) {
	b := s.Pick(id)
	if b == nil {
		return nil, fmt.Errorf("row is not found")
	}
	if b.Model().GetTableRow() == nil {
		return nil, fmt.Errorf("block is not a row")
	}
	return b, nil
}

func pickColumn(s *state.State, id string) (simple.Block, error) {
	b := s.Pick(id)
	if b == nil {
		return nil, fmt.Errorf("block is not found")
	}
	if b.Model().GetTableColumn() == nil {
		return nil, fmt.Errorf("block is not a column")
	}
	return b, nil
}

func makeCellId(rowId, colId string) string {
	return fmt.Sprintf("%s-%s", rowId, colId)
}

func ParseCellId(id string) (rowId string, colId string, err error) {
	toks := strings.SplitN(id, "-", 2)
	if len(toks) != 2 {
		return "", "", fmt.Errorf("invalid id: must contains rowId and colId")
	}
	return toks[0], toks[1], nil
}

func addCell(s *state.State, rowId, colId string) (string, error) {
	tb := simple.New(&model.Block{
		Id: makeCellId(rowId, colId),
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		},
	})
	if !s.Add(tb) {
		return "", fmt.Errorf("add text block")
	}
	return tb.Model().Id, nil
}

// Table aggregates valid table structure in state
type Table struct {
	s     *state.State
	block simple.Block
}

// NewTable creates helper for easy access to various parts of the table.
// It receives any id that belongs to table structure and search for the root table block
func NewTable(s *state.State, id string) (*Table, error) {
	tb := Table{
		s: s,
	}

	next := s.Pick(id)
	for next != nil {
		if next.Model().GetTable() != nil {
			tb.block = next
			break
		}
		next = s.PickParentOf(next.Model().Id)
	}
	if tb.block == nil {
		return nil, fmt.Errorf("root table block is not found")
	}

	if len(tb.block.Model().ChildrenIds) != 2 {
		return nil, fmt.Errorf("inconsistent state: table block")
	}

	if tb.Columns() == nil {
		return nil, fmt.Errorf("columns block is not found")
	}
	if tb.Rows() == nil {
		return nil, fmt.Errorf("rows block is not found")
	}

	return &tb, nil
}

func (tb Table) Block() simple.Block {
	return tb.block
}

func (tb Table) Columns() *model.Block {
	b := tb.s.Pick(tb.block.Model().ChildrenIds[0])
	if b == nil ||
		b.Model().GetLayout() == nil ||
		b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableColumns {
		return nil
	}
	return b.Model()
}

func (tb Table) Rows() *model.Block {
	b := tb.s.Pick(tb.block.Model().ChildrenIds[1])
	if b == nil ||
		b.Model().GetLayout() == nil ||
		b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableRows {
		return nil
	}
	return b.Model()
}

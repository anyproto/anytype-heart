package table

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
)

func New(sb smartblock.SmartBlock) Table {
	genId := func() string {
		return bson.NewObjectId().Hex()
	}
	return table{
		SmartBlock:    sb,
		generateRowId: genId,
		generateColId: genId,
	}
}

type Table interface {
	TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (id string, err error)
	Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error
	RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) error
	RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error
	RowMove(s *state.State, req pb.RpcBlockTableRowMoveRequest) error
	RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) error
	RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error
	ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) error
	ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error
	ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error
	ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error)
}

type table struct {
	smartblock.SmartBlock

	generateRowId func() string
	generateColId func() string
}

func (t table) TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (id string, err error) {
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
		return "", fmt.Errorf("can't add columns block")
	}

	rowIds := make([]string, 0, req.Rows)
	for i := uint32(0); i < req.Rows; i++ {
		id, err := t.addRow(s, columnIds)
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
		return "", fmt.Errorf("can't add rows block")
	}

	table := s.Pick(id)
	table.Model().ChildrenIds = []string{columnsLayout.Model().Id, rowsLayout.Model().Id}

	return id, nil
}

func (t table) RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) error {

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	if err := t.rowCreate(s, tb, req); err != nil {
		return err
	}

	return nil
}

func (t table) RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error {

	_, err := pickRow(s, req.TargetId)
	if err != nil {
		return err
	}
	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink row block")
	}

	return nil
}

func (t table) RowMove(s *state.State, req pb.RpcBlockTableRowMoveRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	_, err := pickRow(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get target row: %w", err)
	}
	_, err = pickRow(s, req.DropTargetId)
	if err != nil {
		return fmt.Errorf("get drop target row: %w", err)
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink target row")
	}

	if err = s.InsertTo(req.DropTargetId, req.Position, req.TargetId); err != nil {
		return fmt.Errorf("can't insert the row: %w", err)
	}

	return nil
}

func (t table) ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error {
	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	for _, rowId := range tb.rows().ChildrenIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}

		for _, cellId := range row.Model().ChildrenIds {
			_, colId, err := parseCellId(cellId)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellId, err)
			}

			if colId == req.TargetId {
				if !s.Unlink(cellId) {
					return fmt.Errorf("can't unlink cell %s", cellId)
				}
				break
			}
		}
	}
	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink column in header")
	}

	return nil
}

func (t table) ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error {
	switch req.Position {
	// TODO: crutch
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

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("can't init table block: %w", err)
	}

	targetPos := slice.FindPos(tb.columns().ChildrenIds, req.TargetId)
	if targetPos < 0 {
		return fmt.Errorf("can't find target column position")
	}
	dropPos := slice.FindPos(tb.columns().ChildrenIds, req.DropTargetId)
	if dropPos < 0 {
		return fmt.Errorf("can't find target column position")
	}
	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink target column")
	}
	if err = s.InsertTo(req.DropTargetId, req.Position, req.TargetId); err != nil {
		return fmt.Errorf("can't insert column: %w", err)
	}

	colIdx := map[string]int{}
	for i, c := range tb.columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, id := range tb.rows().ChildrenIds {
		row, err := pickRow(s, id)
		if err != nil {
			return fmt.Errorf("can't get row %s: %w", id, err)
		}

		if err = normalizeRow(s, colIdx, row); err != nil {
			return fmt.Errorf("normalize row %s: %w", id, err)
		}
	}

	return nil
}

func (t table) RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) error {
	return fmt.Errorf("not implemented")
}

func (t table) RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	tb, err := newTableBlockFromState(s, req.BlockIds[0])
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	columns := tb.columns().ChildrenIds

	for _, rowId := range req.BlockIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
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
		s.Set(row)
	}

	return nil
}

func (t table) ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) error {
	_, err := pickColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get column: %w", err)
	}
	switch req.Position {
	// TODO: crutch
	case model.Block_Left:
		req.Position = model.Block_Top
	case model.Block_Right:
		req.Position = model.Block_Bottom
	default:
		return fmt.Errorf("position is not supported")
	}

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	colPos := slice.FindPos(tb.columns().ChildrenIds, req.TargetId)
	if colPos < 0 {
		return fmt.Errorf("can't find target column")
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

func (t table) ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error) {
	switch req.Position {
	// TODO: crutch
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

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return "", fmt.Errorf("can't init table block: %w", err)
	}

	srcPos := slice.FindPos(tb.columns().ChildrenIds, req.BlockId)
	if srcPos < 0 {
		return "", fmt.Errorf("can't find source column position")
	}
	targetPos := slice.FindPos(tb.columns().ChildrenIds, req.TargetId)
	if targetPos < 0 {
		return "", fmt.Errorf("can't find target column position")
	}

	newCol := srcCol.Copy()
	newCol.Model().Id = t.generateColId()
	if !s.Add(newCol) {
		return "", fmt.Errorf("can't add column block")
	}

	for _, id := range tb.rows().ChildrenIds {
		row, err := pickRow(s, id)
		if err != nil {
			return "", fmt.Errorf("can't get row %s: %w", id, err)
		}

		if len(row.Model().ChildrenIds) != tb.columnsCount() {
			return "", fmt.Errorf("invalid number of columns in row %s", id)
		}

		srcId := row.Model().ChildrenIds[srcPos]
		targetId := row.Model().ChildrenIds[targetPos]

		cell := s.Pick(srcId)
		if cell == nil {
			return "", fmt.Errorf("cell %s is not found", srcId)
		}
		cell = cell.Copy()
		cell.Model().Id = makeCellId(id, newCol.Model().Id)

		if !s.Add(cell) {
			return "", fmt.Errorf("can't add cell block")
		}

		if err = s.InsertTo(targetId, req.Position, cell.Model().Id); err != nil {
			return "", fmt.Errorf("can't insert cell: %w", err)
		}
	}

	if err = s.InsertTo(req.TargetId, req.Position, newCol.Model().Id); err != nil {
		return "", fmt.Errorf("can't insert column: %w", err)
	}

	return newCol.Model().Id, t.Apply(s)
}

func (t table) Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error {

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("can't init table block: %w", err)
	}

	for i := uint32(0); i < req.Columns; i++ {
		err := t.ColumnCreate(s, pb.RpcBlockTableColumnCreateRequest{
			TargetId: tb.columns().ChildrenIds[tb.columnsCount()-1],
			Position: model.Block_Right,
		})
		if err != nil {
			return fmt.Errorf("create column: %w", err)
		}
	}

	for i := uint32(0); i < req.Rows; i++ {
		err := t.rowCreate(s, tb, pb.RpcBlockTableRowCreateRequest{
			TargetId: tb.rows().ChildrenIds[tb.rowsCount()-1],
			Position: model.Block_Bottom,
		})
		if err != nil {
			return fmt.Errorf("create row: %w", err)
		}
	}

	return nil
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

func parseCellId(id string) (rowId string, colId string, err error) {
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
		return "", fmt.Errorf("can't add text block")
	}
	return tb.Model().Id, nil
}

func (t table) rowCreate(s *state.State, tb *tableBlock, req pb.RpcBlockTableRowCreateRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	rowId, err := t.addRow(s, tb.columns().ChildrenIds)
	if err != nil {
		return err
	}

	if err = s.InsertTo(req.TargetId, req.Position, rowId); err != nil {
		return fmt.Errorf("insert row: %w", err)
	}

	return nil
}

func (t table) addColumnHeader(s *state.State) (string, error) {
	b := simple.New(&model.Block{
		Id: t.generateColId(),
		Content: &model.BlockContentOfTableColumn{
			TableColumn: &model.BlockContentTableColumn{},
		},
	})

	if !s.Add(b) {
		return "", fmt.Errorf("can't add column block")
	}
	return b.Model().Id, nil
}

func (t table) addRow(s *state.State, columns []string) (string, error) {
	row := simple.New(&model.Block{
		Id: t.generateRowId(),
		Content: &model.BlockContentOfTableRow{
			TableRow: &model.BlockContentTableRow{},
		},
	})

	if !s.Add(row) {
		return "", fmt.Errorf("can't add row block")
	}
	return row.Model().Id, nil
}

type tableBlock struct {
	s     *state.State
	block simple.Block
}

// newTableBlockFromState creates helper for easy access to various parts of the table.
// It receives any id that belongs to table structure and search for the root table block
func newTableBlockFromState(s *state.State, id string) (*tableBlock, error) {
	tb := tableBlock{
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

	if tb.columns() == nil {
		return nil, fmt.Errorf("columns block is not found")
	}
	if tb.rows() == nil {
		return nil, fmt.Errorf("rows block is not found")
	}

	return &tb, nil
}

func (tb tableBlock) columns() *model.Block {
	b := tb.s.Pick(tb.block.Model().ChildrenIds[0])
	if b == nil ||
		b.Model().GetLayout() == nil ||
		b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableColumns {
		return nil
	}
	return b.Model()
}

func (tb tableBlock) columnsCount() int {
	return len(tb.columns().ChildrenIds)
}

func (tb tableBlock) rows() *model.Block {
	b := tb.s.Pick(tb.block.Model().ChildrenIds[1])
	if b == nil ||
		b.Model().GetLayout() == nil ||
		b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableRows {
		return nil
	}
	return b.Model()
}

func (tb tableBlock) rowsCount() int {
	return len(tb.rows().ChildrenIds)
}

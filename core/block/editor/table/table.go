package table

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	blocktable "github.com/anytypeio/go-anytype-middleware/core/block/simple/table"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func New(sb smartblock.SmartBlock) Table {
	return table{
		SmartBlock: sb,
		basic:      basic.NewBasic(sb),
	}
}

type Table interface {
	TableCreate(ctx *state.Context, groupId string, req pb.RpcBlockTableCreateRequest) (id string, err error)
	RowCreate(ctx *state.Context, req pb.RpcBlockTableRowCreateRequest) error
	ColumnCreate(ctx *state.Context, req pb.RpcBlockTableColumnCreateRequest) error
	RowDelete(ctx *state.Context, req pb.RpcBlockTableRowDeleteRequest) error
	ColumnDelete(ctx *state.Context, req pb.RpcBlockTableColumnDeleteRequest) error
	RowMove(ctx *state.Context, req pb.RpcBlockTableRowMoveRequest) error
	ColumnMove(ctx *state.Context, req pb.RpcBlockTableColumnMoveRequest) error

	CellSetVerticalAlign(ctx *state.Context, req pb.RpcBlockTableCellSetVerticalAlignRequest) error
}

type table struct {
	smartblock.SmartBlock

	basic basic.Basic
}

func (t table) TableCreate(ctx *state.Context, groupId string, req pb.RpcBlockTableCreateRequest) (id string, err error) {
	if err = t.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if t.Type() == model.SmartBlockType_Set {
		return "", basic.ErrNotSupported
	}

	s := t.NewStateCtx(ctx).SetGroupId(groupId)

	id, err = basic.CreateBlock(s, groupId, pb.RpcBlockCreateRequest{
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
		id, err := addColumnHeader(s)
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
		id, err := addRow(s, req.Columns, nil)
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

	if err = t.Apply(s); err != nil {
		return
	}
	return id, nil
}

func (t table) RowCreate(ctx *state.Context, req pb.RpcBlockTableRowCreateRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	s := t.NewStateCtx(ctx)

	rowTarget, err := pickRow(s, req.TargetId)

	count := uint32(len(rowTarget.Model().ChildrenIds))
	rowId, err := addRow(s, count, nil)
	if err != nil {
		return err
	}

	if err = s.InsertTo(req.TargetId, req.Position, rowId); err != nil {
		return fmt.Errorf("insert row: %w", err)
	}

	return t.Apply(s)
}

func (t table) RowDelete(ctx *state.Context, req pb.RpcBlockTableRowDeleteRequest) error {
	s := t.NewStateCtx(ctx)

	_, err := pickRow(s, req.TargetId)
	if err != nil {
		return err
	}
	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink row block")
	}

	return t.Apply(s)
}

func (t table) RowMove(ctx *state.Context, req pb.RpcBlockTableRowMoveRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	s := t.NewStateCtx(ctx)

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

	return t.Apply(s)
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

func (t table) ColumnCreate(ctx *state.Context, req pb.RpcBlockTableColumnCreateRequest) error {
	s := t.NewStateCtx(ctx)

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

	colPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.TargetId)
	if colPos < 0 {
		return fmt.Errorf("can't find target column")
	}

	for _, rowId := range tb.rows.Model().ChildrenIds {
		cellId, err := addCell(s, nil)
		if err != nil {
			return fmt.Errorf("add cell: %w", err)
		}

		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}
		if len(row.Model().ChildrenIds) != tb.columnsCount() {
			return fmt.Errorf("inconsistent row state")
		}

		targetColumnId := row.Model().ChildrenIds[colPos]
		if err = s.InsertTo(targetColumnId, req.Position, cellId); err != nil {
			return fmt.Errorf("insert cell: %w", err)
		}
	}

	id, err := addColumnHeader(s)
	if err != nil {
		return err
	}
	if err = s.InsertTo(req.TargetId, req.Position, id); err != nil {
		return fmt.Errorf("insert column header: %w", err)
	}

	return t.Apply(s)
}

func (t table) ColumnDelete(ctx *state.Context, req pb.RpcBlockTableColumnDeleteRequest) error {
	s := t.NewStateCtx(ctx)

	tb, err := newTableBlockFromState(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	colPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.TargetId)
	if colPos < 0 {
		return fmt.Errorf("can't find target column")
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink column in header")
	}

	for _, rowId := range tb.rows.Model().ChildrenIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}
		if len(row.Model().ChildrenIds) != tb.columnsCount() {
			return fmt.Errorf("inconsistent row state")
		}

		cellId := row.Model().ChildrenIds[colPos]
		if !s.Unlink(cellId) {
			return fmt.Errorf("can't unlink cell %s", cellId)
		}
	}

	return t.Apply(s)
}

func (t table) ColumnMove(ctx *state.Context, req pb.RpcBlockTableColumnMoveRequest) error {
	switch req.Position {
	// TODO: crutch
	case model.Block_Left:
		req.Position = model.Block_Top
	case model.Block_Right:
		req.Position = model.Block_Bottom
	default:
		return fmt.Errorf("position is not supported")
	}

	s := t.NewStateCtx(ctx)

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

	targetPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.TargetId)
	if targetPos < 0 {
		return fmt.Errorf("can't find target column position")
	}
	dropPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.DropTargetId)
	if dropPos < 0 {
		return fmt.Errorf("can't find target column position")
	}

	for _, id := range tb.rows.Model().ChildrenIds {
		row, err := pickRow(s, id)
		if err != nil {
			return fmt.Errorf("can't get row %s: %w", id, err)
		}

		if len(row.Model().ChildrenIds) != tb.columnsCount() {
			return fmt.Errorf("invalid number of columns in row %s", id)
		}
		// TODO: write own implementation of inserting?

		targetId := row.Model().ChildrenIds[targetPos]
		dropId := row.Model().ChildrenIds[dropPos]

		if !s.Unlink(targetId) {
			return fmt.Errorf("can't unlink column in row %s", id)
		}
		if err = s.InsertTo(dropId, req.Position, targetId); err != nil {
			return fmt.Errorf("can't insert column: %w", err)
		}
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("can't unlink target column")
	}
	if err = s.InsertTo(req.DropTargetId, req.Position, req.TargetId); err != nil {
		return fmt.Errorf("can't insert column: %w", err)
	}

	return t.Apply(s)
}

func (t table) CellSetVerticalAlign(ctx *state.Context, req pb.RpcBlockTableCellSetVerticalAlignRequest) error {
	s := t.NewStateCtx(ctx)

	cell, err := getCell(s, req.BlockId)
	if err != nil {
		return fmt.Errorf("get cell: %w", err)
	}
	cell.SetVerticalAlign(req.VerticalAlign)

	return t.Apply(s)
}

func getCell(s *state.State, id string) (blocktable.Cell, error) {
	b := s.Get(id)
	if b == nil {
		return nil, fmt.Errorf("block is not found")
	}
	c, ok := b.(blocktable.Cell)
	if !ok {
		return nil, fmt.Errorf("block is not a cell: %T", b)
	}
	return c, nil
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

func addCell(s *state.State, cellBlockProto *model.Block) (string, error) {
	tb := simple.New(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		},
	})
	if !s.Add(tb) {
		return "", fmt.Errorf("can't add text block")
	}
	cell := simple.New(&model.Block{
		Align:           cellBlockProto.GetAlign(),
		BackgroundColor: cellBlockProto.GetBackgroundColor(),
		ChildrenIds:     []string{tb.Model().Id},
		Content: &model.BlockContentOfTableCell{
			TableCell: &model.BlockContentTableCell{
				VerticalAlign: cellBlockProto.GetTableCell().GetVerticalAlign(),
			},
		},
	})
	if !s.Add(cell) {
		return "", fmt.Errorf("can't add cell block")
	}

	return cell.Model().Id, nil
}

func addColumnHeader(s *state.State) (string, error) {
	b := simple.New(&model.Block{
		Content: &model.BlockContentOfTableColumn{
			TableColumn: &model.BlockContentTableColumn{},
		},
	})

	if !s.Add(b) {
		return "", fmt.Errorf("can't add column block")
	}
	return b.Model().Id, nil
}

func addRow(s *state.State, columns uint32, cellBlockProto *model.Block) (string, error) {
	cellIds := make([]string, 0, columns)
	for j := uint32(0); j < columns; j++ {
		id, err := addCell(s, cellBlockProto)
		if err != nil {
			return "", err
		}
		cellIds = append(cellIds, id)
	}

	row := simple.New(&model.Block{
		ChildrenIds: cellIds,
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
	block   simple.Block
	columns simple.Block
	rows    simple.Block
}

func (b tableBlock) columnsCount() int {
	return len(b.columns.Model().ChildrenIds)
}

func (b tableBlock) rowsCount() int {
	return len(b.rows.Model().ChildrenIds)
}

func newTableBlockFromState(s *state.State, id string) (*tableBlock, error) {
	tb := tableBlock{}

	next := s.Pick(id)
	for next != nil {
		if next.Model().GetTable() != nil {
			tb.block = next
			break
		}
		next = s.GetParentOf(next.Model().Id)
	}
	if tb.block == nil {
		return nil, fmt.Errorf("root table block is not found")
	}

	if len(tb.block.Model().ChildrenIds) != 2 {
		return nil, fmt.Errorf("inconsistent state: table block")
	}

	{
		b := s.Pick(tb.block.Model().ChildrenIds[0])
		if b == nil ||
			b.Model().GetLayout() == nil ||
			b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableColumns {
			return nil, fmt.Errorf("inconsistent state: columns block")
		}
		tb.columns = b
	}

	{
		b := s.Pick(tb.block.Model().ChildrenIds[1])
		if b == nil ||
			b.Model().GetLayout() == nil ||
			b.Model().GetLayout().GetStyle() != model.BlockContentLayout_TableRows {
			return nil, fmt.Errorf("inconsistent state: rows block")
		}
		tb.rows = b
	}

	return &tb, nil
}

package table

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
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
	CreateTable(ctx *state.Context, groupId string, req pb.RpcBlockTableCreateRequest) (id string, err error)
	CreateRow(ctx *state.Context, req pb.RpcBlockTableCreateRowRequest) error
	CreateColumn(ctx *state.Context, req pb.RpcBlockTableCreateColumnRequest) error
	DeleteRow(ctx *state.Context, req pb.RpcBlockTableDeleteRowRequest) error
	DeleteColumn(ctx *state.Context, req pb.RpcBlockTableDeleteColumnRequest) error
	MoveRow(ctx *state.Context, req pb.RpcBlockTableMoveRowRequest) error
	MoveColumn(ctx *state.Context, req pb.RpcBlockTableMoveColumnRequest) error
}

type table struct {
	smartblock.SmartBlock

	basic basic.Basic
}

func (t table) CreateTable(ctx *state.Context, groupId string, req pb.RpcBlockTableCreateRequest) (id string, err error) {
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

func (t table) CreateRow(ctx *state.Context, req pb.RpcBlockTableCreateRowRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	s := t.NewStateCtx(ctx)

	rowTarget, err := getRow(s, req.TargetRowId)
	// TODO: move cols/rows to table block ?
	columns := uint32(len(rowTarget.Model().ChildrenIds))
	rowId, err := addRow(s, columns, nil)
	if err != nil {
		return err
	}

	if err = s.InsertTo(req.TargetRowId, req.Position, rowId); err != nil {
		return fmt.Errorf("insert row: %w", err)
	}

	return t.Apply(s)
}

func (t table) DeleteRow(ctx *state.Context, req pb.RpcBlockTableDeleteRowRequest) error {
	s := t.NewStateCtx(ctx)

	_, err := getRow(s, req.TargetRowId)
	if err != nil {
		return err
	}
	if !s.Unlink(req.TargetRowId) {
		return fmt.Errorf("can't unlink row block")
	}

	return t.Apply(s)
}

func (t table) MoveRow(ctx *state.Context, req pb.RpcBlockTableMoveRowRequest) error {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

	s := t.NewStateCtx(ctx)

	_, err := getRow(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get target row: %w", err)
	}
	_, err = getRow(s, req.DropTargetId)
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

func getRow(s *state.State, id string) (simple.Block, error) {
	b := s.Pick(id)
	if b == nil {
		return nil, fmt.Errorf("row is not found")
	}
	if b.Model().GetTableRow() == nil {
		return nil, fmt.Errorf("block is not a row")
	}
	return b, nil
}

func (t table) CreateColumn(ctx *state.Context, req pb.RpcBlockTableCreateColumnRequest) error {
	s := t.NewStateCtx(ctx)

	_, err := getColumn(s, req.TargetColumnId)
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

	tb, err := newTableBlockFromState(s, req.TargetColumnId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	colPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.TargetColumnId)
	if colPos < 0 {
		return fmt.Errorf("can't find target column")
	}

	rowsCount := len(tb.rows.Model().ChildrenIds)

	for i := 0; i < rowsCount; i++ {
		cellId, err := addCell(s, nil)
		if err != nil {
			return fmt.Errorf("add cell: %w", err)
		}

		row := s.Pick(tb.rows.Model().ChildrenIds[i])
		if row == nil || row.Model().GetTableRow() == nil || len(row.Model().ChildrenIds) <= colPos {
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
	if err = s.InsertTo(req.TargetColumnId, req.Position, id); err != nil {
		return fmt.Errorf("insert column header: %w", err)
	}

	return t.Apply(s)
}

func (t table) DeleteColumn(ctx *state.Context, req pb.RpcBlockTableDeleteColumnRequest) error {
	s := t.NewStateCtx(ctx)

	tb, err := newTableBlockFromState(s, req.TargetColumnId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	colPos := slice.FindPos(tb.columns.Model().ChildrenIds, req.TargetColumnId)
	if colPos < 0 {
		return fmt.Errorf("can't find target column")
	}

	if !s.Unlink(req.TargetColumnId) {
		return fmt.Errorf("can't unlink column in header")
	}

	for _, id := range tb.rows.Model().ChildrenIds {
		// TODO: make tb.rows a slice: rows []simple.Block or make helper tb.getRow(id)
		row := s.Pick(id)
		if row == nil || row.Model().GetTableRow() == nil || len(row.Model().ChildrenIds) <= colPos {
			return fmt.Errorf("inconsistent row state %s", id)
		}

		cellId := row.Model().ChildrenIds[colPos]
		if !s.Unlink(cellId) {
			return fmt.Errorf("can't unlink cell %s", cellId)
		}
	}

	return t.Apply(s)
}

func (t table) MoveColumn(ctx *state.Context, req pb.RpcBlockTableMoveColumnRequest) error {
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

	_, err := getColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get target column: %w", err)
	}
	_, err = getColumn(s, req.DropTargetId)
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
		row, err := getRow(s, id)
		if err != nil {
			return fmt.Errorf("can't get row %s: %w", id, err)
		}

		// TODO: move getRow to tableBlock, make tableBlock lazy, assert columns count in row
		if len(row.Model().ChildrenIds) < len(tb.columns.Model().ChildrenIds) {
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

func getColumn(s *state.State, id string) (simple.Block, error) {
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
				Color:         cellBlockProto.GetTableCell().GetColor(),
				Style:         cellBlockProto.GetTableCell().GetStyle(),
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

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
	s := t.NewStateCtx(ctx)

	rowTarget := s.Pick(req.TargetRowId)
	if rowTarget == nil {
		return fmt.Errorf("target block not found")
	}
	if rowTarget.Model().GetTableRow() == nil {
		return fmt.Errorf("target block in not a row")
	}
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	default:
		return fmt.Errorf("position is not supported")
	}

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

	rowTarget := s.Pick(req.TargetRowId)
	if rowTarget == nil {
		return fmt.Errorf("target block not found")
	}
	if rowTarget.Model().GetTableRow() == nil {
		return fmt.Errorf("target block in not a row")
	}

	if !s.Unlink(req.TargetRowId) {
		return fmt.Errorf("can't unlink row block")
	}

	return t.Apply(s)
}

func (t table) CreateColumn(ctx *state.Context, req pb.RpcBlockTableCreateColumnRequest) error {
	s := t.NewStateCtx(ctx)

	colTarget := s.Pick(req.TargetColumnId)
	if colTarget == nil {
		return fmt.Errorf("target block not found")
	}
	if colTarget.Model().GetTableColumn() == nil {
		return fmt.Errorf("target block is not a column")
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

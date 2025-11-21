package table

import (
	"errors"
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// nolint:revive,interfacebloat
type TableEditor interface {
	TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (string, error)
	CellCreate(s *state.State, rowID string, colID string, b *model.Block) (string, error)

	RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) (string, error)
	RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error
	RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) (newRowID string, err error)
	// RowMove is done via BlockListMoveToExistingObject
	RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error
	RowListClean(s *state.State, req pb.RpcBlockTableRowListCleanRequest) error
	RowSetHeader(s *state.State, req pb.RpcBlockTableRowSetHeaderRequest) error

	ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) (string, error)
	ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error
	ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error)
	ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error
	ColumnListFill(s *state.State, req pb.RpcBlockTableColumnListFillRequest) error

	Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error
	Sort(s *state.State, req pb.RpcBlockTableSortRequest) error

	cleanupTables(_ smartblock.ApplyInfo) error
	cloneColumnStyles(s *state.State, srcColID string, targetColID string) error
}

type editor struct {
	sb smartblock.SmartBlock

	generateRowID func() string
	generateColID func() string
}

var _ TableEditor = &editor{}

func NewEditor(sb smartblock.SmartBlock) TableEditor {
	genID := func() string {
		return bson.NewObjectId().Hex()
	}

	t := editor{
		sb:            sb,
		generateRowID: genID,
		generateColID: genID,
	}
	if sb != nil {
		sb.AddHook(t.cleanupTables, smartblock.HookOnBlockClose)
	}
	return &t
}

func (t *editor) TableCreate(s *state.State, req pb.RpcBlockTableCreateRequest) (string, error) {
	if t.sb != nil {
		if err := t.sb.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
			return "", err
		}
	}

	tableBlock := simple.New(&model.Block{
		Content: &model.BlockContentOfTable{
			Table: &model.BlockContentTable{},
		},
	})
	if !s.Add(tableBlock) {
		return "", fmt.Errorf("add table block")
	}

	if err := s.InsertTo(req.TargetId, req.Position, tableBlock.Model().Id); err != nil {
		return "", fmt.Errorf("insert block: %w", err)
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

	rowIDs := make([]string, 0, req.Rows)
	for i := uint32(0); i < req.Rows; i++ {
		id, err := t.addRow(s)
		if err != nil {
			return "", err
		}
		rowIDs = append(rowIDs, id)
	}

	rowsLayout := simple.New(&model.Block{
		ChildrenIds: rowIDs,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows,
			},
		},
	})
	if !s.Add(rowsLayout) {
		return "", fmt.Errorf("add rows block")
	}

	tableBlock.Model().ChildrenIds = []string{columnsLayout.Model().Id, rowsLayout.Model().Id}

	if !req.WithHeaderRow {
		return tableBlock.Model().Id, nil
	}

	if len(rowIDs) == 0 {
		return "", fmt.Errorf("no rows to make header row")
	}
	headerID := rowIDs[0]

	if err := t.RowSetHeader(s, pb.RpcBlockTableRowSetHeaderRequest{
		TargetId: headerID,
		IsHeader: true,
	}); err != nil {
		return "", fmt.Errorf("row set header: %w", err)
	}

	if err := t.RowListFill(s, pb.RpcBlockTableRowListFillRequest{
		BlockIds: []string{headerID},
	}); err != nil {
		return "", fmt.Errorf("fill header row: %w", err)
	}

	row, err := getRow(s, headerID)
	if err != nil {
		return "", fmt.Errorf("get header row: %w", err)
	}

	for _, cellID := range row.Model().ChildrenIds {
		cell := s.Get(cellID)
		if cell == nil {
			return "", fmt.Errorf("get header cell id %s", cellID)
		}

		cell.Model().BackgroundColor = "grey"
	}

	return tableBlock.Model().Id, nil
}

func (t *editor) CellCreate(s *state.State, rowID string, colID string, b *model.Block) (string, error) {
	tb, err := NewTable(s, rowID)
	if err != nil {
		return "", fmt.Errorf("initialize table state: %w", err)
	}

	row, err := getRow(s, rowID)
	if err != nil {
		return "", fmt.Errorf("get row: %w", err)
	}
	if _, err = pickColumn(s, colID); err != nil {
		return "", fmt.Errorf("pick column: %w", err)
	}

	cellID, err := addCell(s, rowID, colID)
	if err != nil {
		return "", fmt.Errorf("add cell: %w", err)
	}
	cell := s.Get(cellID)
	cell.Model().Content = b.Content
	if err := s.InsertTo(rowID, model.Block_Inner, cellID); err != nil {
		return "", fmt.Errorf("insert to: %w", err)
	}

	tb.normalizeRow(nil, row)

	return cellID, nil
}

func (t *editor) RowCreate(s *state.State, req pb.RpcBlockTableRowCreateRequest) (string, error) {
	switch req.Position {
	case model.Block_Top, model.Block_Bottom:
	case model.Block_Inner:
		tb, err := NewTable(s, req.TargetId)
		if err != nil {
			return "", fmt.Errorf("initialize table state: %w", err)
		}
		req.TargetId = tb.Rows().Id
	default:
		return "", fmt.Errorf("position is not supported")
	}

	rowID, err := t.addRow(s)
	if err != nil {
		return "", err
	}
	if err := s.InsertTo(req.TargetId, req.Position, rowID); err != nil {
		return "", fmt.Errorf("insert row: %w", err)
	}
	return rowID, nil
}

func (t *editor) RowDelete(s *state.State, req pb.RpcBlockTableRowDeleteRequest) error {
	_, err := pickRow(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("pick target row: %w", err)
	}

	if !s.Unlink(req.TargetId) {
		return fmt.Errorf("unlink row block")
	}
	return nil
}

func (t *editor) RowDuplicate(s *state.State, req pb.RpcBlockTableRowDuplicateRequest) (newRowID string, err error) {
	if req.Position != model.Block_Top && req.Position != model.Block_Bottom {
		return "", fmt.Errorf("position %s is not supported", model.BlockPosition_name[int32(req.Position)])
	}
	srcRow, err := pickRow(s, req.BlockId)
	if err != nil {
		return "", fmt.Errorf("pick source row: %w", err)
	}

	if _, err = pickRow(s, req.TargetId); err != nil {
		return "", fmt.Errorf("pick target row: %w", err)
	}

	newRow := srcRow.Copy()
	newRow.Model().Id = t.generateRowID()
	if !s.Add(newRow) {
		return "", fmt.Errorf("add new row %s", newRow.Model().Id)
	}
	if err = s.InsertTo(req.TargetId, req.Position, newRow.Model().Id); err != nil {
		return "", fmt.Errorf("insert column: %w", err)
	}

	for i, srcID := range newRow.Model().ChildrenIds {
		cell := s.Pick(srcID)
		if cell == nil {
			return "", fmt.Errorf("cell %s is not found", srcID)
		}
		_, colID, err := ParseCellID(srcID)
		if err != nil {
			return "", fmt.Errorf("parse cell id %s: %w", srcID, err)
		}

		newCell := cell.Copy()
		newCell.Model().Id = MakeCellID(newRow.Model().Id, colID)
		if !s.Add(newCell) {
			return "", fmt.Errorf("add new cell %s", newCell.Model().Id)
		}
		newRow.Model().ChildrenIds[i] = newCell.Model().Id
	}

	return newRow.Model().Id, nil
}

func (t *editor) RowListFill(s *state.State, req pb.RpcBlockTableRowListFillRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	tb, err := NewTable(s, req.BlockIds[0])
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	columns := tb.ColumnIDs()

	for _, rowID := range req.BlockIds {
		row, err := getRow(s, rowID)
		if err != nil {
			return fmt.Errorf("get row %s: %w", rowID, err)
		}

		newIds := make([]string, 0, len(columns))
		for _, colID := range columns {
			id := MakeCellID(rowID, colID)
			newIds = append(newIds, id)

			if !s.Exists(id) {
				_, err := addCell(s, rowID, colID)
				if err != nil {
					return fmt.Errorf("add cell %s: %w", id, err)
				}
			}
		}
		row.Model().ChildrenIds = newIds
	}
	return nil
}

func (t *editor) RowListClean(s *state.State, req pb.RpcBlockTableRowListCleanRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	for _, rowID := range req.BlockIds {
		row, err := pickRow(s, rowID)
		if err != nil {
			return fmt.Errorf("pick row: %w", err)
		}

		for _, cellID := range row.Model().ChildrenIds {
			cell := s.Pick(cellID)
			if v, ok := cell.(text.Block); ok && v.IsEmpty() {
				s.Unlink(cellID)
			}
		}
	}
	return nil
}

func (t *editor) RowSetHeader(s *state.State, req pb.RpcBlockTableRowSetHeaderRequest) error {
	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	row, err := getRow(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("get target row: %w", err)
	}

	if row.Model().GetTableRow().IsHeader != req.IsHeader {
		row.Model().GetTableRow().IsHeader = req.IsHeader

		err = tb.normalizeHeaderRows()
		if err != nil {
			return fmt.Errorf("normalize rows: %w", err)
		}
	}

	return nil
}

func (t *editor) ColumnCreate(s *state.State, req pb.RpcBlockTableColumnCreateRequest) (string, error) {
	switch req.Position {
	case model.Block_Left:
		req.Position = model.Block_Top
		if _, err := pickColumn(s, req.TargetId); err != nil {
			return "", fmt.Errorf("pick column: %w", err)
		}
	case model.Block_Right:
		req.Position = model.Block_Bottom
		if _, err := pickColumn(s, req.TargetId); err != nil {
			return "", fmt.Errorf("pick column: %w", err)
		}
	case model.Block_Inner:
		tb, err := NewTable(s, req.TargetId)
		if err != nil {
			return "", fmt.Errorf("initialize table state: %w", err)
		}
		req.TargetId = tb.Columns().Id
	default:
		return "", fmt.Errorf("position is not supported")
	}

	colID, err := t.addColumnHeader(s)
	if err != nil {
		return "", err
	}
	if err = s.InsertTo(req.TargetId, req.Position, colID); err != nil {
		return "", fmt.Errorf("insert column header: %w", err)
	}

	return colID, t.cloneColumnStyles(s, req.TargetId, colID)
}

func (t *editor) ColumnDelete(s *state.State, req pb.RpcBlockTableColumnDeleteRequest) error {
	_, err := pickColumn(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("pick target column: %w", err)
	}

	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("initialize table state: %w", err)
	}

	for _, rowID := range tb.RowIDs() {
		row, err := pickRow(s, rowID)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowID, err)
		}

		for _, cellID := range row.Model().ChildrenIds {
			_, colID, err := ParseCellID(cellID)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellID, err)
			}

			if colID == req.TargetId {
				if !s.Unlink(cellID) {
					return fmt.Errorf("unlink cell %s", cellID)
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

func (t *editor) ColumnDuplicate(s *state.State, req pb.RpcBlockTableColumnDuplicateRequest) (id string, err error) {
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
	newCol.Model().Id = t.generateColID()
	if !s.Add(newCol) {
		return "", fmt.Errorf("add column block")
	}
	if err = s.InsertTo(req.TargetId, req.Position, newCol.Model().Id); err != nil {
		return "", fmt.Errorf("insert column: %w", err)
	}

	colIdx := tb.MakeColumnIndex()

	for _, rowID := range tb.RowIDs() {
		row, err := getRow(s, rowID)
		if err != nil {
			return "", fmt.Errorf("get row %s: %w", rowID, err)
		}

		var cellID string
		for _, id := range row.Model().ChildrenIds {
			_, colID, err := ParseCellID(id)
			if err != nil {
				return "", fmt.Errorf("parse cell %s in row %s: %w", cellID, rowID, err)
			}
			if colID == req.BlockId {
				cellID = id
				break
			}
		}
		if cellID == "" {
			continue
		}

		cell := s.Pick(cellID)
		if cell == nil {
			return "", fmt.Errorf("cell %s is not found", cellID)
		}
		cell = cell.Copy()
		cell.Model().Id = MakeCellID(rowID, newCol.Model().Id)

		if !s.Add(cell) {
			return "", fmt.Errorf("add cell block")
		}

		row.Model().ChildrenIds = append(row.Model().ChildrenIds, cell.Model().Id)
		tb.normalizeRow(colIdx, row)
	}

	return newCol.Model().Id, nil
}

func (t *editor) ColumnMove(s *state.State, req pb.RpcBlockTableColumnMoveRequest) error {
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

	colIdx := tb.MakeColumnIndex()

	for _, id := range tb.RowIDs() {
		row, err := getRow(s, id)
		if err != nil {
			return fmt.Errorf("get row %s: %w", id, err)
		}
		tb.normalizeRow(colIdx, row)
	}

	return nil
}

func (t *editor) ColumnListFill(s *state.State, req pb.RpcBlockTableColumnListFillRequest) error {
	if len(req.BlockIds) == 0 {
		return fmt.Errorf("empty row list")
	}

	tb, err := NewTable(s, req.BlockIds[0])
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}

	rows := tb.RowIDs()

	for _, colID := range req.BlockIds {
		for _, rowID := range rows {
			id := MakeCellID(rowID, colID)
			if s.Exists(id) {
				continue
			}
			_, err := addCell(s, rowID, colID)
			if err != nil {
				return fmt.Errorf("add cell %s: %w", id, err)
			}

			row, err := getRow(s, rowID)
			if err != nil {
				return fmt.Errorf("get row %s: %w", rowID, err)
			}

			row.Model().ChildrenIds = append(row.Model().ChildrenIds, id)
		}
	}

	colIdx := tb.MakeColumnIndex()
	for _, rowID := range rows {
		row, err := getRow(s, rowID)
		if err != nil {
			return fmt.Errorf("get row %s: %w", rowID, err)
		}
		tb.normalizeRow(colIdx, row)
	}

	return nil
}

func (t *editor) Expand(s *state.State, req pb.RpcBlockTableExpandRequest) error {
	tb, err := NewTable(s, req.TargetId)
	if err != nil {
		return fmt.Errorf("init table block: %w", err)
	}

	for i := uint32(0); i < req.Columns; i++ {
		_, err := t.ColumnCreate(s, pb.RpcBlockTableColumnCreateRequest{
			TargetId: req.TargetId,
			Position: model.Block_Inner,
		})
		if err != nil {
			return fmt.Errorf("create column: %w", err)
		}
	}

	for i := uint32(0); i < req.Rows; i++ {
		rows := tb.Rows()
		_, err := t.RowCreate(s, pb.RpcBlockTableRowCreateRequest{
			TargetId: rows.ChildrenIds[len(rows.ChildrenIds)-1],
			Position: model.Block_Bottom,
		})
		if err != nil {
			return fmt.Errorf("create row: %w", err)
		}
	}
	return nil
}

func (t *editor) Sort(s *state.State, req pb.RpcBlockTableSortRequest) error {
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
		rowIDs:   make([]string, 0, len(rows.Model().ChildrenIds)),
		values:   make([]string, len(rows.Model().ChildrenIds)),
		collator: collate.New(language.Und),
	}

	var headers []string

	var i int
	for _, rowID := range rows.Model().ChildrenIds {
		row, err := pickRow(s, rowID)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowID, err)
		}
		if row.Model().GetTableRow().GetIsHeader() {
			headers = append(headers, rowID)
			continue
		}

		sorter.rowIDs = append(sorter.rowIDs, rowID)
		for _, cellID := range row.Model().ChildrenIds {
			_, colID, err := ParseCellID(cellID)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellID, err)
			}
			if colID == req.ColumnId {
				cell := s.Pick(cellID)
				if cell == nil {
					return fmt.Errorf("cell %s is not found", cellID)
				}
				sorter.values[i] = cell.Model().GetText().GetText()
			}
		}
		i++
	}

	if req.Type == model.BlockContentDataviewSort_Asc {
		sort.Stable(sorter)
	} else {
		sort.Stable(sort.Reverse(sorter))
	}

	// nolint:gocritic
	rows.Model().ChildrenIds = append(headers, sorter.rowIDs...)

	return nil
}

func (t *editor) cleanupTables(_ smartblock.ApplyInfo) error {
	if t.sb == nil {
		return fmt.Errorf("nil smartblock")
	}
	s := t.sb.NewState()

	err := s.Iterate(func(b simple.Block) bool {
		if b.Model().GetTable() == nil {
			return true
		}

		tb, err := NewTable(s, b.Model().Id)
		if err != nil {
			log.Errorf("cleanup: init table %s: %s", b.Model().Id, err)
			return true
		}
		err = t.RowListClean(s, pb.RpcBlockTableRowListCleanRequest{
			BlockIds: tb.RowIDs(),
		})
		if err != nil {
			log.Errorf("cleanup table %s: %s", b.Model().Id, err)
			return true
		}
		return true
	})
	if err != nil {
		log.Errorf("cleanup iterate: %s", err)
	}

	s.SetChangeType(domain.ChangeTypeCleanupTables)

	if err = t.sb.Apply(s, smartblock.KeepInternalFlags); err != nil {
		if errors.Is(err, source.ErrReadOnly) {
			return nil
		}
		log.Errorf("cleanup apply: %s", err)
	}
	return nil
}

func (t *editor) cloneColumnStyles(s *state.State, srcColID, targetColID string) error {
	tb, err := NewTable(s, srcColID)
	if err != nil {
		return fmt.Errorf("init table block: %w", err)
	}
	colIdx := tb.MakeColumnIndex()

	for _, rowID := range tb.RowIDs() {
		row, err := pickRow(s, rowID)
		if err != nil {
			return fmt.Errorf("pick row: %w", err)
		}

		var protoBlock simple.Block
		for _, cellID := range row.Model().ChildrenIds {
			_, colID, err := ParseCellID(cellID)
			if err != nil {
				return fmt.Errorf("parse cell id: %w", err)
			}

			if colID == srcColID {
				protoBlock = s.Pick(cellID)
			}
		}

		if protoBlock != nil && protoBlock.Model().BackgroundColor != "" {
			targetCellID := MakeCellID(rowID, targetColID)

			if !s.Exists(targetCellID) {
				_, err := addCell(s, rowID, targetColID)
				if err != nil {
					return fmt.Errorf("add cell: %w", err)
				}
			}
			cell := s.Get(targetCellID)
			cell.Model().BackgroundColor = protoBlock.Model().BackgroundColor

			row = s.Get(row.Model().Id)
			row.Model().ChildrenIds = append(row.Model().ChildrenIds, targetCellID)
			tb.normalizeRow(colIdx, row)
		}
	}

	return nil
}

func (t *editor) addColumnHeader(s *state.State) (string, error) {
	b := simple.New(&model.Block{
		Id: t.generateColID(),
		Content: &model.BlockContentOfTableColumn{
			TableColumn: &model.BlockContentTableColumn{},
		},
	})
	if !s.Add(b) {
		return "", fmt.Errorf("add column block")
	}
	return b.Model().Id, nil
}

func (t *editor) addRow(s *state.State) (string, error) {
	row := makeRow(t.generateRowID())
	if !s.Add(row) {
		return "", fmt.Errorf("add row block")
	}
	return row.Model().Id, nil
}

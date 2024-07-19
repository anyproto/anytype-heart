package table

import (
	"fmt"
	"slices"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/table"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("anytype-simple-tables")

var ErrCannotMoveTableBlocks = fmt.Errorf("can not move table blocks")

type tableSorter struct {
	rowIDs []string
	values []string
}

func (t tableSorter) Len() int {
	return len(t.rowIDs)
}

func (t tableSorter) Less(i, j int) bool {
	return strings.ToLower(t.values[i]) < strings.ToLower(t.values[j])
}

func (t tableSorter) Swap(i, j int) {
	t.values[i], t.values[j] = t.values[j], t.values[i]
	t.rowIDs[i], t.rowIDs[j] = t.rowIDs[j], t.rowIDs[i]
}

func makeRow(id string) simple.Block {
	return simple.New(&model.Block{
		Id: id,
		Content: &model.BlockContentOfTableRow{
			TableRow: &model.BlockContentTableRow{},
		},
	})
}

func getRow(s *state.State, id string) (simple.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, fmt.Errorf("row is not found")
	}
	_, ok := b.(table.RowBlock)
	if !ok {
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

func MakeCellID(rowID, colID string) string {
	return fmt.Sprintf("%s%s%s", rowID, table.TableCellSeparator, colID)
}

func ParseCellID(id string) (rowID string, colID string, err error) {
	toks := strings.SplitN(id, table.TableCellSeparator, 2)
	if len(toks) != 2 {
		return "", "", fmt.Errorf("invalid id: must contains rowID and colID")
	}
	return toks[0], toks[1], nil
}

func isTableCell(id string) bool {
	_, _, err := ParseCellID(id)
	return err == nil
}

func addCell(s *state.State, rowID, colID string) (string, error) {
	c := simple.New(&model.Block{
		Id: MakeCellID(rowID, colID),
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		},
	})
	if !s.Add(c) {
		return "", fmt.Errorf("add text block")
	}
	return c.Model().Id, nil
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

	tb.block = GetTableRootBlock(s, id)
	if tb.block == nil {
		return nil, fmt.Errorf("root table block is not found")
	}

	if len(tb.block.Model().ChildrenIds) < 2 {
		return nil, fmt.Errorf("inconsistent state: table block")
	}

	if tb.Columns() == nil {
		return nil, fmt.Errorf("columns block is not found")
	}
	if tb.Rows() == nil {
		return nil, fmt.Errorf("rows block is not found")
	}

	// we don't want divs in tables
	destructureDivs(s, tb.Rows().Id)
	destructureDivs(s, tb.Columns().Id)
	for _, rowID := range tb.RowIDs() {
		destructureDivs(s, rowID)
	}

	return &tb, nil
}

// GetTableRootBlock iterates over parents of block. Returns nil in case root table block is not found
func GetTableRootBlock(s *state.State, id string) (block simple.Block) {
	next := s.Pick(id)
	for next != nil {
		if next.Model().GetTable() != nil {
			block = next
			break
		}
		next = s.PickParentOf(next.Model().Id)
	}
	return block
}

// destructureDivs removes child dividers from block
func destructureDivs(s *state.State, blockID string) {
	parent := s.Pick(blockID)
	if parent == nil {
		return
	}
	var foundDiv bool
	var ids []string
	for _, id := range parent.Model().ChildrenIds {
		b := s.Pick(id)
		if b == nil {
			continue
		}
		if b.Model().GetLayout().GetStyle() == model.BlockContentLayout_Div {
			foundDiv = true
			ids = append(ids, b.Model().ChildrenIds...)
			continue
		}
	}

	if foundDiv {
		parent = s.Get(blockID)
		parent.Model().ChildrenIds = ids
	}
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

func (tb Table) ColumnIDs() []string {
	return tb.Columns().ChildrenIds
}

func (tb Table) MakeColumnIndex() map[string]int {
	colIdx := map[string]int{}
	for i, c := range tb.ColumnIDs() {
		colIdx[c] = i
	}
	return colIdx
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

func (tb Table) RowIDs() []string {
	return tb.Rows().ChildrenIds
}

func (tb Table) PickRow(rowID string) (simple.Block, error) {
	return pickRow(tb.s, rowID)
}

type CellPosition struct {
	RowID, ColID, CellID string
	RowNumber, ColNumber int
}

// Iterate iterates by each existing cells
func (tb Table) Iterate(f func(b simple.Block, pos CellPosition) bool) error {
	colIndex := tb.MakeColumnIndex()

	for rowNumber, rowID := range tb.RowIDs() {
		row, err := pickRow(tb.s, rowID)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowID, err)
		}

		for _, cellID := range row.Model().ChildrenIds {
			_, colID, err := ParseCellID(cellID)
			if err != nil {
				return fmt.Errorf("parse cell id %s: %w", cellID, err)
			}

			colNumber := colIndex[colID]

			ok := f(tb.s.Pick(cellID), CellPosition{
				RowID: rowID, RowNumber: rowNumber,
				ColID: colID, ColNumber: colNumber,
				CellID: cellID,
			})
			if !ok {
				return nil
			}
		}
	}
	return nil
}

// CheckTableBlocksMove checks if Insert operation is allowed in case table blocks are affected
func CheckTableBlocksMove(st *state.State, target string, pos model.BlockPosition, blockIds []string) (string, model.BlockPosition, error) {
	// nolint:errcheck
	if t, _ := NewTable(st, target); t != nil {
		// we allow moving rows between each other
		if slice.ContainsAll(t.RowIDs(), append(blockIds, target)...) {
			if pos == model.Block_Bottom || pos == model.Block_Top {
				return target, pos, nil
			}
			return "", 0, fmt.Errorf("failed to move rows: position should be Top or Bottom, got %s", model.BlockPosition_name[int32(pos)])
		}
	}

	for _, id := range blockIds {
		t := GetTableRootBlock(st, id)
		if t != nil && t.Model().Id != id {
			// we should not move table blocks except table root block
			return "", 0, ErrCannotMoveTableBlocks
		}
	}

	t := GetTableRootBlock(st, target)
	if t != nil && t.Model().Id != target {
		// we allow inserting blocks into table cell
		if isTableCell(target) && slices.Contains([]model.BlockPosition{model.Block_Inner, model.Block_Replace, model.Block_InnerFirst}, pos) {
			return target, pos, nil
		}

		// if the target is one of table blocks, but not cell or table root, we should insert blocks under the table
		return t.Model().Id, model.Block_Bottom, nil
	}

	return target, pos, nil
}

package table

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/table"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("anytype-simple-tables")

var ErrCannotMoveTableBlocks = fmt.Errorf("can not move table blocks")

var (
	errNotARow        = fmt.Errorf("block is not a row")
	errNotAColumn     = fmt.Errorf("block is not a column")
	errRowNotFound    = fmt.Errorf("row is not found")
	errColumnNotFound = fmt.Errorf("column is not found")
)

type tableSorter struct {
	rowIDs   []string
	values   []string
	collator *collate.Collator
}

func (t tableSorter) Len() int {
	return len(t.rowIDs)
}

func (t tableSorter) Less(i, j int) bool {
	valI := strings.TrimSpace(t.values[i])
	valJ := strings.TrimSpace(t.values[j])

	// Try to parse both values as numbers
	numI, errI := strconv.ParseFloat(valI, 64)
	numJ, errJ := strconv.ParseFloat(valJ, 64)

	// If both values are valid numbers, compare numerically
	if errI == nil && errJ == nil {
		return numI < numJ
	}

	// If only one is a number, numbers come before strings
	if errI == nil {
		return true
	}
	if errJ == nil {
		return false
	}

	// Both are strings, use collator for text comparison
	return t.collator.CompareString(strings.ToLower(valI), strings.ToLower(valJ)) < 0
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
		return nil, errRowNotFound
	}
	_, ok := b.(table.RowBlock)
	if !ok {
		return nil, errNotARow
	}
	return b, nil
}

func pickRow(s *state.State, id string) (simple.Block, error) {
	b := s.Pick(id)
	if b == nil {
		return nil, errRowNotFound
	}
	if b.Model().GetTableRow() == nil {
		return nil, errNotARow
	}
	return b, nil
}

func makeColumn(id string) simple.Block {
	return simple.New(&model.Block{
		Id: id,
		Content: &model.BlockContentOfTableColumn{
			TableColumn: &model.BlockContentTableColumn{},
		},
	})
}

func pickColumn(s *state.State, id string) (simple.Block, error) {
	b := s.Pick(id)
	if b == nil {
		return nil, errColumnNotFound
	}
	if b.Model().GetTableColumn() == nil {
		return nil, errNotAColumn
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

	tb.block = PickTableRootBlock(s, id)
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

// PickTableRootBlock iterates over parents of block. Returns nil in case root table block is not found
func PickTableRootBlock(s *state.State, id string) (block simple.Block) {
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

func (tb Table) MoveBlocksUnderTheTable(ids ...string) {
	parent := tb.s.GetParentOf(tb.block.Model().Id)
	if parent == nil {
		log.Errorf("failed to get parent of table block '%s'", tb.block.Model().Id)
		return
	}
	children := parent.Model().ChildrenIds
	pos := slice.FindPos(children, tb.block.Model().Id)
	if pos == -1 {
		log.Errorf("failed to find table block '%s' among children of block '%s'", tb.block.Model().Id, parent.Model().Id)
		return
	}
	tb.s.SetChildrenIds(parent.Model(), slice.Insert(children, pos+1, ids...))
}

// CheckTableBlocksMove checks if Insert operation is allowed in case table blocks are affected
func CheckTableBlocksMove(st *state.State, target string, pos model.BlockPosition, blockIds []string) (string, model.BlockPosition, error) {
	if t, err := NewTable(st, target); err == nil && t != nil {
		// we allow moving rows between each other
		if lo.Every(t.RowIDs(), append(blockIds, target)) {
			if pos == model.Block_Bottom || pos == model.Block_Top {
				return target, pos, nil
			}
			return "", 0, fmt.Errorf("failed to move rows: position should be Top or Bottom, got %s", model.BlockPosition_name[int32(pos)])
		}
	}

	for _, id := range blockIds {
		t := PickTableRootBlock(st, id)
		if t != nil && t.Model().Id != id {
			// we should not move table blocks except table root block
			return "", 0, ErrCannotMoveTableBlocks
		}
	}

	t := PickTableRootBlock(st, target)
	if t != nil && t.Model().Id != target {
		// if the target is one of table blocks, but not table root, we should insert blocks under the table
		return t.Model().Id, model.Block_Bottom, nil
	}

	return target, pos, nil
}

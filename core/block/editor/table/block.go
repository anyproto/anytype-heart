package table

import (
	"errors"
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBlock)
}

func NewBlock(b *model.Block) simple.Block {
	if c := b.GetTable(); c != nil {
		return &block{
			Base: base.NewBase(b).(*base.Base),
		}
	}
	return nil
}

type Block interface {
	simple.Block
	Duplicate(s *state.State) (newID string, visitedIds []string, blocks []simple.Block, err error)
	Normalize(s *state.State) error
}

type block struct {
	*base.Base
}

func (b *block) Copy() simple.Block {
	return NewBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *block) Duplicate(s *state.State) (newID string, visitedIds []string, blocks []simple.Block, err error) {
	tb, err := NewTable(s, b.Id)
	if err != nil {
		err = fmt.Errorf("init table: %w", err)
		return
	}
	visitedIds = append(visitedIds, b.Id)

	colMapping := map[string]string{}
	genID := func() string {
		return bson.NewObjectId().Hex()
	}

	cols := pbtypes.CopyBlock(tb.Columns())
	visitedIds = append(visitedIds, cols.Id)
	cols.Id = ""
	for i, colID := range cols.ChildrenIds {
		col := s.Pick(colID)
		if col == nil {
			err = fmt.Errorf("column %s is not found", colID)
			return
		}
		visitedIds = append(visitedIds, colID)
		col = col.Copy()
		col.Model().Id = genID()
		blocks = append(blocks, col)
		colMapping[colID] = col.Model().Id
		cols.ChildrenIds[i] = col.Model().Id
	}
	blocks = append(blocks, simple.New(cols))

	rows := pbtypes.CopyBlock(tb.Rows())
	visitedIds = append(visitedIds, rows.Id)
	rows.Id = ""
	for i, rowID := range rows.ChildrenIds {
		visitedIds = append(visitedIds, rowID)
		row := s.Pick(rowID)
		row = row.Copy()
		row.Model().Id = genID()
		blocks = append(blocks, row)

		for j, cellID := range row.Model().ChildrenIds {
			_, oldColID, err2 := ParseCellID(cellID)
			if err2 != nil {
				err = fmt.Errorf("parse cell id %s: %w", cellID, err2)
				return
			}

			newColID, ok := colMapping[oldColID]
			if !ok {
				err = fmt.Errorf("column mapping for %s is not found", oldColID)
				return
			}
			visitedIds = append(visitedIds, cellID)
			cell := s.Pick(cellID)
			cell = cell.Copy()
			cell.Model().Id = MakeCellID(row.Model().Id, newColID)
			blocks = append(blocks, cell)

			row.Model().ChildrenIds[j] = cell.Model().Id
		}
		rows.ChildrenIds[i] = row.Model().Id
	}
	blocks = append(blocks, simple.New(rows))

	block := tb.block.Copy()
	block.Model().Id = genID()
	block.Model().ChildrenIds = []string{cols.Id, rows.Id}
	blocks = append(blocks, block)

	return block.Model().Id, visitedIds, blocks, nil
}

func (b *block) Normalize(s *state.State) error {
	tb, err := NewTable(s, b.Id)
	if err != nil {
		log.Errorf("normalize table %s: broken table state: %s", b.Id, err)
		if !s.Unlink(b.Id) {
			log.Errorf("failed to unlink table block: %s", b.Id)
		}
		return nil
	}

	tb.normalizeColumns()
	tb.normalizeRows()
	if err = tb.normalizeHeaderRows(); err != nil {
		// actually we cannot get error here, as all rows are checked in normalizeRows
		log.Errorf("normalize header rows: %v", err)
	}
	return nil
}

type rowSort struct {
	indices []int
	cells   []string
	touched bool
}

func (r *rowSort) Len() int {
	return len(r.cells)
}

func (r *rowSort) Less(i, j int) bool {
	return r.indices[i] < r.indices[j]
}

func (r *rowSort) Swap(i, j int) {
	r.touched = true
	r.indices[i], r.indices[j] = r.indices[j], r.indices[i]
	r.cells[i], r.cells[j] = r.cells[j], r.cells[i]
}

func (tb Table) normalizeHeaderRows() error {
	rows := tb.s.Get(tb.Rows().Id)

	var headers []string
	regular := make([]string, 0, len(rows.Model().ChildrenIds))
	for _, rowID := range rows.Model().ChildrenIds {
		row, err := pickRow(tb.s, rowID)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowID, err)
		}

		if row.Model().GetTableRow().IsHeader {
			headers = append(headers, rowID)
		} else {
			regular = append(regular, rowID)
		}
	}

	tb.s.SetChildrenIds(rows.Model(), append(headers, regular...))
	return nil
}

func (tb Table) normalizeRow(colIdx map[string]int, row simple.Block) {
	if row == nil || row.Model() == nil {
		return
	}

	if colIdx == nil {
		colIdx = tb.MakeColumnIndex()
	}

	rs := &rowSort{
		cells:   make([]string, 0, len(row.Model().ChildrenIds)),
		indices: make([]int, 0, len(row.Model().ChildrenIds)),
	}
	toRemove := []string{}
	for _, id := range row.Model().ChildrenIds {
		_, colID, err := ParseCellID(id)
		if err != nil {
			log.Warnf("normalize row %s: move cell %s under the table: invalid id", row.Model().Id, id)
			toRemove = append(toRemove, id)
			rs.touched = true
			continue
		}

		v, ok := colIdx[colID]
		if !ok {
			log.Warnf("normalize row %s: move cell %s under the table: column %s not found", row.Model().Id, id, colID)
			toRemove = append(toRemove, id)
			rs.touched = true
			continue
		}
		rs.cells = append(rs.cells, id)
		rs.indices = append(rs.indices, v)
	}
	sort.Sort(rs)

	if rs.touched {
		tb.MoveBlocksUnderTheTable(toRemove...)
		tb.s.SetChildrenIds(row.Model(), rs.cells)
	}
}

func (tb Table) normalizeColumns() {
	var (
		invalidFound bool
		colIds       = make([]string, 0)
		toRemove     = make([]string, 0)
	)

	for _, colId := range tb.ColumnIDs() {
		if _, err := pickColumn(tb.s, colId); err != nil {
			invalidFound = true
			switch {
			case errors.Is(err, errColumnNotFound):
				// Fix data integrity by adding missing column
				log.Warnf("normalize columns '%s': column '%s' is not found: recreating it", tb.Columns().Id, colId)
				col := makeColumn(colId)
				if !tb.s.Add(col) {
					log.Errorf("add missing column block %s", colId)
					toRemove = append(toRemove, colId)
					continue
				}
				colIds = append(colIds, colId)
			case errors.Is(err, errNotAColumn):
				log.Warnf("normalize columns '%s': block '%s' is not a column: move it under the table", tb.Columns().Id, colId)
				tb.MoveBlocksUnderTheTable(colId)
			default:
				log.Errorf("pick column %s: %v", colId, err)
				toRemove = append(toRemove, colId)
			}
			continue
		}
		colIds = append(colIds, colId)
	}

	if invalidFound {
		tb.s.SetChildrenIds(tb.Columns(), colIds)
	}
}

func (tb Table) normalizeRows() {
	var (
		invalidFound bool
		rowIds       = make([]string, 0)
		toRemove     = make([]string, 0)
		colIdx       = tb.MakeColumnIndex()
	)

	for _, rowId := range tb.RowIDs() {
		row, err := getRow(tb.s, rowId)
		if err != nil {
			invalidFound = true
			switch {
			case errors.Is(err, errRowNotFound):
				// Fix data integrity by adding missing row
				log.Warnf("normalize rows '%s': row '%s' is not found: recreating it", tb.Rows().Id, rowId)
				row = makeRow(rowId)
				if !tb.s.Add(row) {
					log.Errorf("add missing row block %s", rowId)
					toRemove = append(toRemove, rowId)
					continue
				}
				rowIds = append(rowIds, rowId)
			case errors.Is(err, errNotARow):
				log.Warnf("normalize rows '%s': block '%s' is not a row: move it under the table", tb.Rows().Id, rowId)
				tb.MoveBlocksUnderTheTable(rowId)
			default:
				log.Errorf("get row %s: %v", rowId, err)
				toRemove = append(toRemove, rowId)
			}
			continue
		}
		tb.normalizeRow(colIdx, row)
		rowIds = append(rowIds, rowId)
	}

	if invalidFound {
		tb.s.SetChildrenIds(tb.Rows(), rowIds)
	}
}

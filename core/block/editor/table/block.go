package table

import (
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBlock)
}

func NewBlock(b *model.Block) simple.Block {
	if c := b.GetTable(); c != nil {
		return &block{
			Base:    base.NewBase(b).(*base.Base),
			content: c,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	Normalize(s *state.State) error
	Duplicate(s *state.State) (newID string, visitedIds []string, blocks []simple.Block, err error)
}

type block struct {
	*base.Base
	content *model.BlockContentTable
}

func (b *block) Copy() simple.Block {
	return NewBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *block) Normalize(s *state.State) error {
	tb, err := NewTable(s, b.Id)
	if err != nil {
		log.Errorf("normalize table %s: broken table state: %s", b.Model().Id, err)
		return nil
	}

	colIdx := map[string]int{}
	for i, c := range tb.ColumnIDs() {
		colIdx[c] = i
	}

	for _, rowID := range tb.RowIDs() {
		row := s.Get(rowID)
		// Fix data integrity by adding missing row
		if row == nil {
			row = makeRow(rowID)
			if !s.Add(row) {
				return fmt.Errorf("add missing row block %s", rowID)
			}
			continue
		}
		normalizeRow(colIdx, row)
	}

	if err := normalizeRows(s, tb); err != nil {
		return fmt.Errorf("normalize rows: %w", err)
	}
	return nil
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
			cell.Model().Id = makeCellID(row.Model().Id, newColID)
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

func normalizeRows(s *state.State, tb *Table) error {
	rows := s.Get(tb.Rows().Id)

	var headers []string
	regular := make([]string, 0, len(rows.Model().ChildrenIds))

	for _, rowID := range rows.Model().ChildrenIds {
		row, err := pickRow(s, rowID)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowID, err)
		}

		if row.Model().GetTableRow().IsHeader {
			headers = append(headers, rowID)
		} else {
			regular = append(regular, rowID)
		}
	}

	// nolint:gocritic
	rows.Model().ChildrenIds = append(headers, regular...)
	return nil
}

func normalizeRow(colIdx map[string]int, row simple.Block) {
	if row == nil || row.Model() == nil {
		return
	}
	rs := &rowSort{
		cells:   make([]string, 0, len(row.Model().ChildrenIds)),
		indices: make([]int, 0, len(row.Model().ChildrenIds)),
	}
	for _, id := range row.Model().ChildrenIds {
		_, colID, err := ParseCellID(id)
		if err != nil {
			log.Warnf("normalize row %s: discard cell %s: invalid id", row.Model().Id, id)
			rs.touched = true
			continue
		}

		v, ok := colIdx[colID]
		if !ok {
			log.Warnf("normalize row %s: discard cell %s: column %s not found", row.Model().Id, id, colID)
			rs.touched = true
			continue
		}
		rs.cells = append(rs.cells, id)
		rs.indices = append(rs.indices, v)
	}
	sort.Sort(rs)

	if rs.touched {
		row.Model().ChildrenIds = rs.cells
	}
}

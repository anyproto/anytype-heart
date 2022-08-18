package table

import (
	"fmt"
	"sort"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
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
	Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error)
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
	for i, c := range tb.Columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, rowId := range tb.Rows().ChildrenIds {
		row := s.Get(rowId)
		normalizeRow(colIdx, row)
	}

	if err := normalizeRows(s, tb); err != nil {
		return fmt.Errorf("normalize rows: %w", err)
	}
	return nil
}

func (b *block) Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error) {
	tb, err := NewTable(s, b.Id)
	if err != nil {
		err = fmt.Errorf("init table: %w", err)
		return
	}
	visitedIds = append(visitedIds, b.Id)

	colMapping := map[string]string{}
	genId := func() string {
		return bson.NewObjectId().Hex()
	}

	cols := pbtypes.CopyBlock(tb.Columns())
	visitedIds = append(visitedIds, cols.Id)
	cols.Id = ""
	for i, colId := range cols.ChildrenIds {
		col := s.Pick(colId)
		if col == nil {
			err = fmt.Errorf("column %s is not found", colId)
			return
		}
		visitedIds = append(visitedIds, colId)
		col = col.Copy()
		col.Model().Id = genId()
		blocks = append(blocks, col)
		colMapping[colId] = col.Model().Id
		cols.ChildrenIds[i] = col.Model().Id
	}
	blocks = append(blocks, simple.New(cols))

	rows := pbtypes.CopyBlock(tb.Rows())
	visitedIds = append(visitedIds, rows.Id)
	rows.Id = ""
	for i, rowId := range rows.ChildrenIds {
		visitedIds = append(visitedIds, rowId)
		row := s.Pick(rowId)
		row = row.Copy()
		row.Model().Id = genId()
		blocks = append(blocks, row)

		for j, cellId := range row.Model().ChildrenIds {
			_, oldColId, err2 := ParseCellId(cellId)
			if err2 != nil {
				err = fmt.Errorf("parse cell id %s: %w", cellId, err2)
				return
			}

			newColId, ok := colMapping[oldColId]
			if !ok {
				err = fmt.Errorf("column mapping for %s is not found", oldColId)
				return
			}
			visitedIds = append(visitedIds, cellId)
			cell := s.Pick(cellId)
			cell = cell.Copy()
			cell.Model().Id = makeCellId(row.Model().Id, newColId)
			blocks = append(blocks, cell)

			row.Model().ChildrenIds[j] = cell.Model().Id
		}
		rows.ChildrenIds[i] = row.Model().Id
	}
	blocks = append(blocks, simple.New(rows))

	block := tb.block.Copy()
	block.Model().Id = genId()
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
	normal := make([]string, 0, len(rows.Model().ChildrenIds))

	for _, rowId := range rows.Model().ChildrenIds {
		row, err := pickRow(s, rowId)
		if err != nil {
			return fmt.Errorf("pick row %s: %w", rowId, err)
		}

		if row.Model().GetTableRow().IsHeader {
			headers = append(headers, rowId)
		} else {
			normal = append(normal, rowId)
		}
	}

	rows.Model().ChildrenIds = append(headers, normal...)
	return nil
}

func normalizeRow(colIdx map[string]int, row simple.Block) {
	rs := &rowSort{
		cells:   make([]string, 0, len(row.Model().ChildrenIds)),
		indices: make([]int, 0, len(row.Model().ChildrenIds)),
	}
	for _, id := range row.Model().ChildrenIds {
		_, colId, err := ParseCellId(id)
		if err != nil {
			log.Warnf("normalize row %s: discard cell %s: invalid id", row.Model().Id, id)
			rs.touched = true
			continue
		}

		v, ok := colIdx[colId]
		if !ok {
			log.Warnf("normalize row %s: discard cell %s: column %s not found", row.Model().Id, id, colId)
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

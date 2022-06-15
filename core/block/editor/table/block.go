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
	Duplicate(s *state.State) (newId string, err error)
}

type block struct {
	*base.Base
	content *model.BlockContentTable
}

func (b block) Copy() simple.Block {
	return NewBlock(pbtypes.CopyBlock(b.Model()))
}

func (b *block) Normalize(s *state.State) error {
	tb, err := newTableBlockFromState(s, b.Id)
	if err != nil {
		log.Errorf("normalize table %s: broken table state", b.Model().Id)
		return nil
	}

	colIdx := map[string]int{}
	for i, c := range tb.columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, rowId := range tb.rows().ChildrenIds {
		row := s.Get(rowId)
		normalizeRow(colIdx, row)
	}
	return nil
}

func (b block) Duplicate(s *state.State) (newId string, err error) {
	tb, err := newTableBlockFromState(s, b.Id)
	if err != nil {
		return "", fmt.Errorf("init table: %w", err)
	}

	colMapping := map[string]string{}
	genId := func() string {
		return bson.NewObjectId().Hex()
	}

	cols := pbtypes.CopyBlock(tb.columns())
	cols.Id = ""
	for i, colId := range cols.ChildrenIds {
		col := s.Pick(colId)
		if col == nil {
			return "", fmt.Errorf("column %s is not found", colId)
		}
		col = col.Copy()
		col.Model().Id = genId()
		if !s.Add(col) {
			return "", fmt.Errorf("add copy of column %s", colId)
		}
		colMapping[colId] = col.Model().Id
		cols.ChildrenIds[i] = col.Model().Id
	}
	if !s.Add(simple.New(cols)) {
		return "", fmt.Errorf("add copy of columns")
	}

	rows := pbtypes.CopyBlock(tb.rows())
	rows.Id = ""
	for i, rowId := range rows.ChildrenIds {
		row := s.Pick(rowId)
		row = row.Copy()
		row.Model().Id = genId()
		if !s.Add(row) {
			return "", fmt.Errorf("add copy of row %s", rowId)
		}

		for j, cellId := range row.Model().ChildrenIds {
			_, oldColId, err := parseCellId(cellId)
			if err != nil {
				return "", fmt.Errorf("parse cell id %s: %w", cellId, err)
			}

			newColId, ok := colMapping[oldColId]
			if !ok {
				return "", fmt.Errorf("column mapping for %s is not found", oldColId)
			}
			cell := s.Pick(cellId)
			cell = cell.Copy()
			cell.Model().Id = makeCellId(row.Model().Id, newColId)
			if !s.Add(cell) {
				return "", fmt.Errorf("add copy of cell %s", cellId)
			}
			row.Model().ChildrenIds[j] = cell.Model().Id
		}
		rows.ChildrenIds[i] = row.Model().Id
	}
	if !s.Add(simple.New(rows)) {
		return "", fmt.Errorf("add copy of rows")
	}

	block := tb.block.Copy()
	block.Model().Id = genId()
	block.Model().ChildrenIds = []string{cols.Id, rows.Id}
	if !s.Add(block) {
		return "", fmt.Errorf("add copy of table")
	}
	return block.Model().Id, nil
}

type rowSort struct {
	indices []int
	cells   []string
	touched bool
}

func (r rowSort) Len() int {
	return len(r.cells)
}

func (r rowSort) Less(i, j int) bool {
	return r.indices[i] < r.indices[j]
}

func (r *rowSort) Swap(i, j int) {
	r.touched = true
	r.indices[i], r.indices[j] = r.indices[j], r.indices[i]
	r.cells[i], r.cells[j] = r.cells[j], r.cells[i]
}

func normalizeRow(colIdx map[string]int, row simple.Block) {
	rs := &rowSort{
		cells:   make([]string, 0, len(row.Model().ChildrenIds)),
		indices: make([]int, 0, len(row.Model().ChildrenIds)),
	}
	for _, id := range row.Model().ChildrenIds {
		_, colId, err := parseCellId(id)
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

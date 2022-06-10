package table

import (
	"fmt"
	"sort"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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
}

type block struct {
	*base.Base
	content *model.BlockContentTable
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

func normalizeRow(s *state.State, colIdx map[string]int, row simple.Block) error {
	rs := &rowSort{
		cells:   row.Model().ChildrenIds,
		indices: make([]int, 0, len(row.Model().ChildrenIds)),
	}
	for _, id := range row.Model().ChildrenIds {
		_, colId, err := parseCellId(id)
		if err != nil {
			return fmt.Errorf("parse cell id: %w", err)
		}

		v, ok := colIdx[colId]
		if !ok {
			// TODO: maybe delete cell?
			return fmt.Errorf("bad cell id=%s: column not found", id)
		}
		rs.indices = append(rs.indices, v)
	}

	sort.Sort(rs)

	if rs.touched {
		row.Model().ChildrenIds = rs.cells
		s.Set(row)
	}
	return nil
}

func (b block) Normalize(s *state.State) error {
	tb, err := newTableBlockFromState(s, b.Id)
	if err != nil {
		return err
	}

	colIdx := map[string]int{}
	for i, c := range tb.columns().ChildrenIds {
		colIdx[c] = i
	}

	for _, rowId := range tb.rows().ChildrenIds {
		row := s.Pick(rowId)
		if err := normalizeRow(s, colIdx, row); err != nil {
			return fmt.Errorf("normalize row %s: %w", rowId, err)
		}
	}
	return nil
}

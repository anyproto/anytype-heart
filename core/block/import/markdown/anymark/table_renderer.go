package anymark

import (
	"bytes"

	"github.com/globalsign/mgo/bson"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	table "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	table2 "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type columnCreator interface {
	createColumns(ts *tableState, br *blocksRenderer)
	changeState(ts *tableState, creator columnCreator)
	addColumn()
}

type noColumnsState struct {
	columnWidth int64
}

func (n *noColumnsState) createColumns(ts *tableState, br *blocksRenderer) {
	for _, block := range br.blocks {
		if layout, ok := block.Content.(*model.BlockContentOfLayout); ok &&
			layout.Layout.Style == model.BlockContentLayout_TableColumns {
			columnIds := make([]string, 0, n.columnWidth)
			for i := int64(0); i < n.columnWidth; i++ {
				id := bson.NewObjectId().Hex()
				bl := &model.Block{
					Id:      id,
					Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}},
				}
				columnIds = append(columnIds, id)
				br.blocks = append(br.blocks, bl)
			}
			block.ChildrenIds = append(block.ChildrenIds, columnIds...)
			break
		}
	}
	n.changeState(ts, &alreadyHasColumnsState{})
}

func (n *noColumnsState) addColumn() {
	n.columnWidth++
}

func (n *noColumnsState) changeState(ts *tableState, creator columnCreator) {
	ts.setColumnState(creator)
}

type alreadyHasColumnsState struct{}

func (a *alreadyHasColumnsState) changeState(ts *tableState, creator columnCreator) {}

func (a *alreadyHasColumnsState) createColumns(ts *tableState, br *blocksRenderer) {}

func (a *alreadyHasColumnsState) addColumn() {}

type rowState struct {
	currTableRow *model.Block
}

func (r *rowState) addRow(br *blocksRenderer, isHeader bool) {
	id := bson.NewObjectId().Hex()
	blRow := &model.Block{
		Id:      id,
		Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: isHeader}},
	}
	r.currTableRow = blRow
	for _, block := range br.blocks {
		if layout, ok := block.Content.(*model.BlockContentOfLayout); ok &&
			layout.Layout.Style == model.BlockContentLayout_TableRows {
			block.ChildrenIds = append(block.ChildrenIds, id)
			break
		}
	}
	br.blocks = append(br.blocks, blRow)
}

func (r *rowState) getRow() *model.Block {
	return r.currTableRow
}

type tableState struct {
	listRenderFunctions map[ast.NodeKind]renderer.NodeRendererFunc
	rowState            rowState
	columnsState        columnCreator
}

func (s *tableState) setRenderFunction(kind ast.NodeKind, rendererFunc renderer.NodeRendererFunc) {
	s.listRenderFunctions[kind] = rendererFunc
}

func (s *tableState) setColumnState(creator columnCreator) {
	s.columnsState = creator
}

type TableRenderer struct {
	blockRenderer *blocksRenderer
	tableState    *tableState
}

func NewTableRenderer(br *blocksRenderer) *TableRenderer {
	return &TableRenderer{
		blockRenderer: br,
		tableState: &tableState{
			listRenderFunctions: make(map[ast.NodeKind]renderer.NodeRendererFunc, 0),
			rowState:            rowState{},
		},
	}
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs .
func (r *TableRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(table.KindTable, r.renderTable)
	r.tableState.setRenderFunction(table.KindTable, r.renderTable)

	reg.Register(table.KindTableHeader, r.renderTableHeader)
	r.tableState.setRenderFunction(table.KindTableHeader, r.renderTableHeader)

	reg.Register(table.KindTableRow, r.renderTableRow)
	r.tableState.setRenderFunction(table.KindTableRow, r.renderTableRow)

	reg.Register(table.KindTableCell, r.renderTableCell)
	r.tableState.setRenderFunction(table.KindTableCell, r.renderTableCell)
}

func (r *TableRenderer) renderTable(_ util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		// create table block and layouts blocks
		layoutID := bson.NewObjectId().Hex()
		layout := &model.Block{
			Id: layoutID,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableRows,
				},
			},
		}
		layoutColumnID := bson.NewObjectId().Hex()
		layoutColumn := &model.Block{
			Id: layoutColumnID,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableColumns,
				},
			},
		}
		r.blockRenderer.blocks = append(r.blockRenderer.blocks, &model.Block{
			Id:          bson.NewObjectId().Hex(),
			ChildrenIds: []string{layoutID, layoutColumnID},
			Content:     &model.BlockContentOfTable{Table: &model.BlockContentTable{}},
		})
		r.blockRenderer.blocks = append(r.blockRenderer.blocks, layout)
		r.blockRenderer.blocks = append(r.blockRenderer.blocks, layoutColumn)
		r.tableState.setColumnState(&noColumnsState{columnWidth: 0})
		return ast.WalkContinue, nil
	}
	// changing id of results blocks
	var (
		columns   []string
		textBlock = make(map[string]*model.Block, 0)
	)
	for _, block := range r.blockRenderer.blocks {
		if layout, ok := block.Content.(*model.BlockContentOfLayout); ok {
			if layout.Layout.Style == model.BlockContentLayout_TableColumns {
				columns = append(columns, block.ChildrenIds...)
			}
		}
		if _, ok := block.Content.(*model.BlockContentOfText); ok {
			textBlock[block.Id] = block
		}
	}

	for _, block := range r.blockRenderer.blocks {
		if _, ok := block.Content.(*model.BlockContentOfTableRow); ok {
			newChildren := make([]string, 0, len(block.ChildrenIds))
			for i, id := range block.ChildrenIds {
				tb := textBlock[id]
				tb.Id = table2.MakeCellID(block.Id, columns[i])
				newChildren = append(newChildren, tb.Id)
			}
			block.ChildrenIds = newChildren
		}
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableHeader(_ util.BufWriter,
	_ []byte,
	_ ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		isHeader := true
		r.tableState.rowState.addRow(r.blockRenderer, isHeader)
	} else {
		r.tableState.columnsState.createColumns(r.tableState, r.blockRenderer)
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableRow(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		isHeader := false
		r.tableState.rowState.addRow(r.blockRenderer, isHeader)
	} else {
		r.tableState.columnsState.createColumns(r.tableState, r.blockRenderer)
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableCell(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		if node != nil {
			// recursive handler of markdown inside table cell
			ren := NewRenderer(newBlocksRenderer("", nil))
			gm := goldmark.New(goldmark.WithRenderer(
				renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(ren, 100))),
			))
			n := node.Lines()
			for i := 0; i < n.Len(); i++ {
				seg := n.At(i)
				err := gm.Convert(seg.Value(source), &bytes.Buffer{})
				if err != nil {
					return ast.WalkContinue, err
				}

				for _, block := range ren.GetBlocks() {
					currRow := r.tableState.rowState.getRow()
					currRow.ChildrenIds = append(currRow.ChildrenIds, block.Id)
				}
				r.blockRenderer.blocks = append(r.blockRenderer.blocks, ren.GetBlocks()...)
				r.tableState.columnsState.addColumn()
			}
		}
	}
	return ast.WalkContinue, nil
}

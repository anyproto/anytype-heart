package anymark

import (
	"bytes"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	table "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	te "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type rowState struct {
	currTableRow string
}

func (r *rowState) getRowID() string {
	return r.currTableRow
}

func (r *rowState) setRowID(id string) {
	r.currTableRow = id
}

type tableState struct {
	listRenderFunctions map[ast.NodeKind]renderer.NodeRendererFunc
	rowState            rowState
}

func (s *tableState) setRenderFunction(kind ast.NodeKind, rendererFunc renderer.NodeRendererFunc) {
	s.listRenderFunctions[kind] = rendererFunc
}

type TableRenderer struct {
	blockRenderer *blocksRenderer
	tableState    *tableState
	tableEditor   te.TableEditor
	state         *state.State
}

func NewTableRenderer(br *blocksRenderer, tableEditor te.TableEditor) *TableRenderer {
	return &TableRenderer{
		blockRenderer: br,
		tableState: &tableState{
			listRenderFunctions: make(map[ast.NodeKind]renderer.NodeRendererFunc, 0),
			rowState:            rowState{},
		},
		tableEditor: tableEditor,
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
		r.state = state.NewDoc("root", nil).NewState()
		_, err := r.tableEditor.TableCreate(r.state, pb.RpcBlockTableCreateRequest{})
		if err != nil {
			return ast.WalkContinue, err
		}
		return ast.WalkContinue, nil
	} else {
		r.blockRenderer.blocks = append(r.blockRenderer.blocks, r.state.Blocks()...)
		r.state = nil
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableHeader(_ util.BufWriter,
	_ []byte,
	_ ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		id, err := r.tableEditor.RowCreate(r.state, pb.RpcBlockTableRowCreateRequest{
			Position: model.Block_Bottom,
		})
		if err != nil {
			return ast.WalkContinue, err
		}
		r.tableState.rowState.setRowID(id)
		err = r.tableEditor.RowSetHeader(r.state, pb.RpcBlockTableRowSetHeaderRequest{
			IsHeader: true,
		})
		if err != nil {
			return ast.WalkContinue, err
		}
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableRow(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		id, err := r.tableEditor.RowCreate(r.state, pb.RpcBlockTableRowCreateRequest{
			Position: model.Block_Bottom,
		})
		r.tableState.rowState.setRowID(id)
		if err != nil {
			return ast.WalkContinue, err
		}
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

				colID, err := r.tableEditor.ColumnCreate(r.state, pb.RpcBlockTableColumnCreateRequest{
					Position: model.Block_Right,
				})
				if err != nil {
					return ast.WalkContinue, err
				}
				if len(ren.GetBlocks()) != 0 {
					_, err = r.tableEditor.CellCreate(r.state, r.tableState.rowState.getRowID(), colID, ren.GetBlocks()[0])
					if err != nil {
						return ast.WalkContinue, err
					}
				}
			}
		}
	}
	return ast.WalkContinue, nil
}

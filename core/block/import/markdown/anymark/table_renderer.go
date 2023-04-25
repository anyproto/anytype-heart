package anymark

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	table "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	te "github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type tableState struct {
	listRenderFunctions map[ast.NodeKind]renderer.NodeRendererFunc
	tableID             string
	currTableRow        string
	columnsIDs          []string
	currColumnIDIndex   int
}

func (s *tableState) setRenderFunction(kind ast.NodeKind, rendererFunc renderer.NodeRendererFunc) {
	s.listRenderFunctions[kind] = rendererFunc
}

func (s *tableState) resetState() {
	s.tableID = ""
}

type TableRenderer struct {
	blockRenderer *blocksRenderer
	tableState    tableState
	tableEditor   te.TableEditor
	blocksState   *state.State
}

func NewTableRenderer(br *blocksRenderer, tableEditor te.TableEditor) *TableRenderer {
	return &TableRenderer{
		blockRenderer: br,
		tableState: tableState{
			listRenderFunctions: make(map[ast.NodeKind]renderer.NodeRendererFunc, 0),
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
		r.blocksState = state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Content: &model.BlockContentOfSmartblock{
					Smartblock: &model.BlockContentSmartblock{},
				},
			}),
		}).NewState()
		id, err := r.tableEditor.TableCreate(r.blocksState, pb.RpcBlockTableCreateRequest{})
		r.tableState.tableID = id
		if err != nil {
			return ast.WalkContinue, err
		}
		return ast.WalkContinue, nil
	}
	blocksToAdd := make([]*model.Block, 0, len(r.blocksState.Blocks()))
	for _, block := range r.blocksState.Blocks() {
		if block.GetContent() != nil {
			if _, ok := block.GetContent().(*model.BlockContentOfSmartblock); ok {
				continue
			}
		}
		blocksToAdd = append(blocksToAdd, block)
	}
	r.blockRenderer.blocks = append(r.blockRenderer.blocks, blocksToAdd...)
	r.blocksState = nil
	r.tableState.resetState()
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableHeader(_ util.BufWriter,
	_ []byte,
	_ ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		id, err := r.tableEditor.RowCreate(r.blocksState, pb.RpcBlockTableRowCreateRequest{
			Position: model.Block_Inner,
			TargetId: r.tableState.tableID,
		})
		if err != nil {
			return ast.WalkContinue, err
		}
		r.tableState.currTableRow = id
		err = r.tableEditor.RowSetHeader(r.blocksState, pb.RpcBlockTableRowSetHeaderRequest{
			IsHeader: true,
			TargetId: id,
		})
		if err != nil {
			return ast.WalkContinue, err
		}
	} else {
		// this calls after we create cells for according row. After that we don't need to create columns,
		// because we've already created them in renderTableCells
		r.tableState.currColumnIDIndex = 0
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableRow(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		id, err := r.tableEditor.RowCreate(r.blocksState, pb.RpcBlockTableRowCreateRequest{
			Position: model.Block_Inner,
			TargetId: r.tableState.tableID,
		})
		r.tableState.currTableRow = id
		if err != nil {
			return ast.WalkContinue, err
		}
	} else {
		// this calls after we create cells for according row. After that we don't need to create columns,
		// because we've already created them in renderTableCells
		r.tableState.currColumnIDIndex = 0
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) renderTableCell(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering || node == nil {
		return ast.WalkContinue, nil
	}
	if node != nil {
		// recursive handler of markdown inside table cell
		ren := NewRenderer(newBlocksRenderer("", nil))
		gm := goldmark.New(goldmark.WithRenderer(
			renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(ren, 100))),
		))
		n := node.Lines()

		status, err := r.createCell(n, gm, source, ren)
		if err != nil {
			return status, err
		}
	}
	return ast.WalkContinue, nil
}

func (r *TableRenderer) createCell(n *text.Segments,
	gm goldmark.Markdown,
	source []byte, ren *Renderer) (ast.WalkStatus, error) {
	for i := 0; i < n.Len(); i++ {
		seg := n.At(i)
		err := gm.Convert(seg.Value(source), &bytes.Buffer{})
		if err != nil {
			return ast.WalkContinue, err
		}

		colID, err := r.getColumnID()
		if err != nil {
			return ast.WalkContinue, err
		}
		if len(ren.GetBlocks()) != 0 {
			// if it's not text block - skip it, as we don't support non text blocks in tables
			block := ren.GetBlocks()[0]
			if _, ok := block.Content.(*model.BlockContentOfText); !ok {
				block.Content = &model.BlockContentOfText{Text: &model.BlockContentText{}}
			}
			_, err = r.tableEditor.CellCreate(r.blocksState, r.tableState.currTableRow, colID, block)
			if err != nil {
				return ast.WalkContinue, err
			}
		}
	}
	return 0, nil
}

func (r *TableRenderer) getColumnID() (string, error) {
	var (
		colID string
		err   error
	)
	// we create columns only once, then we use saved columns ids to create cells.
	if r.tableState.currColumnIDIndex >= len(r.tableState.columnsIDs) {
		colID, err = r.tableEditor.ColumnCreate(r.blocksState, pb.RpcBlockTableColumnCreateRequest{
			Position: model.Block_Inner,
			TargetId: r.tableState.tableID,
		})
		if err != nil {
			return "", err
		}
		r.tableState.columnsIDs = append(r.tableState.columnsIDs, colID)
	} else {
		colID = r.tableState.columnsIDs[r.tableState.currColumnIDIndex]
	}
	r.tableState.currColumnIDIndex++
	return colID, nil
}

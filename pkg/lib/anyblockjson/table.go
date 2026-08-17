package anyblockjson

// table.go maps the internal table block subtree (table → row/column layout
// wrappers → cells with composite <rowId>-<colId> ids) to the §6.1
// columns/rows JSON form and back.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const tableWidthField = "width"

// tableToJSON flattens the table subtree into columns/rows, mirroring the
// editor's normalization: header rows first, cells sorted into column order,
// orphan cells dropped. Only a structurally unrecognizable subtree (missing
// wrappers) is an error (§6.1).
func (e *exporter) tableToJSON(m *omap, b *model.Block) error {
	m.set("type", "table")

	var colsWrapper, rowsWrapper *model.Block
	for _, id := range b.ChildrenIds {
		child := e.blocks[id]
		if child == nil {
			continue
		}
		if l, ok := child.Content.(*model.BlockContentOfLayout); ok {
			switch l.Layout.Style {
			case model.BlockContentLayout_TableColumns:
				colsWrapper = child
			case model.BlockContentLayout_TableRows:
				rowsWrapper = child
			}
		}
	}
	if colsWrapper == nil || rowsWrapper == nil {
		return fmt.Errorf("table %s: missing row/column wrappers", b.Id)
	}

	var colIds []string
	var columns []any
	for _, colId := range colsWrapper.ChildrenIds {
		col := e.blocks[colId]
		if col == nil || e.visited[colId] {
			continue
		}
		if _, ok := col.Content.(*model.BlockContentOfTableColumn); !ok {
			continue // orphan blocks in the columns wrapper are dropped
		}
		e.visited[colId] = true
		colIds = append(colIds, colId)
		cm := &omap{}
		if !e.opts.OmitIds {
			cm.setNonEmpty("id", e.tableInnerId(colId))
		}
		lifted := map[string]bool{}
		if col.Fields != nil {
			if w := col.Fields.Fields[tableWidthField]; w != nil {
				if _, isNum := w.GetKind().(*types.Value_NumberValue); isNum {
					cm.setNonEmpty("width", w.GetNumberValue())
					lifted[tableWidthField] = true
				}
			}
		}
		cm.setNonEmpty("fields", e.fieldsToJSON(col.Fields, lifted))
		columns = append(columns, cm)
	}

	// header rows come first (editor invariant); stable to keep row order
	var rowBlocks []*model.Block
	for _, rowId := range rowsWrapper.ChildrenIds {
		row := e.blocks[rowId]
		if row == nil || e.visited[rowId] {
			continue
		}
		if _, ok := row.Content.(*model.BlockContentOfTableRow); !ok {
			continue
		}
		e.visited[rowId] = true
		rowBlocks = append(rowBlocks, row)
	}
	isHeader := func(b *model.Block) bool {
		return orEmpty(b.Content.(*model.BlockContentOfTableRow).TableRow).IsHeader
	}
	sort.SliceStable(rowBlocks, func(i, j int) bool {
		return isHeader(rowBlocks[i]) && !isHeader(rowBlocks[j])
	})

	var rows []any
	for _, row := range rowBlocks {
		rm := &omap{}
		if !e.opts.OmitIds {
			rm.setNonEmpty("id", e.tableInnerId(row.Id))
		}
		rm.setNonEmpty("is_header", isHeader(row))

		// cells sorted into column order; orphans dropped
		byCol := map[string]*model.Block{}
		for _, cellId := range row.ChildrenIds {
			colId, ok := strings.CutPrefix(cellId, row.Id+"-")
			if !ok {
				continue
			}
			if cell := e.blocks[cellId]; cell != nil {
				byCol[colId] = cell
			}
		}
		cells := make([]any, 0, len(colIds))
		for _, colId := range colIds {
			cell := byCol[colId]
			cv, err := e.cellToJSON(cell)
			if err != nil {
				return err
			}
			cells = append(cells, cv)
		}
		// trailing empty cells are omitted (import pads, §6.1)
		for len(cells) > 0 && cells[len(cells)-1] == nil {
			cells = cells[:len(cells)-1]
		}
		rm.setNonEmpty("cells", cells)
		rows = append(rows, rm)
	}

	m.setNonEmpty("columns", columns)
	m.setNonEmpty("rows", rows)
	return nil
}

// cellToJSON renders a cell: nil for empty, the string shorthand for a plain
// paragraph, a block object (without id — derived) otherwise. A cell whose
// block has descendants renders as an array of flat blocks — the cell block
// first at indent 0, the descendants following with their depths (§6.1 F10).
func (e *exporter) cellToJSON(cell *model.Block) (any, error) {
	if cell == nil {
		return nil, nil
	}
	if c, ok := cell.Content.(*model.BlockContentOfText); ok {
		t := orEmpty(c.Text)
		if t.Style == model.BlockContentText_Paragraph &&
			t.Color == "" && !t.Checked &&
			cell.Align == model.Block_AlignLeft &&
			cell.VerticalAlign == model.Block_VerticalAlignTop &&
			cell.BackgroundColor == "" &&
			(cell.Fields == nil || len(cell.Fields.Fields) == 0) &&
			len(cell.ChildrenIds) == 0 {
			md := renderInline(t.Text, e.compactMarks(t.Marks.GetMarks()))
			if md == "" {
				return nil, nil // empty paragraph collapses to an empty cell (§11)
			}
			return md, nil
		}
	}
	m, withChildren, err := e.blockToJSON(cell, 0)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	// cells cannot contain tables — the schema's recursion cut (§6.1, §12).
	// Erring here keeps the invariant that Marshal never emits output its
	// own Validate rejects; the prod sweep found zero such cells, so this is
	// an adversarial/legacy guard, not a live path.
	if blockJSONType(m) == "table" {
		return nil, fmt.Errorf("cell %s: a table block cannot be a cell (cells cannot contain tables)", cell.Id)
	}
	// cell ids are derived, never serialized (§6.1)
	if len(m.keys) > 0 && m.keys[0] == "id" {
		m.keys = m.keys[1:]
		m.vals = m.vals[1:]
	}
	if withChildren && len(cell.ChildrenIds) > 0 {
		arr := []any{m}
		if err := e.appendBlocksFlat(&arr, cell.ChildrenIds, 1, false); err != nil {
			return nil, err
		}
		for _, el := range arr[1:] {
			if bm, ok := el.(*omap); ok && blockJSONType(bm) == "table" {
				return nil, fmt.Errorf("cell %s: a table block among cell descendants cannot be represented (cells cannot contain tables)", cell.Id)
			}
		}
		if len(arr) > 1 {
			return arr, nil
		}
		// every descendant was dropped (visited/content-less): bare form stays
		// canonical
	}
	return m, nil
}

// blockJSONType reads the rendered block's type discriminator.
func blockJSONType(m *omap) string {
	for i, k := range m.keys {
		if k == "type" {
			s, _ := m.vals[i].(string)
			return s
		}
	}
	return ""
}

//
// ---- import ----
//

type jsonTableColumn struct {
	Id     string         `json:"id"`
	Width  float64        `json:"width"`
	Fields map[string]any `json:"fields"`
}

type jsonTableRow struct {
	Id       string     `json:"id"`
	IsHeader bool       `json:"is_header"`
	Cells    []jsonCell `json:"cells"`
}

// jsonCell is string | null | block object | array of flat blocks (§6.1).
type jsonCell struct {
	Text   *string
	Block  *jsonBlock
	Blocks []*jsonBlock // array form: cell block first, descendants flat (F10)
}

func (c *jsonCell) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := jsonUnmarshal(data, &s); err != nil {
			return err
		}
		c.Text = &s
		return nil
	}
	if strings.HasPrefix(trimmed, `[`) {
		return jsonUnmarshal(data, &c.Blocks)
	}
	var b jsonBlock
	if err := jsonUnmarshal(data, &b); err != nil {
		return err
	}
	c.Block = &b
	return nil
}

// tableFromJSON rebuilds the internal subtree. It returns the table block
// and every block of the subtree (wrappers, columns, rows, cells).
func (imp *importer) tableFromJSON(jb *jsonBlock, tableId string) (*model.Block, []*model.Block, error) {
	table := &model.Block{
		Id:      tableId,
		Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}},
	}
	var extra []*model.Block

	colsWrapper := &model.Block{
		Id:      imp.genId(),
		Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}},
	}
	rowsWrapper := &model.Block{
		Id:      imp.genId(),
		Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}},
	}
	table.ChildrenIds = []string{colsWrapper.Id, rowsWrapper.Id}

	colIds := make([]string, 0, len(jb.Columns))
	for _, jc := range jb.Columns {
		id := jc.Id
		if id == "" {
			id = imp.newTableInnerId()
		} else {
			imp.claimTableInnerId(id)
		}
		colIds = append(colIds, id)
		fields := jsonMapToProtoStruct(jc.Fields)
		if jc.Width != 0 {
			if fields == nil || fields.Fields == nil {
				fields = &types.Struct{Fields: map[string]*types.Value{}}
			}
			fields.Fields[tableWidthField] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: jc.Width}}
		}
		if len(fields.GetFields()) == 0 {
			fields = nil
		}
		col := &model.Block{
			Id:      id,
			Fields:  fields,
			Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}},
		}
		colsWrapper.ChildrenIds = append(colsWrapper.ChildrenIds, id)
		extra = append(extra, col)
	}

	// header rows first: import reorders rather than rejects (§6.1)
	rows := make([]jsonTableRow, len(jb.Rows))
	copy(rows, jb.Rows)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].IsHeader && !rows[j].IsHeader })

	for _, jr := range rows {
		rowId := jr.Id
		if rowId == "" {
			rowId = imp.newTableInnerId()
		} else {
			imp.claimTableInnerId(rowId)
		}
		row := &model.Block{
			Id:      rowId,
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: jr.IsHeader}},
		}
		if len(jr.Cells) > len(colIds) {
			return nil, nil, fmt.Errorf("table %s row %s: %d cells for %d columns", tableId, rowId, len(jr.Cells), len(colIds))
		}
		for i, cell := range jr.Cells {
			cellBlocks, err := imp.cellFromJSON(cell, rowId+"-"+colIds[i])
			if err != nil {
				return nil, nil, err
			}
			if len(cellBlocks) > 0 {
				row.ChildrenIds = append(row.ChildrenIds, cellBlocks[0].Id)
				extra = append(extra, cellBlocks...)
			}
		}
		rowsWrapper.ChildrenIds = append(rowsWrapper.ChildrenIds, rowId)
		extra = append(extra, row)
	}

	extra = append([]*model.Block{colsWrapper, rowsWrapper}, extra...)
	return table, extra, nil
}

// cellFromJSON builds a cell block (with its derived id) and, for the array
// form, its flat descendants (F10). Empty cells produce no blocks.
func (imp *importer) cellFromJSON(cell jsonCell, cellId string) ([]*model.Block, error) {
	if cell.Text != nil {
		if *cell.Text == "" {
			return nil, nil
		}
		text, marks, err := parseInline(*cell.Text)
		if err != nil {
			return nil, fmt.Errorf("cell %s: %w", cellId, err)
		}
		imp.resolveMarkTargets(marks)
		return []*model.Block{{
			Id: cellId,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:  text,
				Marks: &model.BlockContentTextMarks{Marks: marks},
			}},
		}}, nil
	}
	if len(cell.Blocks) > 0 {
		// array form: first element is the cell block, the rest its
		// descendants per the §4 F6 stack rebuild
		blocks, err := imp.blockFromJSON(cell.Blocks[0], cellId)
		if err != nil {
			return nil, err
		}
		rest := cell.Blocks[1:]
		extra, err := imp.flatSubtree(rest, imp.blockIndents(rest, 0), blocks[0], 0)
		if err != nil {
			return nil, err
		}
		return append(blocks, extra...), nil
	}
	if cell.Block == nil {
		return nil, nil
	}
	// an empty plain paragraph collapses to an empty cell (§11)
	b := cell.Block
	if b.Type == "paragraph" && b.Text == "" && b.Color == "" && !b.Checked &&
		b.Align == "" && b.VerticalAlign == "" && b.BackgroundColor == "" &&
		len(b.Fields) == 0 {
		return nil, nil
	}
	blocks, err := imp.blockFromJSON(b, cellId)
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

// claimTableInnerId reserves an authored row/column id so a generated one
// cannot collide with it. claimAuthoredIds has normally seen it already; this
// keeps the guarantee local to the caller rather than assuming that.
func (imp *importer) claimTableInnerId(id string) string {
	return imp.claimId(id)
}

// newTableInnerId mints a row or column id that is safe to build a cell id
// from. A cell's id is rowId + "-" + colId, and the whole editor recovers the
// column from it with SplitN(id, "-", 2) (table.ParseCellID, which drives
// every column insert/delete/move, HTML export and table normalization), so a
// row or column id must contain no "-" at all — hence the schema's
// [A-Za-z0-9_]{1,64} on authored ones.
//
// Generated ids have to honour the same rule, and Options.GenerateId belongs
// to the caller: the convert wiring derives ids from file paths, which are
// full of dashes. So sanitize rather than trust, and disambiguate on
// collision instead of hoping the sanitized forms stay distinct.
func (imp *importer) newTableInnerId() string {
	// genId already claimed its answer; the sanitized form is a different
	// string, so it has to be claimed on its own
	return imp.claimId(uniqueLabel(sanitizeTableInnerId(imp.genId()), imp.idTaken))
}

// maxTableInnerId mirrors the schema's tableInnerId length bound, so a
// generated id validates on re-export.
const maxTableInnerId = 64

func sanitizeTableInnerId(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "c"
	}
	if len(out) > maxTableInnerId {
		out = out[:maxTableInnerId]
	}
	return out
}

// tableInnerId renders a stored row/column id for output. Stored ids can hold
// characters the format forbids in that position (§6.1): historical data and
// any generator that derives ids from file paths both produce "-", which is
// the cell-id separator. Emitting one verbatim would make Marshal write a
// document its own Validate rejects, so normalize it once here. Only the
// label changes — the cell mapping keys off the stored id.
//
// The uniqueness domain is the whole document, not the table: a column id
// sanitized to "c_1" has to avoid a sibling paragraph already called "c_1"
// just as much as it has to avoid another column (§4).
func (e *exporter) tableInnerId(stored string) string {
	return e.idLabel(stored, sanitizeTableInnerId)
}

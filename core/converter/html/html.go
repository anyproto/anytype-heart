package html

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io/ioutil"
	"strconv"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	utf16 "github.com/anyproto/anytype-heart/util/text"
)

var log = logging.Logger("html-converter").Desugar()

func NewHTMLConverter(s *state.State, fileObjectService fileobject.Service) *HTML {
	return &HTML{
		s:                 s,
		fileObjectService: fileObjectService,
	}
}

type HTML struct {
	s                 *state.State
	buf               *bytes.Buffer
	fileObjectService fileobject.Service
}

func (h *HTML) Convert() (result string) {
	if len(h.s.Pick(h.s.RootId()).Model().ChildrenIds) == 0 {
		return ""
	}
	h.buf = bytes.NewBuffer(nil)
	h.buf.WriteString(wrapCopyStart)
	h.renderChildren(h.s.Pick(h.s.RootId()).Model())
	h.buf.WriteString(wrapCopyEnd)
	result = h.buf.String()
	h.buf.Reset()
	return
}

func (h *HTML) Export() (result string) {
	h.buf = bytes.NewBuffer(nil)
	h.buf.WriteString(wrapExportStart)
	h.renderChildren(h.s.Pick(h.s.RootId()).Model())
	h.buf.WriteString(wrapExportEnd)
	return h.buf.String()
}

func (h *HTML) render(rs *renderState, b *model.Block) {
	switch b.Content.(type) {
	case *model.BlockContentOfSmartblock:
		rs.Close()
	case *model.BlockContentOfText:
		h.renderText(rs, b)
	case *model.BlockContentOfFile:
		rs.Close()
		h.renderFile(b)
	case *model.BlockContentOfBookmark:
		rs.Close()
		h.renderBookmark(b)
	case *model.BlockContentOfDiv:
		rs.Close()
		h.renderDiv(b)
	case *model.BlockContentOfLayout:
		rs.Close()
		h.renderLayout(b)
	case *model.BlockContentOfLink:
		rs.Close()
		h.renderLink(b)
	case *model.BlockContentOfTable:
		rs.Close()
		h.renderTable(b)
	default:
		rs.Close()
		h.renderLayout(b)
	}
}

func (h *HTML) renderChildren(parent *model.Block) {
	var rs = &renderState{h: h}
	for index, chID := range parent.ChildrenIds {
		b := h.s.Pick(chID)
		if b == nil {
			continue
		}
		if index == 0 {
			rs.isFirst = true
		}
		if index == len(parent.ChildrenIds)-1 {
			rs.isLast = true
		}
		h.render(rs, b.Model())
	}
}

func (h *HTML) renderText(rs *renderState, b *model.Block) {
	text := b.GetText()
	switch text.Style {
	case model.BlockContentText_Marked:
		if rs.isFirst {
			rs.OpenUL()
		}
		h.buf.WriteString(`<li>`)
		h.writeTextToBuf(text)
		h.renderChildren(b)
		h.buf.WriteString(`</li>`)
		if rs.isLast {
			rs.Close()
		}
	case model.BlockContentText_Numbered:
		if rs.isFirst {
			rs.OpenOL()
		}
		h.buf.WriteString(`<li>`)
		h.writeTextToBuf(text)
		h.renderChildren(b)
		h.buf.WriteString(`</li>`)
		if rs.isLast {
			rs.Close()
		}
	case model.BlockContentText_Callout:
		rs.Close()

		img := ""
		if text.IconEmoji != "" {
			img = fmt.Sprintf(`<span class="callout-image">%s</span>`, text.IconEmoji)
		}

		fmt.Fprintf(h.buf, `<div style="%s">%s`, styleCallout, img)
		h.writeTextToBuf(text)
		h.renderChildren(b)
		h.buf.WriteString(`</div>`)
	default:
		tags, ok := styleTags[text.Style]
		if !ok {
			tags = styleTags[defaultStyle]
			if text.Text == "" {
				tags = styleTag{OpenTag: "<p>", CloseTag: "</p>"}
			}
		}
		rs.Close()
		h.buf.WriteString(tags.OpenTag)
		h.writeTextToBuf(text)
		h.renderChildren(b)
		h.buf.WriteString(tags.CloseTag)
	}
}

func (h *HTML) renderFile(b *model.Block) {
	file := b.GetFile()
	if file.State != model.BlockContentFile_Done {
		return
	}
	goToAnytypeMsg := `<div class="message">
		<div class="header">This content is available in Anytype.</div>
		Follow <a href="https://anytype.io">link</a> to ask a permission to get the content
	</div>`

	switch file.Type {
	case model.BlockContentFile_File:
		h.buf.WriteString(`<div class="file"><div class="name">`)
		h.buf.WriteString(html.EscapeString(file.Name))
		h.buf.WriteString(`</div>`)
		h.buf.WriteString(goToAnytypeMsg)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	case model.BlockContentFile_Image:
		baseImg, err := h.getImageBase64(context.Background(), file.TargetObjectId)
		if err != nil {
			log.Error("getImageBase64", zap.Error(err))
		}
		fmt.Fprintf(h.buf, `<div><img alt="%s" src="%s" />`, html.EscapeString(file.Name), baseImg)
		h.renderChildren(b)
		h.buf.WriteString("</div>")

	case model.BlockContentFile_Video:
		h.buf.WriteString(`<div class="video"><div class="name">`)
		h.buf.WriteString(html.EscapeString(file.Name))
		h.buf.WriteString(`</div>`)
		h.buf.WriteString(goToAnytypeMsg)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	case model.BlockContentFile_Audio:
		h.buf.WriteString(`<div class="audio"><div class="name">`)
		h.buf.WriteString(html.EscapeString(file.Name))
		h.buf.WriteString(`</div>`)
		h.buf.WriteString(goToAnytypeMsg)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	case model.BlockContentFile_PDF:
		h.buf.WriteString(`<div class="pdf"><div class="name">`)
		h.buf.WriteString(html.EscapeString(file.Name))
		h.buf.WriteString(`</div>`)
		h.buf.WriteString(goToAnytypeMsg)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	}
}

func (h *HTML) renderBookmark(b *model.Block) {
	bm := b.GetBookmark()
	if bm.Url != "" {
		fmt.Fprintf(h.buf, `<div class="bookmark"><a href="%s">%s</a><div class="description">%s</div>`, bm.Url, html.EscapeString(bm.Title), html.EscapeString(bm.Description))
	} else {
		h.buf.WriteString("<div>")
	}
	h.renderChildren(b)
	h.buf.WriteString("</div>")
}

func (h *HTML) renderDiv(b *model.Block) {
	switch b.GetDiv().Style {
	case model.BlockContentDiv_Dots:
		h.buf.WriteString(`<hr class="dots">`)
	case model.BlockContentDiv_Line:
		h.buf.WriteString(`<hr class="line">`)
	}
	h.renderChildren(b)
}

func (h *HTML) renderLayout(b *model.Block) {
	style := model.BlockContentLayoutStyle(-1)
	layout := b.GetLayout()
	if layout != nil {
		style = layout.Style
	}

	switch style {
	case model.BlockContentLayout_Column:
		style := ""
		fields := b.Fields
		if fields != nil && fields.Fields != nil && fields.Fields["width"] != nil {
			width := pbtypes.GetFloat64(fields, "width")
			if width > 0 {
				style = `style="width: ` + strconv.Itoa(int(width*100)) + `%">`
			}
		}
		h.buf.WriteString(`<div class="column" ` + style + `>`)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	case model.BlockContentLayout_Row:
		h.buf.WriteString(`<div class="row" style="display: flex">`)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	case model.BlockContentLayout_Div:
		h.renderChildren(b)
	default:
		h.buf.WriteString(`<div>`)
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	}
}

func (h *HTML) renderLink(b *model.Block) {
	if len(b.ChildrenIds) > 0 {
		h.buf.WriteString("<div>")
	}
	h.buf.WriteString(`<div class="message">
		<div class="header">This content is available in Anytype.</div>
		Follow <a href="https://anytype.io">link</a> to ask a permission to get the content
	</div>`)
	if len(b.ChildrenIds) > 0 {
		h.renderChildren(b)
		h.buf.WriteString("</div>")
	}
}

func (h *HTML) renderTable(b *model.Block) {
	tb, err := table.NewTable(h.s, b.Id)
	if err != nil {
		return
	}

	h.buf.WriteString(`<table style="border-collapse: collapse; border: 1px solid #dfddd0;">`)
	defer h.buf.WriteString("</table>")

	cols := tb.Columns()
	colWidth := map[string]float64{}
	for _, colId := range cols.ChildrenIds {
		col := h.s.Pick(colId)
		if col == nil {
			continue
		}
		colWidth[colId] = pbtypes.GetFloat64(col.Model().GetFields(), "width")
	}
	for _, rowID := range tb.RowIDs() {
		h.renderRow(rowID, cols, colWidth)
	}
}

func (h *HTML) renderRow(rowId string, cols *model.Block, colWidth map[string]float64) {
	row := h.s.Pick(rowId)
	if row == nil {
		return
	}
	h.buf.WriteString("<tr>")
	defer h.buf.WriteString("</tr>")

	colToCell := map[string]string{}
	for _, cellID := range row.Model().ChildrenIds {
		_, colID, err := table.ParseCellID(cellID)
		if err != nil {
			continue
		}
		colToCell[colID] = cellID
	}

	for _, colId := range cols.ChildrenIds {
		h.renderCell(colWidth, colId, colToCell)
	}
}

func (h *HTML) renderCell(colWidth map[string]float64, colId string, colToCell map[string]string) {
	var extraAttr, extraStyle string
	if w := colWidth[colId]; w > 0 {
		extraAttr += fmt.Sprintf(` width="%d"`, int(w))
	}

	var cell simple.Block
	cellId, ok := colToCell[colId]
	if ok {
		cell = h.s.Pick(cellId)
		if cell != nil {
			if bg := cell.Model().BackgroundColor; bg != "" {
				extraStyle += fmt.Sprintf(`; background-color: %s`, backgroundColor(bg))
			}
		}
	}

	fmt.Fprintf(h.buf, `<td style="border: 1px solid #dfddd0; padding: 9px; font-size: 14px; line-height: 22px%s"%s>`, extraStyle, extraAttr)
	defer h.buf.WriteString("</td>")

	if cell != nil {
		rs := &renderState{h: h}
		h.render(rs, cell.Model())
	} else {
		h.buf.WriteString("&nbsp;")
	}
}

func (h *HTML) writeTag(m *model.BlockContentTextMark, start bool) {
	switch m.Type {
	case model.BlockContentTextMark_Strikethrough:
		if start {
			h.buf.WriteString("<s>")
		} else {
			h.buf.WriteString("</s>")
		}
	case model.BlockContentTextMark_Keyboard:
		if start {
			h.buf.WriteString(`<kbd style="` + styleKbd + `">`)
		} else {
			h.buf.WriteString(`</kbd>`)
		}
	case model.BlockContentTextMark_Italic:
		if start {
			h.buf.WriteString("<i>")
		} else {
			h.buf.WriteString("</i>")
		}
	case model.BlockContentTextMark_Bold:
		if start {
			h.buf.WriteString("<b>")
		} else {
			h.buf.WriteString("</b>")
		}
	case model.BlockContentTextMark_Link:
		if start {
			fmt.Fprintf(h.buf, `<a href="%s">`, m.Param)
		} else {
			h.buf.WriteString("</a>")
		}
	case model.BlockContentTextMark_TextColor:
		if start {
			fmt.Fprintf(h.buf, `<span style="color:%s">`, textColor(m.Param))
		} else {
			h.buf.WriteString("</span>")
		}
	case model.BlockContentTextMark_BackgroundColor:
		if start {
			fmt.Fprintf(h.buf, `<span style="backgound-color:%s">`, backgroundColor(m.Param))
		} else {
			h.buf.WriteString("</span>")
		}
	case model.BlockContentTextMark_Underscored:
		if start {
			h.buf.WriteString("<u>")
		} else {
			h.buf.WriteString("</u>")
		}
	}
}

func (h *HTML) closeTagsUntil(
	text *model.BlockContentText,
	lastOpenedTags *[]model.BlockContentTextMarkType,
	bottom model.BlockContentTextMarkType,
	index int,
) {
	closed := 0
	for _, tag := range *lastOpenedTags {
		if tag == bottom {
			*lastOpenedTags = (*lastOpenedTags)[closed:]
			return
		}
		for _, mark := range text.Marks.Marks {
			if mark.Type == tag && int(mark.Range.From) < index && int(mark.Range.To) >= index {
				h.writeTag(mark, false)
				if int(mark.Range.To) == index {
					mark.Range.To--
				} else {
					mark.Range.From = int32(index)
				}
				break
			}
		}
		closed++
	}
}

func (h *HTML) writeTextToBuf(text *model.BlockContentText) {
	var (
		breakpoints    = make(map[int]struct{})
		lastOpenedTags = make([]model.BlockContentTextMarkType, 0)
	)
	if text.Marks != nil {
		for _, m := range text.Marks.Marks {
			breakpoints[int(m.Range.From)] = struct{}{}
			breakpoints[int(m.Range.To)] = struct{}{}
		}
	}

	textLen := utf16.UTF16RuneCountString(text.Text)
	runes := []rune(text.Text)
	// the end position of markdown text equals full length of text
	for i := 0; i <= textLen; i++ {
		if _, ok := breakpoints[i]; ok {
			// iterate marks forwards to put closing tags
			for _, m := range text.Marks.Marks {
				if int(m.Range.To) == i {
					h.closeTagsUntil(text, &lastOpenedTags, m.Type, i)
					h.writeTag(m, false)
					if len(lastOpenedTags) != 0 {
						lastOpenedTags = lastOpenedTags[1:]
					}
				}
			}
			// iterate marks backwards to put opening tags
			for j := len(text.Marks.Marks) - 1; j >= 0; j-- {
				m := text.Marks.Marks[j]
				if int(m.Range.From) == i {
					h.writeTag(m, true)
					lastOpenedTags = append([]model.BlockContentTextMarkType{m.Type}, lastOpenedTags...)
				}
			}

		}
		if i < len(runes) {
			h.buf.WriteString(html.EscapeString(string(runes[i])))
		}
	}
}

func (h *HTML) getImageBase64(ctx context.Context, fileObjectId string) (string, error) {
	im, err := h.fileObjectService.GetImageData(ctx, fileObjectId)
	if err != nil {
		return "", fmt.Errorf("get image data: %w", err)
	}
	f, err := im.GetFileForWidth(1024)
	if err != nil {
		return "", fmt.Errorf("get image variant by width: %w", err)
	}
	rd, err := f.Reader(ctx)
	if err != nil {
		return "", fmt.Errorf("get reader: %w", err)
	}
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	dataBase64 := base64.StdEncoding.EncodeToString(data)
	encoded := fmt.Sprintf("data:%s;base64, %s", f.Meta().Media, dataBase64)
	return encoded, nil
}

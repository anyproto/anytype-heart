package html

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io/ioutil"
	"strconv"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const (
	wrapCopyStart = `<html>
		<head>
			<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
			<meta http-equiv="Content-Style-Type" content="text/css">
			<title></title>
			<meta name="Generator" content="Cocoa HTML Writer">
			<meta name="CocoaVersion" content="1894.1">
			<style type="text/css">
				.row > * { display: flex; }
				.header1 { padding: 23px 0px 1px 0px; font-size: 28px; line-height: 32px; letter-spacing: -0.36px; font-weight: 600; }
				.header2 { padding: 15px 0px 1px 0px; font-size: 22px; line-height: 28px; letter-spacing: -0.16px; font-weight: 600; }
				.header3 { padding: 15px 0px 1px 0px; font-size: 17px; line-height: 24px; font-weight: 600; }
				.quote { padding: 7px 0px 7px 0px; font-size: 18px; line-height: 26px; font-style: italic; }
				.paragraph { font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word; }
				a { cursor: pointer; }
				kbd { display: inline; font-family: 'Mono'; line-height: 1.71; background: rgba(247,245,240,0.5); padding: 0px 4px; border-radius: 2px; }
			</style>
		</head>
		<body>`
	wrapCopyEnd = `</body>
	</html>`
	wrapExportStart = `
	<!DOCTYPE html>
		<html>
			<head>
				<meta http-equiv="content-type" content="text/html; charset=utf-8" />
				<title></title>
				<style type="text/css"></style>
				<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/styles/github.min.css">
				<script src="https://code.jquery.com/jquery-3.4.1.min.js"></script>
				<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/highlight.min.js"></script>
			</head>
			<body>
				<div class="anytype-container">`
	wrapExportEnd = `</div>
			</body>
		</html>`
)

func NewHTMLConverter(a anytype.Service, s *state.State) *HTML {
	return &HTML{a: a, s: s}
}

type HTML struct {
	a   anytype.Service
	s   *state.State
	buf *bytes.Buffer
}

func (h *HTML) Convert() (result string) {
	if len(h.s.Pick(h.s.RootId()).Model().ChildrenIds) == 0 {
		return ""
	}
	h.buf = bytes.NewBuffer(nil)
	h.buf.WriteString(wrapCopyStart)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model())
	h.buf.WriteString(wrapCopyEnd)
	result = h.buf.String()
	h.buf.Reset()
	return
}

func (h *HTML) Export() (result string) {
	h.buf = bytes.NewBuffer(nil)
	h.buf.WriteString(wrapExportStart)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model())
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
	default:
		rs.Close()
		h.renderLayout(b)
	}
}

func (h *HTML) renderChilds(parent *model.Block) {
	var rs = &renderState{h: h}
	for _, chId := range parent.ChildrenIds {
		b := h.s.Pick(chId)
		if b == nil {
			continue
		}
		h.render(rs, b.Model())
	}
}

func (h *HTML) renderText(rs *renderState, b *model.Block) {
	styleParagraph := "font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;"
	styleHeader1 := "padding: 23px 0px 1px 0px; font-size: 28px; line-height: 32px; letter-spacing: -0.36px; font-weight: 600;"
	styleHeader2 := "padding: 15px 0px 1px 0px; font-size: 22px; line-height: 28px; letter-spacing: -0.16px; font-weight: 600;"
	styleHeader3 := "padding: 15px 0px 1px 0px; font-size: 17px; line-height: 24px; font-weight: 600;"
	styleHeader4 := ""
	styleQuote := "padding: 7px 0px 7px 0px; font-size: 18px; line-height: 26px; font-style: italic;"
	styleCode := "font-size:15px; font-family: monospace;"
	styleTitle := ""
	styleCheckbox := "font-size:15px;"
	styleToggle := "font-size:15px;"
	styleKbd := "display: inline; font-family: 'Mono'; line-height: 1.71; background: rgba(247,245,240,0.5); padding: 0px 4px; border-radius: 2px;"

	text := b.GetText()

	writeMark := func(m *model.BlockContentTextMark, start bool) {
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
				fmt.Fprintf(h.buf, `<span style="color:%s">`, colorMapping(m.Param, true))
			} else {
				h.buf.WriteString("</span>")
			}
		case model.BlockContentTextMark_BackgroundColor:
			if start {
				fmt.Fprintf(h.buf, `<span style="backgound-color:%s">`, colorMapping(m.Param, true))
			} else {
				h.buf.WriteString("</span>")
			}
		}
	}

	renderText := func() {
		var breakpoints = make(map[int]struct{})
		if text.Marks != nil {
			for _, m := range text.Marks.Marks {
				breakpoints[int(m.Range.From)] = struct{}{}
				breakpoints[int(m.Range.To)] = struct{}{}
			}
		}

		for i, r := range []rune(text.Text) {
			if _, ok := breakpoints[i]; ok {
				for _, m := range text.Marks.Marks {
					if int(m.Range.To) == i {
						writeMark(m, false)
					}
					if int(m.Range.From) == i {
						writeMark(m, true)
					}
				}
			}
			h.buf.WriteString(html.EscapeString(string(r)))
		}
	}

	switch text.Style {
	case model.BlockContentText_Header1:
		rs.Close()
		h.buf.WriteString(`<h1 style="` + styleHeader1 + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</h1>`)
	case model.BlockContentText_Header2:
		rs.Close()
		h.buf.WriteString(`<h2 style="` + styleHeader2 + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</h2>`)
	case model.BlockContentText_Header3:
		rs.Close()
		h.buf.WriteString(`<h3 style="` + styleHeader3 + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</h3>`)
	case model.BlockContentText_Header4:
		rs.Close()
		h.buf.WriteString(`<h4 style="` + styleHeader4 + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</h4>`)
	case model.BlockContentText_Quote:
		rs.Close()
		h.buf.WriteString(`<quote style="` + styleQuote + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</quote>`)
	case model.BlockContentText_Code:
		rs.Close()
		h.buf.WriteString(`<code style="` + styleCode + `"><pre>`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</pre></code>`)
	case model.BlockContentText_Title:
		rs.Close()
		h.buf.WriteString(`<h1 style="` + styleTitle + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</h1>`)
	case model.BlockContentText_Checkbox:
		rs.Close()
		h.buf.WriteString(`<div style="` + styleCheckbox + `" class="check"><input type="checkbox"/>`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</div>`)
	case model.BlockContentText_Marked:
		rs.OpenUL()
		h.buf.WriteString(`<li>`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</li>`)
	case model.BlockContentText_Numbered:
		rs.OpenOL()
		h.buf.WriteString(`<li>`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</li>`)
	case model.BlockContentText_Toggle:
		rs.Close()
		h.buf.WriteString(`<div style="` + styleToggle + `" class="toggle">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</div>`)
	default:
		rs.Close()
		h.buf.WriteString(`<div style="` + styleParagraph + `" class="paragraph" style="` + styleParagraph + `">`)
		renderText()
		h.renderChilds(b)
		h.buf.WriteString(`</div>`)
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
		h.renderChilds(b)
		h.buf.WriteString("</div>")
	case model.BlockContentFile_Image:
		baseImg := h.getImageBase64(file.Hash)
		fmt.Fprintf(h.buf, `<div><img alt="%s" src="%s" />`, html.EscapeString(file.Name), baseImg)
		h.renderChilds(b)
		h.buf.WriteString("</div>")
	case model.BlockContentFile_Video:
		h.buf.WriteString(`<div class="video"><div class="name">`)
		h.buf.WriteString(html.EscapeString(file.Name))
		h.buf.WriteString(`</div>`)
		h.buf.WriteString(goToAnytypeMsg)
		h.renderChilds(b)
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
	h.renderChilds(b)
	h.buf.WriteString("</div>")
}

func (h *HTML) renderDiv(b *model.Block) {
	switch b.GetDiv().Style {
	case model.BlockContentDiv_Dots:
		h.buf.WriteString(`<hr class="dots">`)
	case model.BlockContentDiv_Line:
		h.buf.WriteString(`<hr class="line">`)
	}
	h.renderChilds(b)
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
		h.renderChilds(b)
		h.buf.WriteString("</div>")
	case model.BlockContentLayout_Row:
		h.buf.WriteString(`<div class="row" style="display: flex">`)
		h.renderChilds(b)
		h.buf.WriteString("</div>")
	case model.BlockContentLayout_Div:
		h.renderChilds(b)
	default:
		h.buf.WriteString(`<div>`)
		h.renderChilds(b)
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
		h.renderChilds(b)
		h.buf.WriteString("</div>")
	}
}

func (h *HTML) getImageBase64(hash string) (res string) {
	im, err := h.a.ImageByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	f, err := im.GetFileForWidth(context.TODO(), 1024)
	if err != nil {
		return
	}
	rd, err := f.Reader()
	if err != nil {
		return
	}
	data, _ := ioutil.ReadAll(rd)
	dataBase64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64, %s", f.Meta().Media, dataBase64)
}

type renderState struct {
	ulOpened, olOpened bool

	h *HTML
}

func (rs *renderState) OpenUL() {
	if rs.ulOpened {
		return
	}
	if rs.olOpened {
		rs.Close()
	}
	rs.h.buf.WriteString(`<ul style="font-size:15px;">`)
	rs.ulOpened = true
}

func (rs *renderState) OpenOL() {
	if rs.olOpened {
		return
	}
	if rs.ulOpened {
		rs.Close()
	}
	rs.h.buf.WriteString("<ol style=\"font-size:15px;\">")
	rs.olOpened = true
}

func (rs *renderState) Close() {
	if rs.ulOpened {
		rs.h.buf.WriteString("</ul>")
		rs.ulOpened = false
	} else if rs.olOpened {
		rs.h.buf.WriteString("</ol>")
		rs.olOpened = false
	}
}

func colorMapping(color string, isText bool) (out string) {
	if isText {
		switch color {
		case "grey":
			out = "#aca996"
		case "yellow":
			out = "#ecd91b"
		case "orange":
			out = "#ffb522"
		case "red":
			out = "#f55522"
		case "pink":
			out = "#e51ca0"
		case "purple":
			out = "#ab50cc"
		case "blue":
			out = "#3e58"
		case "ice":
			out = "#2aa7ee"
		case "teal":
			out = "#0fc8ba"
		case "lime":
			out = "#5dd400"
		case "black":
			out = "#2c2b27"
		default:
			out = color
		}
	} else {
		switch color {
		case "grey":
			out = "#f3f2ec"
		case "yellow":
			out = "#fef9cc"
		case "orange":
			out = "#fef3c5"
		case "red":
			out = "#ffebe5"
		case "pink":
			out = "#fee3f5"
		case "purple":
			out = "#f4e3fa"
		case "blue":
			out = "#f4e3fa"
		case "ice":
			out = "#d6effd"
		case "teal":
			out = "#d6f5f3"
		case "lime":
			out = "#e3f7d0"
		default:
			out = color
		}
	}

	return out
}

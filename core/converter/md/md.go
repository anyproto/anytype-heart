package md

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/escape"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewMDConverter(a anytype.Service, s *state.State) *MD {
	return &MD{a: a, s: s}
}

type MD struct {
	a   anytype.Service
	s   *state.State
	buf *bytes.Buffer

	fileHashes  []string
	imageHashes []string
}

func (h *MD) Convert() (result string) {
	if len(h.s.Pick(h.s.RootId()).Model().ChildrenIds) == 0 {
		return ""
	}
	h.buf = bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model(), in)
	result = h.buf.String()
	h.buf.Reset()
	return
}

func (h *MD) Export() (result string) {
	h.buf = bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model(), in)
	return h.buf.String()
}

type renderState struct {
	indent     string
	listOpened bool
}

func (in renderState) AddNBSpace() *renderState {
	return &renderState{indent: in.indent + "  "}
}

func (in renderState) AddSpace() *renderState {
	return &renderState{indent: in.indent + "  "}
}

func (h *MD) render(b *model.Block, in *renderState) {
	switch b.Content.(type) {
	case *model.BlockContentOfSmartblock:
	case *model.BlockContentOfText:
		h.renderText(b, in)
	case *model.BlockContentOfFile:
		h.renderFile(b, in)
	case *model.BlockContentOfBookmark:
		h.renderBookmark(b, in)
	case *model.BlockContentOfDiv:
		h.renderDiv(b, in)
	case *model.BlockContentOfLayout:
		h.renderLayout(b, in)
	case *model.BlockContentOfLink:
		h.renderLink(b, in)
	default:
		h.renderLayout(b, in)
	}
}

func (h *MD) renderChilds(parent *model.Block, in *renderState) {
	for _, chId := range parent.ChildrenIds {
		b := h.s.Pick(chId)
		if b == nil {
			continue
		}
		h.render(b.Model(), in)
	}
}

func (h *MD) renderText(b *model.Block, in *renderState) {
	text := b.GetText()
	writeMark := func(m *model.BlockContentTextMark, start bool) {
		switch m.Type {
		case model.BlockContentTextMark_Strikethrough:
			h.buf.WriteString("~~")
		case model.BlockContentTextMark_Italic:
			h.buf.WriteString("*")
		case model.BlockContentTextMark_Bold:
			h.buf.WriteString("**")
		case model.BlockContentTextMark_Link:
			if start {
				h.buf.WriteString("[")
			} else {
				fmt.Fprintf(h.buf, "](%s)", m.Param)
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
		var (
			i int
			r rune
		)

		for i, r = range []rune(text.Text) {
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
			h.buf.WriteString(escape.MarkdownCharacters(html.EscapeString(string(r))))
		}
		i++
		for _, m := range text.Marks.Marks {
			if int(m.Range.To) == i {
				writeMark(m, false)
			}
			if int(m.Range.From) == i {
				writeMark(m, true)
			}
		}
		h.buf.WriteString("   \n")
	}
	if in.listOpened && text.Style != model.BlockContentText_Marked && text.Style != model.BlockContentText_Numbered {
		h.buf.WriteString("   \n")
		in.listOpened = false
	}

	h.buf.WriteString(in.indent)

	switch text.Style {
	case model.BlockContentText_Header1, model.BlockContentText_Title:
		h.buf.WriteString(` # `)
		renderText()
		h.renderChilds(b, in.AddSpace())
	case model.BlockContentText_Header2:
		h.buf.WriteString(` ## `)
		renderText()
		h.renderChilds(b, in.AddSpace())
	case model.BlockContentText_Header3:
		h.buf.WriteString(` ### `)
		renderText()
		h.renderChilds(b, in.AddSpace())
	case model.BlockContentText_Header4:
		h.buf.WriteString(` #### `)
		renderText()
		h.renderChilds(b, in.AddSpace())
	case model.BlockContentText_Quote, model.BlockContentText_Toggle:

	case model.BlockContentText_Code:
		h.buf.WriteString("```\n")
		h.buf.WriteString(strings.ReplaceAll(text.Text, "```", "\\`\\`\\`"))
		h.buf.WriteString("```\n")
		h.renderChilds(b, in)
	case model.BlockContentText_Checkbox:
		if text.Checked {
			h.buf.WriteString("[x] ")
		} else {
			h.buf.WriteString("[ ] ")
		}
		renderText()
		h.renderChilds(b, in.AddNBSpace())
	case model.BlockContentText_Marked:
		h.buf.WriteString(`- `)
		renderText()
		h.renderChilds(b, in.AddSpace())
		in.listOpened = true
	case model.BlockContentText_Numbered:
		h.buf.WriteString(`1. `)
		renderText()
		h.renderChilds(b, in.AddSpace())
		in.listOpened = true
	default:
		renderText()
		h.renderChilds(b, in.AddNBSpace())
	}
}

func (h *MD) renderFile(b *model.Block, in *renderState) {
	file := b.GetFile()
	if file == nil || file.State != model.BlockContentFile_Done {
		return
	}
	name := escape.MarkdownCharacters(html.EscapeString(file.Name))
	h.buf.WriteString(in.indent)
	if file.Type == model.BlockContentFile_Image {
		fmt.Fprintf(h.buf, "![%s](files/%s)    \n", name, file.Hash+"_"+name)
		h.fileHashes = append(h.fileHashes, file.Hash)
	} else {
		fmt.Fprintf(h.buf, "[%s](files/%s)    \n", name, file.Hash+"_"+file.Name)
		h.imageHashes = append(h.imageHashes, file.Hash)
	}
}

func (h *MD) renderBookmark(b *model.Block, in *renderState) {
	bm := b.GetBookmark()
	if bm != nil && bm.Url != "" {
		h.buf.WriteString(in.indent)
		fmt.Fprintf(h.buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(bm.Title)), bm.Url)
	}
}

func (h *MD) renderDiv(b *model.Block, in *renderState) {
	switch b.GetDiv().Style {
	case model.BlockContentDiv_Dots, model.BlockContentDiv_Line:
		h.buf.WriteString(" --- \n")
	}
	h.renderChilds(b, in)
}

func (h *MD) renderLayout(b *model.Block, in *renderState) {
	style := model.BlockContentLayoutStyle(-1)
	layout := b.GetLayout()
	if layout != nil {
		style = layout.Style
	}

	switch style {
	default:
		h.renderChilds(b, in)
	}
}

func (h *MD) renderLink(b *model.Block, in *renderState) {
	l := b.GetLink()
	h.a.ObjectStore().GetByIDs()
	if l != nil && l.TargetBlockId != "" {
		var title string
		ois, err := h.a.ObjectStore().GetByIDs(l.TargetBlockId)
		if err == nil && len(ois) > 0 {
			title = pbtypes.GetString(ois[0].Details, "name")
		}
		if title == "" {
			title = l.TargetBlockId
		}
		h.buf.WriteString(in.indent)
		fmt.Fprintf(h.buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(title)), l.TargetBlockId+".md")
	}
}

func (h *MD) FileHashes() []string {
	return h.fileHashes
}

func (h *MD) ImageHashes() []string {
	return h.imageHashes
}

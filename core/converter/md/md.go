package md

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/gogo/protobuf/types"

	"github.com/JohannesKaufmann/html-to-markdown/escape"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type FileNamer interface {
	Get(path, hash, title, ext string) (name string)
}

func NewMDConverter(a core.Service, s *state.State, fn FileNamer) converter.Converter {
	return &MD{a: a, s: s, fn: fn}
}

type MD struct {
	a core.Service
	s *state.State

	fileHashes  []string
	imageHashes []string

	knownDocs map[string]*types.Struct

	mw *marksWriter
	fn FileNamer
}

func (h *MD) Convert() (result []byte) {
	if len(h.s.Pick(h.s.RootId()).Model().ChildrenIds) == 0 {
		return
	}
	buf := bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(buf, h.s.Pick(h.s.RootId()).Model(), in)
	result = buf.Bytes()
	buf.Reset()
	return
}

func (h *MD) Export() (result string) {
	buf := bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(buf, h.s.Pick(h.s.RootId()).Model(), in)
	return buf.String()
}

func (h *MD) Ext() string {
	return ".md"
}

type renderState struct {
	indent     string
	listOpened bool
	listNumber int
}

func (in renderState) AddNBSpace() *renderState {
	return &renderState{indent: in.indent + "  "}
}

func (in renderState) AddSpace() *renderState {
	return &renderState{indent: in.indent + "    "}
}

type writer interface {
	io.Writer
	io.StringWriter
}

func (h *MD) render(buf writer, b *model.Block, in *renderState) {
	switch b.Content.(type) {
	case *model.BlockContentOfSmartblock:
	case *model.BlockContentOfText:
		h.renderText(buf, b, in)
	case *model.BlockContentOfFile:
		h.renderFile(buf, b, in)
	case *model.BlockContentOfBookmark:
		h.renderBookmark(buf, b, in)
	case *model.BlockContentOfDiv:
		h.renderDiv(buf, b, in)
	case *model.BlockContentOfLayout:
		h.renderLayout(buf, b, in)
	case *model.BlockContentOfLink:
		h.renderLink(buf, b, in)
	case *model.BlockContentOfLatex:
		h.renderLatex(buf, b, in)
	default:
		h.renderLayout(buf, b, in)
	}
}

func (h *MD) renderChilds(buf writer, parent *model.Block, in *renderState) {
	for _, chId := range parent.ChildrenIds {
		b := h.s.Pick(chId)
		if b == nil {
			continue
		}
		h.render(buf, b.Model(), in)
	}
}

func (h *MD) renderText(buf writer, b *model.Block, in *renderState) {
	text := b.GetText()
	renderText := func() {
		mw := h.marksWriter(text)
		var (
			i int
			r rune
		)

		for i, r = range []rune(text.Text) {
			mw.writeMarks(buf, i)
			buf.WriteString(escape.MarkdownCharacters(string(r)))
		}
		mw.writeMarks(buf, i+1)
		buf.WriteString("   \n")
	}
	if in.listOpened && text.Style != model.BlockContentText_Marked && text.Style != model.BlockContentText_Numbered {
		buf.WriteString("   \n")
		in.listOpened = false
		in.listNumber = 0
	}

	buf.WriteString(in.indent)

	switch text.Style {
	case model.BlockContentText_Header1, model.BlockContentText_Title:
		buf.WriteString(` # `)
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
	case model.BlockContentText_Header2:
		buf.WriteString(` ## `)
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
	case model.BlockContentText_Header3:
		buf.WriteString(` ### `)
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
	case model.BlockContentText_Header4:
		buf.WriteString(` #### `)
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
	case model.BlockContentText_Quote, model.BlockContentText_Toggle:
		buf.WriteString("> ")
		buf.WriteString(strings.ReplaceAll(text.Text, "\n", "   \n> "))
		buf.WriteString("   \n\n")
		h.renderChilds(buf, b, in)
	case model.BlockContentText_Code:
		buf.WriteString("```\n")
		buf.WriteString(strings.ReplaceAll(text.Text, "```", "\\`\\`\\`"))
		buf.WriteString("\n```\n")
		h.renderChilds(buf, b, in)
	case model.BlockContentText_Checkbox:
		if text.Checked {
			buf.WriteString("- [x] ")
		} else {
			buf.WriteString("- [ ] ")
		}
		renderText()
		h.renderChilds(buf, b, in.AddNBSpace())
	case model.BlockContentText_Marked:
		buf.WriteString(`- `)
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
		in.listOpened = true
	case model.BlockContentText_Numbered:
		in.listNumber++
		buf.WriteString(fmt.Sprintf(`%d. `, in.listNumber))
		renderText()
		h.renderChilds(buf, b, in.AddSpace())
		in.listOpened = true
	default:
		renderText()
		h.renderChilds(buf, b, in.AddNBSpace())
	}
}

func (h *MD) renderFile(buf writer, b *model.Block, in *renderState) {
	file := b.GetFile()
	if file == nil || file.State != model.BlockContentFile_Done {
		return
	}
	name := escape.MarkdownCharacters(html.EscapeString(file.Name))
	buf.WriteString(in.indent)
	if file.Type != model.BlockContentFile_Image {
		fmt.Fprintf(buf, "[%s](%s)    \n", name, h.fn.Get("files", file.Hash, filepath.Base(file.Name), filepath.Ext(file.Name)))
		h.fileHashes = append(h.fileHashes, file.Hash)
	} else {
		fmt.Fprintf(buf, "![%s](%s)    \n", name, h.fn.Get("files", file.Hash, filepath.Base(file.Name), filepath.Ext(file.Name)))
		h.imageHashes = append(h.imageHashes, file.Hash)
	}
}

func (h *MD) renderBookmark(buf writer, b *model.Block, in *renderState) {
	bm := b.GetBookmark()
	if bm != nil && bm.Url != "" {
		buf.WriteString(in.indent)
		url, e := url.Parse(bm.Url)
		if e == nil {
			fmt.Fprintf(buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(bm.Title)), url.String())
		}
	}
}

func (h *MD) renderDiv(buf writer, b *model.Block, in *renderState) {
	switch b.GetDiv().Style {
	case model.BlockContentDiv_Dots, model.BlockContentDiv_Line:
		buf.WriteString(" --- \n")
	}
	h.renderChilds(buf, b, in)
}

func (h *MD) renderLayout(buf writer, b *model.Block, in *renderState) {
	style := model.BlockContentLayoutStyle(-1)
	layout := b.GetLayout()
	if layout != nil {
		style = layout.Style
	}

	switch style {
	default:
		h.renderChilds(buf, b, in)
	}
}

func (h *MD) renderLink(buf writer, b *model.Block, in *renderState) {
	l := b.GetLink()
	if l != nil && l.TargetBlockId != "" {
		title, filename, ok := h.getLinkInfo(l.TargetBlockId)
		if ok {
			buf.WriteString(in.indent)
			fmt.Fprintf(buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(title)), filename)
		}
	}
}

func (h *MD) renderLatex(buf writer, b *model.Block, in *renderState) {
	l := b.GetLatex()
	if l != nil {
		buf.WriteString(in.indent)
		fmt.Fprintf(buf, "\n$$\n%s\n$$\n", l.Text)
	}
}

func (h *MD) FileHashes() []string {
	return h.fileHashes
}

func (h *MD) ImageHashes() []string {
	return h.imageHashes
}

func (h *MD) marksWriter(text *model.BlockContentText) *marksWriter {
	if h.mw == nil {
		h.mw = &marksWriter{
			h: h,
		}
	}
	return h.mw.Init(text)
}

func (h *MD) SetKnownDocs(docs map[string]*types.Struct) converter.Converter {
	h.knownDocs = docs
	return h
}

func (h *MD) getLinkInfo(docId string) (title, filename string, ok bool) {
	info, ok := h.knownDocs[docId]
	if !ok {
		return
	}
	title = pbtypes.GetString(info, bundle.RelationKeyName.String())
	if title == "" {
		title = pbtypes.GetString(info, bundle.RelationKeySnippet.String())
	}
	if title == "" {
		title = docId
	}
	filename = h.fn.Get("", docId, title, h.Ext())
	return
}

type marksWriter struct {
	h           *MD
	breakpoints map[int]struct {
		starts []*model.BlockContentTextMark
		ends   []*model.BlockContentTextMark
	}
	open []*model.BlockContentTextMark
}

func (mw *marksWriter) writeMarks(buf writer, pos int) {
	writeMark := func(m *model.BlockContentTextMark, start bool) {
		switch m.Type {
		case model.BlockContentTextMark_Strikethrough:
			buf.WriteString("~~")
		case model.BlockContentTextMark_Italic:
			buf.WriteString("*")
		case model.BlockContentTextMark_Bold:
			buf.WriteString("**")
		case model.BlockContentTextMark_Link:
			if start {
				buf.WriteString("[")
			} else {
				urlP, e := url.Parse(m.Param)
				urlS := m.Param
				if e == nil {
					urlS = urlP.String()
				}
				fmt.Fprintf(buf, "](%s)", urlS)
			}
		case model.BlockContentTextMark_Mention, model.BlockContentTextMark_Object:
			_, filename, ok := mw.h.getLinkInfo(m.Param)
			if ok {
				if start {
					buf.WriteString("[")
				} else {
					fmt.Fprintf(buf, "](%s)", filename)
				}
			}
		case model.BlockContentTextMark_Keyboard:
			buf.WriteString("`")
		}
	}

	if mw.breakpoints == nil {
		return
	}
	if marks, ok := mw.breakpoints[pos]; ok {
		var (
			hasClosedLink bool
			hasStartLink  bool
		)
		for i := len(marks.ends) - 1; i >= 0; i-- {
			if len(mw.open) > 0 {
				if mw.open[len(mw.open)-1] != marks.ends[i] {
					marks.ends = append(marks.ends, mw.open[len(mw.open)-1])
					marks.starts = append(marks.starts, mw.open[len(mw.open)-1])
					mw.breakpoints[pos] = marks
					mw.writeMarks(buf, pos)
					return
				} else {
					mw.open = mw.open[:len(mw.open)-1]
				}
			}
			writeMark(marks.ends[i], false)
			if !hasClosedLink && marks.ends[i].Type == model.BlockContentTextMark_Link {
				hasClosedLink = true
			}
		}
		for _, m := range marks.starts {
			if m.Type == model.BlockContentTextMark_Link {
				hasStartLink = true
				break
			}
		}
		if hasStartLink && hasClosedLink {
			buf.WriteString(" ")
		}
		for _, m := range marks.starts {
			writeMark(m, true)
			mw.open = append(mw.open, m)
		}
	}
}

func (mw *marksWriter) Init(text *model.BlockContentText) *marksWriter {
	mw.open = mw.open[:0]
	if text.Marks != nil && len(text.Marks.Marks) > 0 {
		mw.breakpoints = make(map[int]struct {
			starts []*model.BlockContentTextMark
			ends   []*model.BlockContentTextMark
		})
		for _, mark := range text.Marks.Marks {
			if mark.Range != nil && mark.Range.From != mark.Range.To {
				from := mw.breakpoints[int(mark.Range.From)]
				from.starts = append(from.starts, mark)
				mw.breakpoints[int(mark.Range.From)] = from
				to := mw.breakpoints[int(mark.Range.To)]
				to.ends = append(to.ends, mark)
				mw.breakpoints[int(mark.Range.To)] = to
			}
		}
		for _, marks := range mw.breakpoints {
			sort.Sort(sortedMarks(marks.starts))
			sort.Sort(sortedMarks(marks.ends))
		}
	} else {
		mw.breakpoints = nil
	}
	return mw
}

type sortedMarks []*model.BlockContentTextMark

func (a sortedMarks) Len() int      { return len(a) }
func (a sortedMarks) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortedMarks) Less(i, j int) bool {
	li := a[i].Range.To - a[i].Range.From
	lj := a[j].Range.To - a[j].Range.From
	if li == lj {
		return a[i].Type < a[j].Type
	}
	return li > lj
}

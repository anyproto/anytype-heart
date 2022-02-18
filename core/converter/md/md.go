package md

import (
	"bytes"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/gogo/protobuf/types"
	"html"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

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
	a   core.Service
	s   *state.State
	buf *bytes.Buffer

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
	h.buf = bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model(), in)
	result = h.buf.Bytes()
	h.buf.Reset()
	return
}

func (h *MD) Export() (result string) {
	h.buf = bytes.NewBuffer(nil)
	var in = new(renderState)
	h.renderChilds(h.s.Pick(h.s.RootId()).Model(), in)
	return h.buf.String()
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
	renderText := func() {
		mw := h.marksWriter(text)
		var (
			i int
			r rune
		)

		for i, r = range []rune(text.Text) {
			mw.writeMarks(i)
			h.buf.WriteString(escape.MarkdownCharacters(html.EscapeString(string(r))))
		}
		mw.writeMarks(i + 1)
		h.buf.WriteString("   \n")
	}
	if in.listOpened && text.Style != model.BlockContentText_Marked && text.Style != model.BlockContentText_Numbered {
		h.buf.WriteString("   \n")
		in.listOpened = false
		in.listNumber = 0
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
		h.buf.WriteString("> ")
		h.buf.WriteString(strings.ReplaceAll(text.Text, "\n", "   \n> "))
		h.buf.WriteString("   \n\n")
		h.renderChilds(b, in)
	case model.BlockContentText_Code:
		h.buf.WriteString("```\n")
		h.buf.WriteString(strings.ReplaceAll(text.Text, "```", "\\`\\`\\`"))
		h.buf.WriteString("\n```\n")
		h.renderChilds(b, in)
	case model.BlockContentText_Checkbox:
		if text.Checked {
			h.buf.WriteString("- [x] ")
		} else {
			h.buf.WriteString("- [ ] ")
		}
		renderText()
		h.renderChilds(b, in.AddNBSpace())
	case model.BlockContentText_Marked:
		h.buf.WriteString(`- `)
		renderText()
		h.renderChilds(b, in.AddSpace())
		in.listOpened = true
	case model.BlockContentText_Numbered:
		in.listNumber++
		h.buf.WriteString(fmt.Sprintf(`%d. `, in.listNumber))
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
	if file.Type != model.BlockContentFile_Image {
		fmt.Fprintf(h.buf, "[%s](%s)    \n", name, h.fn.Get("files", file.Hash, filepath.Base(file.Name), filepath.Ext(file.Name)))
		h.fileHashes = append(h.fileHashes, file.Hash)
	} else {
		fmt.Fprintf(h.buf, "![%s](%s)    \n", name, h.fn.Get("files", file.Hash, filepath.Base(file.Name), filepath.Ext(file.Name)))
		h.imageHashes = append(h.imageHashes, file.Hash)
	}
}

func (h *MD) renderBookmark(b *model.Block, in *renderState) {
	bm := b.GetBookmark()
	if bm != nil && bm.Url != "" {
		h.buf.WriteString(in.indent)
		url, e := url.Parse(bm.Url)
		if e == nil {
			fmt.Fprintf(h.buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(bm.Title)), url.String())
		}
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
	if l != nil && l.TargetBlockId != "" {
		title, filename, ok := h.getLinkInfo(l.TargetBlockId)
		if ok {
			h.buf.WriteString(in.indent)
			fmt.Fprintf(h.buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(title)), filename)
		}
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

func (mw *marksWriter) writeMarks(pos int) {
	writeMark := func(m *model.BlockContentTextMark, start bool) {
		switch m.Type {
		case model.BlockContentTextMark_Strikethrough:
			mw.h.buf.WriteString("~~")
		case model.BlockContentTextMark_Italic:
			mw.h.buf.WriteString("*")
		case model.BlockContentTextMark_Bold:
			mw.h.buf.WriteString("**")
		case model.BlockContentTextMark_Link:
			if start {
				mw.h.buf.WriteString("[")
			} else {
				urlP, e := url.Parse(m.Param)
				urlS := m.Param
				if e == nil {
					urlS = urlP.String()
				}
				fmt.Fprintf(mw.h.buf, "](%s)", urlS)
			}
		case model.BlockContentTextMark_Mention, model.BlockContentTextMark_Object:
			_, filename, ok := mw.h.getLinkInfo(m.Param)
			if ok {
				if start {
					mw.h.buf.WriteString("[")
				} else {
					fmt.Fprintf(mw.h.buf, "](%s)", filename)
				}
			}
		case model.BlockContentTextMark_Keyboard:
			mw.h.buf.WriteString("`")
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
					mw.writeMarks(pos)
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
			mw.h.buf.WriteString(" ")
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

package md

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/escape"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("md-export")

type FileNamer interface {
	Get(path, hash, title, ext string) (name string)
}

func NewMDConverter(s *state.State, fn FileNamer, includeRelations bool) converter.Converter {
	return &MD{s: s, fn: fn, includeRelations: includeRelations, knownDocs: make(map[string]*domain.Details)}
}

func NewMDConverterWithSchema(s *state.State, fn FileNamer, includeRelations bool, includeSchema bool) converter.Converter {
	return &MD{s: s, fn: fn, includeRelations: includeRelations, includeSchema: true, knownDocs: make(map[string]*domain.Details)}
}

type MD struct {
	s *state.State

	fileHashes  []string
	imageHashes []string

	knownDocs map[string]*domain.Details

	includeRelations bool
	includeSchema    bool
	mw               *marksWriter
	fn               FileNamer
}

func (h *MD) Convert(sbType model.SmartBlockType) (result []byte) {
	if h.s.Pick(h.s.RootId()) == nil {
		return
	}
	if len(h.s.Pick(h.s.RootId()).Model().ChildrenIds) == 0 {
		return
	}
	switch sbType {
	case model.SmartBlockType_STType, model.SmartBlockType_STRelation, model.SmartBlockType_STRelationOption:
		return nil
	}
	buf := bytes.NewBuffer(nil)
	in := new(renderState)
	h.renderProperties(buf)
	h.renderChildren(buf, in, h.s.Pick(h.s.RootId()).Model())
	result = buf.Bytes()
	buf.Reset()
	return
}

func (h *MD) renderProperties(buf writer) {
	if !h.includeRelations {
		return
	}
	var propertiesIds []string

	// get type property id, because it can be omitted from recommended relations of the type
	for id, d := range h.knownDocs {
		if d.GetString(bundle.RelationKeyRelationKey) == bundle.RelationKeyType.String() &&
			d.GetInt64(bundle.RelationKeyLayout) == int64(model.ObjectType_relation) {
			propertiesIds = append(propertiesIds, id)
			break
		}
	}

	objectTypeId := h.s.LocalDetails().GetString(bundle.RelationKeyType)
	var objectTypeDetails *domain.Details
	if d, exists := h.knownDocs[objectTypeId]; exists {
		objectTypeDetails = d
		propertiesIds = append(d.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations), d.GetStringList(bundle.RelationKeyRecommendedRelations)...)
	}

	propertiesIds = slices.Compact(propertiesIds)
	if len(propertiesIds) > 0 {
		fmt.Fprintf(buf, "---\n")

		// Add JSON schema reference if enabled
		if h.includeSchema && objectTypeDetails != nil {
			typeName := objectTypeDetails.GetString(bundle.RelationKeyName)
			if typeName != "" {
				schemaFileName := h.GenerateSchemaFileName(typeName)
				fmt.Fprintf(buf, "# yaml-language-server: $schema=%s\n", schemaFileName)
			}
		}
	}
	for _, id := range propertiesIds {
		var (
			name        string
			key         string
			format      model.RelationFormat
			includeTime bool
		)

		if d, ok := h.knownDocs[id]; ok {
			if d.GetBool(bundle.RelationKeyIsHidden) {
				continue
			}
			name = d.GetString(bundle.RelationKeyName)
			key = d.GetString(bundle.RelationKeyRelationKey)
			format = model.RelationFormat(d.GetInt64(bundle.RelationKeyRelationFormat))
			includeTime = d.GetBool(bundle.RelationKeyRelationFormatIncludeTime)
		} else {
			continue
		} // Resolve relation metadata, skipping hidden ones.

		v := h.s.CombinedDetails().Get(domain.RelationKey(key))
		switch format {
		case model.RelationFormat_file:
			// Handle file format relations
			var ids []string
			if v.String() != "" {
				ids = append(ids, v.String())
			} else {
				ids = v.StringList()
			}
			if len(ids) == 0 {
				continue
			}

			if len(ids) == 1 {
				// Single file - check if it's a file object in knownDocs
				fileId := ids[0]
				if info, exists := h.knownDocs[fileId]; exists {
					layout := info.GetInt64(bundle.RelationKeyLayout)
					// Check if it's a file object type
					if layout == int64(model.ObjectType_file) || layout == int64(model.ObjectType_image) ||
						layout == int64(model.ObjectType_audio) || layout == int64(model.ObjectType_video) ||
						layout == int64(model.ObjectType_pdf) {
						// Get the file path using the same logic as getLinkInfo
						_, filename, _ := h.getLinkInfo(fileId)
						if filename != "" {
							_, _ = fmt.Fprintf(buf, "  %s: %s\n", name, filename)
							// Add to appropriate hash list for later use
							if layout == int64(model.ObjectType_image) {
								h.imageHashes = append(h.imageHashes, fileId)
							} else {
								h.fileHashes = append(h.fileHashes, fileId)
							}
						}
					}
				}
				continue
			}

			// Multiple files
			var fileList []string
			for _, fileId := range ids {
				if info, exists := h.knownDocs[fileId]; exists {
					layout := info.GetInt64(bundle.RelationKeyLayout)
					// Check if it's a file object type
					if layout == int64(model.ObjectType_file) || layout == int64(model.ObjectType_image) ||
						layout == int64(model.ObjectType_audio) || layout == int64(model.ObjectType_video) ||
						layout == int64(model.ObjectType_pdf) {
						_, filename, _ := h.getLinkInfo(fileId)
						if filename != "" {
							fileList = append(fileList, filename)
							// Add to appropriate hash list for later use
							if layout == int64(model.ObjectType_image) {
								h.imageHashes = append(h.imageHashes, fileId)
							} else {
								h.fileHashes = append(h.fileHashes, fileId)
							}
						}
					}
				}
			}

			// Only render if we found any files
			if len(fileList) > 0 {
				_, _ = fmt.Fprintf(buf, "  %s:\n", name)
				for _, filename := range fileList {
					_, _ = fmt.Fprintf(buf, "    - %s\n", filename)
				}
			}

		case model.RelationFormat_object:
			// Object format - render with File property when included in export
			var ids []string
			if v.String() != "" {
				ids = append(ids, v.String())
			} else {
				ids = v.StringList()
			}
			if len(ids) == 0 {
				continue
			}

			if len(ids) == 1 {
				if d, ok := h.knownDocs[ids[0]]; ok {
					// Object is included in export, render with File property
					objectName := d.Get(bundle.RelationKeyName).String()
					filename := h.fn.Get("", ids[0], objectName, h.Ext())
					_, _ = fmt.Fprintf(buf, "  %s:\n", name)
					_, _ = fmt.Fprintf(buf, "    Name: %s\n", objectName)
					_, _ = fmt.Fprintf(buf, "    File: %s\n", filename)
				} else {
					// Object not included in export, just show ID or empty
					_, _ = fmt.Fprintf(buf, "  %s: %s\n", name, ids[0])
				}
				continue
			}
			// Each target rendered as list item.
			_, _ = fmt.Fprintf(buf, "  %s:\n", name)
			for _, id := range ids {
				if d, ok := h.knownDocs[id]; ok {
					// Object is included in export
					objectName := d.Get(bundle.RelationKeyName).String()
					filename := h.fn.Get("", id, objectName, h.Ext())
					_, _ = fmt.Fprintf(buf, "    - Name: %s\n", objectName)
					_, _ = fmt.Fprintf(buf, "      File: %s\n", filename)
				}
			}

		case model.RelationFormat_tag,
			model.RelationFormat_status:
			// Tag and status formats - just render names
			var ids []string
			if v.String() != "" {
				ids = append(ids, v.String())
			} else {
				ids = v.StringList()
			}
			if len(ids) == 0 {
				continue
			}

			if len(ids) == 1 {
				if d, ok := h.knownDocs[ids[0]]; ok {
					_, _ = fmt.Fprintf(buf, "  %s: %s\n", name, d.Get(bundle.RelationKeyName).String())
				}
				continue
			}
			// Each target rendered as list item.
			_, _ = fmt.Fprintf(buf, "  %s:\n", name)
			for _, id := range ids {
				if d, ok := h.knownDocs[id]; ok {
					label := d.Get(bundle.RelationKeyName).String()
					_, _ = fmt.Fprintf(buf, "    - %s\n", label)
				}
			}

		case model.RelationFormat_date:

			if ts := v.Int64(); ts > 0 {
				var timeString string
				if includeTime {
					timeString = time.Unix(ts, 0).Format(time.RFC3339)
				} else {
					timeString = time.Unix(ts, 0).Format("2006-01-02")
				}
				_, _ = fmt.Fprintf(buf, "  %s: %s\n", name, timeString)
			}

		case model.RelationFormat_number:
			_, _ = fmt.Fprintf(buf, "  %s: %v\n", name, v.Float64())

		case model.RelationFormat_checkbox:
			// Represent checkboxes as plain booleans.
			_, _ = fmt.Fprintf(buf, "  %s: %t\n", name, v.Bool())

		default:
			if s := v.String(); s != "" {
				_, _ = fmt.Fprintf(buf, "  %s: %s\n", name, s)
			}
		}
	}
	if len(propertiesIds) > 0 {
		fmt.Fprintf(buf, "---\n\n")
	}
}

func (h *MD) Export() (result string) {
	buf := bytes.NewBuffer(nil)
	in := new(renderState)
	h.renderProperties(buf)

	h.renderChildren(buf, in, h.s.Pick(h.s.RootId()).Model())
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
	WriteRune(r rune) (n int, err error)
}

func (h *MD) render(buf writer, in *renderState, b *model.Block) {
	switch b.Content.(type) {
	case *model.BlockContentOfSmartblock:
	case *model.BlockContentOfText:
		h.renderText(buf, in, b)
	case *model.BlockContentOfFile:
		h.renderFile(buf, in, b)
	case *model.BlockContentOfBookmark:
		h.renderBookmark(buf, in, b)
	case *model.BlockContentOfDiv:
		h.renderDiv(buf, in, b)
	case *model.BlockContentOfLayout:
		h.renderLayout(buf, in, b)
	case *model.BlockContentOfLink:
		h.renderLink(buf, in, b)
	case *model.BlockContentOfLatex:
		h.renderLatex(buf, in, b)
	case *model.BlockContentOfTable:
		h.renderTable(buf, in, b)
	default:
		h.renderLayout(buf, in, b)
	}
}

func (h *MD) renderChildren(buf writer, in *renderState, parent *model.Block) {
	for _, chId := range parent.ChildrenIds {
		b := h.s.Pick(chId)
		if b == nil {
			continue
		}
		h.render(buf, in, b.Model())
	}
}

func (h *MD) renderText(buf writer, in *renderState, b *model.Block) {
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
		_, err := buf.WriteString(`# `)
		if err != nil {
			log.Warnf("failed to export header1 in markdown: %v", err)
		}
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
	case model.BlockContentText_Header2:
		_, err := buf.WriteString(`## `)
		if err != nil {
			log.Warnf("failed to export header2 in markdown: %v", err)
		}
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
	case model.BlockContentText_Header3:
		_, err := buf.WriteString(`### `)
		if err != nil {
			log.Warnf("failed to export header3 in markdown: %v", err)
		}
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
	case model.BlockContentText_Header4:
		_, err := buf.WriteString(`#### `)
		if err != nil {
			log.Warnf("failed to export header4 in markdown: %v", err)
		}
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
	case model.BlockContentText_Quote, model.BlockContentText_Toggle:
		buf.WriteString("> ")
		buf.WriteString(strings.ReplaceAll(text.Text, "\n", "   \n> "))
		buf.WriteString("   \n\n")
		h.renderChildren(buf, in, b)
	case model.BlockContentText_Code:
		buf.WriteString("```\n")
		buf.WriteString(strings.ReplaceAll(text.Text, "```", "\\`\\`\\`"))
		buf.WriteString("\n```\n")
		h.renderChildren(buf, in, b)
	case model.BlockContentText_Checkbox:
		if text.Checked {
			buf.WriteString("- [x] ")
		} else {
			buf.WriteString("- [ ] ")
		}
		renderText()
		h.renderChildren(buf, in.AddNBSpace(), b)
	case model.BlockContentText_Marked:
		buf.WriteString(`- `)
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
		in.listOpened = true
	case model.BlockContentText_Numbered:
		in.listNumber++
		buf.WriteString(fmt.Sprintf(`%d. `, in.listNumber))
		renderText()
		h.renderChildren(buf, in.AddSpace(), b)
		in.listOpened = true
	default:
		renderText()
		h.renderChildren(buf, in.AddNBSpace(), b)
	}
}

func (h *MD) renderFile(buf writer, in *renderState, b *model.Block) {
	file := b.GetFile()
	if file == nil || file.State != model.BlockContentFile_Done {
		return
	}
	title, filename, ok := h.getLinkInfo(file.TargetObjectId)
	if !ok {
		filename = h.fn.Get("files", file.TargetObjectId, filepath.Base(file.Name), filepath.Ext(file.Name))
		title = filepath.Base(file.Name)
	}
	buf.WriteString(in.indent)
	if file.Type != model.BlockContentFile_Image {
		fmt.Fprintf(buf, "[%s](%s)    \n", title, filename)
		h.fileHashes = append(h.fileHashes, file.TargetObjectId)
	} else {
		fmt.Fprintf(buf, "![%s](%s)    \n", title, filename)
		h.imageHashes = append(h.imageHashes, file.TargetObjectId)
	}
}

func (h *MD) renderBookmark(buf writer, in *renderState, b *model.Block) {
	bm := b.GetBookmark()
	if bm != nil && bm.Url != "" {
		buf.WriteString(in.indent)
		url, e := uri.ParseURI(bm.Url)
		if e == nil {
			fmt.Fprintf(buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(bm.Title)), url.String())
		}
	}
}

func (h *MD) renderDiv(buf writer, in *renderState, b *model.Block) {
	switch b.GetDiv().Style {
	case model.BlockContentDiv_Dots, model.BlockContentDiv_Line:
		buf.WriteString(" --- \n")
	}
	h.renderChildren(buf, in, b)
}

func (h *MD) renderLayout(buf writer, in *renderState, b *model.Block) {
	style := model.BlockContentLayoutStyle(-1)
	layout := b.GetLayout()
	if layout != nil {
		style = layout.Style
	}

	switch style {
	default:
		h.renderChildren(buf, in, b)
	}
}

func (h *MD) renderLink(buf writer, in *renderState, b *model.Block) {
	l := b.GetLink()
	if l != nil && l.TargetBlockId != "" {
		title, filename, ok := h.getLinkInfo(l.TargetBlockId)
		if ok {
			buf.WriteString(in.indent)
			fmt.Fprintf(buf, "[%s](%s)    \n", escape.MarkdownCharacters(html.EscapeString(title)), filename)
		}
	}
}

func (h *MD) renderLatex(buf writer, in *renderState, b *model.Block) {
	l := b.GetLatex()
	if l != nil {
		buf.WriteString(in.indent)
		fmt.Fprintf(buf, "\n$$\n%s\n$$\n", l.Text)
	}
}

func (h *MD) renderTable(buf writer, in *renderState, b *model.Block) {
	if t := b.GetTable(); t == nil {
		return
	}

	err := func() error {
		tb, err := table.NewTable(h.s, b.Id)
		if err != nil {
			return err
		}

		rowsCount := len(tb.RowIDs())
		colsCount := len(tb.ColumnIDs())
		maxColWidth := make([]int, colsCount)
		cells := make([][]string, rowsCount)
		for rowIdx := range tb.RowIDs() {
			cells[rowIdx] = make([]string, colsCount)
		}

		err = tb.Iterate(func(b simple.Block, pos table.CellPosition) bool {
			cellBuf := &bytes.Buffer{}
			if b != nil {
				h.render(cellBuf, in, b.Model())
			}
			content := cellBuf.String()
			content = strings.ReplaceAll(content, "\r\n", " ")
			content = strings.ReplaceAll(content, "\n", " ")
			content = strings.TrimSpace(content)
			if content == "" {
				content = " "
			}
			content = " " + content + " "

			if len(content) > maxColWidth[pos.ColNumber] {
				maxColWidth[pos.ColNumber] = len(content)
			}
			cells[pos.RowNumber][pos.ColNumber] = content
			return true
		})
		if err != nil {
			return err
		}

		for i, w := range maxColWidth {
			// The minimum width of a column must be 3
			if w < 3 {
				maxColWidth[i] = 3
			}
		}

		for i, row := range cells {
			buf.WriteString(in.indent)
			rowStart := "|"
			for colNumber, cell := range row {
				tmpl := fmt.Sprintf("%%%ds|", maxColWidth[colNumber])
				fmt.Fprint(buf, rowStart)
				rowStart = ""
				fmt.Fprintf(buf, tmpl, cell)
			}
			fmt.Fprintln(buf)

			// Header rule
			if i == 0 {
				buf.WriteString(in.indent)
				rowStart := "|"
				for colNumber := range row {
					fmt.Fprint(buf, rowStart)
					rowStart = ""
					buf.WriteRune(':')
					for i := 0; i < maxColWidth[colNumber]-1; i++ {
						buf.WriteRune('-')
					}
					buf.WriteRune('|')
				}
				fmt.Fprintln(buf)
			}
		}

		return nil
	}()
	fmt.Fprintln(buf)

	if err != nil {
		fmt.Fprintf(buf, "error while rendering table: %s", err)
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

func (h *MD) SetKnownDocs(docs map[string]*domain.Details) converter.Converter {
	h.knownDocs = docs
	return h
}

func (h *MD) getLinkInfo(docId string) (title, filename string, ok bool) {
	info, ok := h.knownDocs[docId]
	if !ok {
		return
	}
	title = info.GetString(bundle.RelationKeyName)
	// if object is a file
	layout := info.GetInt64(bundle.RelationKeyLayout)
	if layout == int64(model.ObjectType_file) || layout == int64(model.ObjectType_image) || layout == int64(model.ObjectType_audio) || layout == int64(model.ObjectType_video) || layout == int64(model.ObjectType_pdf) {
		ext := info.GetString(bundle.RelationKeyFileExt)
		if ext != "" {
			ext = "." + ext
		}
		title = strings.TrimSuffix(title, ext)
		if title == "" {
			title = docId
		}
		filename = h.fn.Get("files", docId, title, ext)
		return
	}
	if title == "" {
		title = info.GetString(bundle.RelationKeySnippet)
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
				urlP, e := uri.ParseURI(m.Param)
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
		case model.BlockContentTextMark_Emoji:
			if start {
				_, err := buf.WriteString(m.Param)
				if err != nil {
					log.Errorf("failed to write emoji: %s, %v", m.Param, err)
				}
			}
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

// GenerateSchemaFileName creates a schema file name from the object type name
func (h *MD) GenerateSchemaFileName(typeName string) string {
	// Convert to lowercase and replace spaces with underscores
	fileName := strings.ToLower(typeName)
	fileName = strings.ReplaceAll(fileName, " ", "_")
	fileName = strings.ReplaceAll(fileName, "/", "_")
	fileName = strings.ReplaceAll(fileName, "\\", "_")
	return "./schemas/" + fileName + ".schema.json"
}

// GenerateJSONSchema generates a JSON schema for the object type
func (h *MD) GenerateJSONSchema() ([]byte, error) {
	objectTypeId := h.s.LocalDetails().GetString(bundle.RelationKeyType)
	objectTypeDetails, exists := h.knownDocs[objectTypeId]
	if !exists {
		return nil, fmt.Errorf("object type not found")
	}

	// Get type name for URN
	typeName := objectTypeDetails.GetString(bundle.RelationKeyName)
	typeNameForId := strings.ToLower(typeName)
	typeNameForId = strings.ReplaceAll(typeNameForId, " ", "-")
	typeNameForId = strings.ReplaceAll(typeNameForId, "/", "-")
	typeNameForId = strings.ReplaceAll(typeNameForId, "\\", "-")

	// Get dates and author for URN components
	lastModified := objectTypeDetails.GetInt64(bundle.RelationKeyLastModifiedDate)
	if lastModified == 0 {
		// Fallback to created date if no last modified
		lastModified = objectTypeDetails.GetInt64(bundle.RelationKeyCreatedDate)
	}
	dateForId := ""
	if lastModified > 0 {
		dateForId = time.Unix(lastModified, 0).UTC().Format("2006-01-02")
	} else {
		// Use current date as fallback
		dateForId = time.Now().UTC().Format("2006-01-02")
	}

	// Get author (lastModifiedBy or fallback to creator)
	author := objectTypeDetails.GetString(bundle.RelationKeyLastModifiedBy)
	if author == "" {
		author = objectTypeDetails.GetString(bundle.RelationKeyCreator)
	}
	// Create short author ID (first 4 chars of ID or "anon")
	authorForId := "anon"
	if author != "" {
		if len(author) >= 4 {
			authorForId = author[:4]
		} else {
			authorForId = author
		}
	}

	// Build URN-style ID
	// Format: urn:anytype:schema:2025-06-14:author-7a12:type-task:gen-2.3.0
	schemaId := fmt.Sprintf("urn:anytype:schema:%s:author-%s:type-%s:gen-%s",
		dateForId,
		authorForId,
		typeNameForId,
		"1.0.0")

	// Handle dates for x-type-date
	createdDate := objectTypeDetails.GetInt64(bundle.RelationKeyCreatedDate)
	typeDate := ""
	if createdDate > 0 {
		typeDate = time.Unix(createdDate, 0).UTC().Format(time.RFC3339)
	}

	schema := map[string]interface{}{
		"$schema":       "http://json-schema.org/draft-07/schema#",
		"$id":           schemaId,
		"type":          "object",
		"x-app":         "Anytype",
		"x-type-author": author,
		"x-type-date":   typeDate,
		"x-genVersion":  "1.0.0",
	}

	// Add type metadata
	if typeName != "" {
		schema["title"] = typeName
	}
	if description := objectTypeDetails.GetString(bundle.RelationKeyDescription); description != "" {
		schema["description"] = description
	}

	// Add custom extensions for Anytype-specific metadata
	if plural := objectTypeDetails.GetString(bundle.RelationKeyPluralName); plural != "" {
		schema["x-plural"] = plural
	}
	if iconEmoji := objectTypeDetails.GetString(bundle.RelationKeyIconEmoji); iconEmoji != "" {
		schema["x-icon-emoji"] = iconEmoji
	}
	if iconImage := objectTypeDetails.GetString(bundle.RelationKeyIconImage); iconImage != "" {
		schema["x-icon-name"] = iconImage
	}

	properties := make(map[string]interface{})
	required := []string{}

	// Get all relations for this type
	featuredRelations := objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	regularRelations := objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedRelations)
	allRelations := append(featuredRelations, regularRelations...)

	// Also add the Type property
	typeRelationId := ""
	for id, d := range h.knownDocs {
		if d.GetString(bundle.RelationKeyRelationKey) == bundle.RelationKeyType.String() &&
			d.GetInt64(bundle.RelationKeyLayout) == int64(model.ObjectType_relation) {
			typeRelationId = id
			break
		}
	}
	if typeRelationId != "" {
		allRelations = append([]string{typeRelationId}, allRelations...)
	}

	for _, relationId := range allRelations {
		relationDetails, ok := h.knownDocs[relationId]
		if !ok || relationDetails.GetBool(bundle.RelationKeyIsHidden) {
			continue
		}

		name := relationDetails.GetString(bundle.RelationKeyName)
		format := model.RelationFormat(relationDetails.GetInt64(bundle.RelationKeyRelationFormat))

		property := h.getJSONSchemaProperty(relationDetails, format)
		if property != nil {
			properties[name] = property

			// Mark required fields (you can customize this logic)
			if featuredRelations != nil && slices.Contains(featuredRelations, relationId) {
				required = append(required, name)
			}
		}
	}

	schema["properties"] = properties
	if len(required) > 0 {
		schema["required"] = required
	}

	return json.MarshalIndent(schema, "", "  ")
}

func (h *MD) getJSONSchemaProperty(relationDetails *domain.Details, format model.RelationFormat) map[string]interface{} {
	key := relationDetails.GetString(bundle.RelationKeyRelationKey)
	property := make(map[string]interface{})
	if key == bundle.RelationKeyType.String() {
		// exception for Object Type relation, we don't need to fill it as object
		property["const"] = relationDetails.GetString(bundle.RelationKeyName)
		return property
	}
	switch format {
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		property["type"] = "string"
		property["description"] = "Long text field"

	case model.RelationFormat_number:
		property["type"] = "number"

	case model.RelationFormat_checkbox:
		property["type"] = "boolean"

	case model.RelationFormat_date:
		property["type"] = "string"
		property["format"] = "date"
		if relationDetails.GetBool(bundle.RelationKeyRelationFormatIncludeTime) {
			property["format"] = "date-time"
		}

	case model.RelationFormat_tag:
		property["type"] = "array"
		property["items"] = map[string]string{"type": "string"}

	case model.RelationFormat_status:
		property["type"] = "string"
		// Get status options if available
		options := h.getRelationOptions(relationDetails.GetString(bundle.RelationKeyId))
		if len(options) > 0 {
			property["enum"] = options
		}

	case model.RelationFormat_email:
		property["type"] = "string"
		property["format"] = "email"

	case model.RelationFormat_url:
		property["type"] = "string"
		property["format"] = "uri"

	case model.RelationFormat_phone:
		property["type"] = "string"
		property["pattern"] = "^[+]?[0-9\\s()-]+$"

	case model.RelationFormat_file:
		// For file format relations, the value is the file path
		property["type"] = "string"
		property["description"] = "Path to the file in the export"

	case model.RelationFormat_object:
		// For object relations, create a more detailed schema
		property["type"] = "object"
		properties := map[string]interface{}{
			"Name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the referenced object",
			},
			"File": map[string]interface{}{
				"type":        "string",
				"description": "Path to the object file in the export",
			},
		}
		required := []string{"Name"}

		// Add Object Type property
		properties["Object type"] = map[string]interface{}{
			"type":        "string",
			"description": "Type of the referenced object",
		}

		// Check if specific object types are defined for this relation
		if objectTypes := relationDetails.GetStringList(bundle.RelationKeyRelationFormatObjectTypes); len(objectTypes) > 0 {
			// Get the names of these object types
			var typeNames []string
			for _, typeId := range objectTypes {
				if typeDetails, exists := h.knownDocs[typeId]; exists {
					if typeName := typeDetails.GetString(bundle.RelationKeyName); typeName != "" {
						typeNames = append(typeNames, typeName)
					}
				}
			}

			// If we found type names, add them as enum
			if len(typeNames) > 0 {
				properties["Object type"].(map[string]interface{})["enum"] = typeNames
			}
		}

		property["properties"] = properties
		property["required"] = required

	default:
		property["type"] = "string"
	}

	return property
}

func (h *MD) getRelationOptions(relationId string) []string {
	var options []string

	// Look for relation options in knownDocs
	for _, details := range h.knownDocs {
		if details.GetString(bundle.RelationKeyRelationKey) == relationId &&
			details.GetInt64(bundle.RelationKeyLayout) == int64(model.ObjectType_relationOption) {
			optionName := details.GetString(bundle.RelationKeyName)
			if optionName != "" {
				options = append(options, optionName)
			}
		}
	}

	return options
}

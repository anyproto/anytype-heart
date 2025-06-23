package md

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/escape"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("md-export")

type FileNamer interface {
	Get(path, hash, title, ext string) (name string)
}

type ObjectResolver interface {
	// ResolveRelation gets relation details by ID
	ResolveRelation(relationId string) (*domain.Details, error)
	// ResolveType gets type details by ID
	ResolveType(typeId string) (*domain.Details, error)
	// ResolveRelationOptions gets relation options for a given relation
	ResolveRelationOptions(relationKey string) ([]*domain.Details, error)
	// ResolveObject gets object details by ID (for already loaded objects)
	ResolveObject(objectId string) (*domain.Details, bool)
	// GetRelationByKey gets relation by its key
	GetRelationByKey(relationKey string) (*domain.Details, error)
}

func NewMDConverter(s *state.State, fn FileNamer, includeRelations bool) converter.Converter {
	return &MD{s: s, fn: fn, includeRelations: includeRelations, knownDocs: make(map[string]*domain.Details)}
}

func NewMDConverterWithSchema(s *state.State, fn FileNamer, includeRelations bool, includeSchema bool) converter.Converter {
	return &MD{s: s, fn: fn, includeRelations: includeRelations, includeSchema: true, knownDocs: make(map[string]*domain.Details)}
}

func NewMDConverterWithResolver(s *state.State, fn FileNamer, includeRelations bool, includeSchema bool, resolver ObjectResolver) converter.Converter {
	return &MD{s: s, fn: fn, includeRelations: includeRelations, includeSchema: includeSchema, resolver: resolver, knownDocs: make(map[string]*domain.Details)}
}

type MD struct {
	s *state.State

	fileHashes  []string
	imageHashes []string

	knownDocs map[string]*domain.Details
	resolver  ObjectResolver

	includeRelations bool
	includeSchema    bool
	mw               *marksWriter
	fn               FileNamer
}

var shortObjectRelations = append(removeArrayRelations, []string{
	bundle.RelationKeyBacklinks.String(),
	bundle.RelationKeyLinks.String(),
	bundle.RelationKeyMentions.String()}...)

var removeArrayRelations = []string{
	bundle.RelationKeyLastModifiedBy.String(),
	bundle.RelationKeyCreator.String(),
	bundle.RelationKeyType.String(),
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

	// Use resolver if available
	if h.resolver == nil {
		log.Error("MD converter requires ObjectResolver to resolve properties, but it is not set")
		return
	}

	// Get object type details
	objectTypeId := h.s.LocalDetails().GetString(bundle.RelationKeyType)
	if objectTypeId == "" {
		return
	}

	objectTypeDetails, err := h.resolver.ResolveType(objectTypeId)
	if err != nil || objectTypeDetails == nil {
		return
	}

	// Get type name for YAML export
	typeName := objectTypeDetails.GetString(bundle.RelationKeyName)

	// Collect properties to export
	properties := make([]yaml.Property, 0)

	// Get all relation IDs (excluding type relation which will be handled separately)
	var propertiesIds []string
	propertiesIds = append(propertiesIds, objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)...)
	propertiesIds = append(propertiesIds, objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedRelations)...)
	propertiesIds = slices.Compact(propertiesIds)

	// Check if this is a collection
	layout, hasLayout := h.s.Layout()
	isCollection := hasLayout && layout == model.ObjectType_collection

	// Process each relation
	for _, id := range propertiesIds {
		relationDetails, err := h.resolver.ResolveRelation(id)
		if err != nil || relationDetails == nil {
			continue
		}
		if relationDetails.GetBool(bundle.RelationKeyIsHidden) {
			continue
		}

		name := relationDetails.GetString(bundle.RelationKeyName)
		key := relationDetails.GetString(bundle.RelationKeyRelationKey)
		format := model.RelationFormat(relationDetails.GetInt64(bundle.RelationKeyRelationFormat))
		includeTime := relationDetails.GetBool(bundle.RelationKeyRelationFormatIncludeTime)

		// Skip the type relation - it will be added as "Object type" at the end
		if key == bundle.RelationKeyType.String() {
			continue
		}

		v := h.s.CombinedDetails().Get(domain.RelationKey(key))
		if v.IsNull() {
			continue
		}

		// Process special cases for file and object relations
		var processedValue domain.Value = v

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

			// Multiple files
			var fileList []string
			for _, fileId := range ids {
				if info, _ := h.getObjectInfo(fileId); info != nil {
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

			if len(fileList) > 0 {
				processedValue = domain.StringList(fileList)
			} else {
				continue
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
			var (
				removeArray bool
			)

			if slices.Contains(removeArrayRelations, key) {
				removeArray = true
			}

			// Handle special single-value relations
			if removeArray && len(ids) > 0 {
				title, _, ok := h.getLinkInfo(ids[0])
				if ok {
					processedValue = domain.String(title)
				}
			} else {
				var objectList []string
				for _, id := range ids {
					title, filename, ok := h.getLinkInfo(id)
					if !ok {
						continue
					}
					if h.knownDocs[id] == nil {
						objectList = append(objectList, title)
					} else {
						objectList = append(objectList, filename)
					}
				}
				if len(objectList) > 0 {
					processedValue = domain.StringList(objectList)
				} else {
					continue
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

			var nameList []string
			for _, id := range ids {
				if d, _ := h.getObjectInfo(id); d != nil {
					if label := d.Get(bundle.RelationKeyName).String(); label != "" {
						nameList = append(nameList, label)
					}
				}
			}
			if len(nameList) > 0 {
				processedValue = domain.StringList(nameList)
			} else {
				continue
			}

		}

		// Create property
		prop := yaml.Property{
			Name:        name,
			Key:         key,
			Format:      format,
			Value:       processedValue,
			IncludeTime: includeTime,
		}
		properties = append(properties, prop)
	}

	// Skip empty property lists unless it's a collection
	if len(properties) == 0 && !isCollection {
		return
	}

	// Prepare export options
	exportOptions := &yaml.ExportOptions{
		ObjectTypeName: typeName,
	}

	// Add object ID as a special property
	if objectId := h.s.LocalDetails().GetString(bundle.RelationKeyId); objectId != "" {
		idProp := yaml.Property{
			Name:   "id",
			Key:    "id",
			Format: model.RelationFormat_shorttext,
			Value:  domain.String(objectId),
		}
		properties = append([]yaml.Property{idProp}, properties...)
	}

	// Export using YAML exporter
	yamlData, err := yaml.ExportToYAML(properties, exportOptions)
	if err != nil {
		log.Errorf("failed to export properties to YAML: %v", err)
		return
	}

	// Extract just the content (without delimiters)
	frontMatter, _, _ := yaml.ExtractYAMLFrontMatter(yamlData)
	if len(frontMatter) == 0 {
		return
	}

	// Write custom front matter with schema reference if needed
	fmt.Fprintf(buf, "---\n")

	// Add JSON schema reference if enabled
	if h.includeSchema && typeName != "" {
		schemaFileName := h.GenerateSchemaFileName(typeName)
		fmt.Fprintf(buf, "# yaml-language-server: $schema=%s\n", schemaFileName)
	}

	// Write the YAML content
	buf.Write(frontMatter)
	
	// Ensure there's a newline before closing delimiter or collection
	if len(frontMatter) > 0 && frontMatter[len(frontMatter)-1] != '\n' {
		buf.WriteString("\n")
	}

	// Add collection objects if this is a collection
	if isCollection {
		collectionObjects := h.s.GetStoreSlice(template.CollectionStoreKey)
		if len(collectionObjects) > 0 {
			fmt.Fprintf(buf, "Collection:\n")
			for _, objId := range collectionObjects {
				if d, isInExport := h.getObjectInfo(objId); d != nil {
					objectName := d.Get(bundle.RelationKeyName).String()
					if objectName == "" {
						objectName = objId
					}

					fmt.Fprintf(buf, "  - Name: %s\n", objectName)

					if isInExport {
						filename := h.fn.Get("", objId, objectName, h.Ext())
						fmt.Fprintf(buf, "    File: %s\n", filename)
					} else {
						fmt.Fprintf(buf, "    Id: %s\n", objId)
					}
				} else {
					fmt.Fprintf(buf, "  - Name: %s\n", objId)
				}
			}
		}
	}

	fmt.Fprintf(buf, "---\n\n")
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

func (h *MD) getObjectInfo(objectId string) (details *domain.Details, isIncludedInExport bool) {
	details, _ = h.knownDocs[objectId]
	if details != nil {
		return details, true
	}

	details, _ = h.resolver.ResolveObject(objectId)
	return details, false
}

func (h *MD) getLinkInfo(docId string) (title, filename string, ok bool) {
	info, _ := h.getObjectInfo(docId)
	if info == nil {
		return
	}
	ok = true
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

// GenerateJSONSchema generates a JSON schema for the object type using the schema package
func (h *MD) GenerateJSONSchema() ([]byte, error) {
	if h.resolver == nil {
		return nil, fmt.Errorf("resolver not set")
	}

	objectTypeId := h.s.LocalDetails().GetString(bundle.RelationKeyType)
	objectTypeDetails, err := h.resolver.ResolveType(objectTypeId)
	if err != nil || objectTypeDetails == nil {
		return nil, fmt.Errorf("object type not found")
	}

	// Get all relations for this type
	featuredRelations := objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	regularRelations := objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedRelations)
	allRelations := append(featuredRelations, regularRelations...)

	// Collect relation details
	var relationDetailsList []*domain.Details
	for _, relationId := range allRelations {
		relationDetails, err := h.resolver.ResolveRelation(relationId)
		if err == nil && relationDetails != nil {
			relationDetailsList = append(relationDetailsList, relationDetails)
		}
	}

	// Create schema using the schema package
	s, err := schema.SchemaFromObjectDetailsWithResolver(objectTypeDetails, relationDetailsList, h.resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Export using the schema package
	exporter := schema.NewJSONSchemaExporter("  ")
	var buf bytes.Buffer
	if err := exporter.Export(s, &buf); err != nil {
		return nil, fmt.Errorf("failed to export schema: %w", err)
	}

	return buf.Bytes(), nil
}

func (h *MD) getRelationOptions(relationKey string) []string {
	var options []string

	// Return empty if resolver is not available
	if h.resolver == nil {
		return options
	}

	optionDetails, err := h.resolver.ResolveRelationOptions(relationKey)
	if err == nil && optionDetails != nil {
		for _, details := range optionDetails {
			if optionName := details.GetString(bundle.RelationKeyName); optionName != "" {
				options = append(options, optionName)
			}
		}
	}

	return options
}

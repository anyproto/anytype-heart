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
	case model.SmartBlockType_STType,
		model.SmartBlockType_STRelation,
		model.SmartBlockType_STRelationOption,
		model.SmartBlockType_Participant,
		model.SmartBlockType_SpaceView,
		model.SmartBlockType_ChatObject,
		model.SmartBlockType_ChatDerivedObject:
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

	if h.resolver == nil {
		log.Error("MD converter requires ObjectResolver to resolve properties, but it is not set")
		return
	}

	// Get object type information
	objectTypeDetails, typeName := h.getObjectTypeInfo()
	if objectTypeDetails == nil {
		return
	}

	// Collect all properties to export
	properties := h.collectProperties(objectTypeDetails)
	if len(properties) == 0 {
		return
	}

	// Add object ID after other properties
	properties = h.appendObjectId(properties)

	// Export to YAML
	h.exportPropertiesToYAML(buf, properties, typeName)
}

// getObjectTypeInfo retrieves object type details and name
func (h *MD) getObjectTypeInfo() (*domain.Details, string) {
	objectTypeId := h.s.LocalDetails().GetString(bundle.RelationKeyType)
	if objectTypeId == "" {
		return nil, ""
	}

	objectTypeDetails, err := h.resolver.ResolveType(objectTypeId)
	if err != nil || objectTypeDetails == nil {
		return nil, ""
	}

	typeName := objectTypeDetails.GetString(bundle.RelationKeyName)
	return objectTypeDetails, typeName
}

// collectProperties gathers all properties from relations
func (h *MD) collectProperties(objectTypeDetails *domain.Details) []yaml.Property {
	var properties []yaml.Property

	// Get all relation IDs
	relationIds := h.getRelationIds(objectTypeDetails)

	// Process each relation
	for _, id := range relationIds {
		relDetails, err := h.resolver.ResolveRelation(id)
		if err != nil || relDetails == nil {
			continue
		}
		prop := h.processRelation(relDetails)
		if prop != nil {
			properties = append(properties, *prop)
		}
	}

	// Add collection property if applicable
	if h.isCollection() {
		properties = h.addCollectionProperty(properties)
	}

	// Add system properties
	properties = h.addSystemProperties(properties)

	return properties
}

// getRelationIds returns all relation IDs from the object type
func (h *MD) getRelationIds(objectTypeDetails *domain.Details) []string {
	var ids []string
	ids = append(ids, objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)...)
	ids = append(ids, objectTypeDetails.GetStringList(bundle.RelationKeyRecommendedRelations)...)
	return slices.Compact(ids)
}

// isCollection checks if the current object is a collection
func (h *MD) isCollection() bool {
	layout, hasLayout := h.s.Layout()
	return hasLayout && layout == model.ObjectType_collection
}

// processRelation processes a single relation and returns a property
func (h *MD) processRelation(details *domain.Details) *yaml.Property {
	// Extract relation metadata
	key := details.GetString(bundle.RelationKeyRelationKey)

	// Get the value for this relation
	v := h.s.CombinedDetails().Get(domain.RelationKey(key))

	// Process the value based on format
	format := model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat))
	processedValue, ok := h.processRelationValue(v, format, key)
	if !ok {
		return nil
	}

	return &yaml.Property{
		Name:        details.GetString(bundle.RelationKeyName),
		Key:         key,
		Format:      format,
		Value:       processedValue,
		IncludeTime: details.GetBool(bundle.RelationKeyRelationFormatIncludeTime),
	}
}

// processRelationValue processes a relation value based on its format
// Returns the processed value and whether it should be included
func (h *MD) processRelationValue(v domain.Value, format model.RelationFormat, key string) (domain.Value, bool) {
	if v.IsNull() {
		return domain.Value{}, false
	}
	switch format {
	case model.RelationFormat_file:
		return h.processFileRelation(v)
	case model.RelationFormat_object:
		return h.processObjectRelation(v, key)
	case model.RelationFormat_tag, model.RelationFormat_status:
		return h.processTagOrStatusRelation(v)
	default:
		return v, true
	}
}

// processFileRelation handles file format relations
func (h *MD) processFileRelation(v domain.Value) (domain.Value, bool) {
	ids := v.WrapToStringList()
	if len(ids) == 0 {
		return v, false
	}

	var fileList []string
	for _, fileId := range ids {
		if filename := h.processFileId(fileId); filename != "" {
			fileList = append(fileList, filename)
		}
	}

	if len(fileList) == 0 {
		return v, false
	}
	return domain.StringList(fileList), true
}

// processFileId processes a single file ID and returns its filename
func (h *MD) processFileId(fileId string) string {
	info, _ := h.getObjectInfo(fileId)
	if info == nil {
		return ""
	}

	layout := info.GetInt64(bundle.RelationKeyLayout)
	if !h.isFileLayout(layout) {
		return ""
	}

	_, filename, _ := h.getLinkInfo(fileId)
	if filename == "" {
		return ""
	}

	// Track file hashes
	if layout == int64(model.ObjectType_image) {
		h.imageHashes = append(h.imageHashes, fileId)
	} else {
		h.fileHashes = append(h.fileHashes, fileId)
	}

	return filename
}

// isFileLayout checks if the layout represents a file object type
func (h *MD) isFileLayout(layout int64) bool {
	return layout == int64(model.ObjectType_file) ||
		layout == int64(model.ObjectType_image) ||
		layout == int64(model.ObjectType_audio) ||
		layout == int64(model.ObjectType_video) ||
		layout == int64(model.ObjectType_pdf)
}

// processObjectRelation handles object format relations
func (h *MD) processObjectRelation(v domain.Value, key string) (domain.Value, bool) {
	ids := v.WrapToStringList()
	if len(ids) == 0 {
		return v, false
	}

	// Check if this is a single-value relation
	if slices.Contains(removeArrayRelations, key) && len(ids) > 0 {
		title, _, ok := h.getLinkInfo(ids[0])
		if ok {
			return domain.String(title), true
		}
		return v, false
	}

	// Process multiple objects
	var objectList []string
	for _, id := range ids {
		if name := h.getObjectName(id); name != "" {
			objectList = append(objectList, name)
		}
	}

	if len(objectList) == 0 {
		return v, false
	}
	return domain.StringList(objectList), true
}

// getObjectName returns the appropriate name for an object
func (h *MD) getObjectName(objectId string) string {
	title, filename, ok := h.getLinkInfo(objectId)
	if !ok {
		return ""
	}

	// Use filename if object is in export, otherwise use title
	if h.knownDocs[objectId] != nil {
		return filename
	}
	return title
}

// processTagOrStatusRelation handles tag and status format relations
func (h *MD) processTagOrStatusRelation(v domain.Value) (domain.Value, bool) {
	ids := v.WrapToStringList()
	if len(ids) == 0 {
		return v, false
	}

	var nameList []string
	for _, id := range ids {
		if d, _ := h.getObjectInfo(id); d != nil {
			if label := d.Get(bundle.RelationKeyName).String(); label != "" {
				nameList = append(nameList, label)
			}
		}
	}

	if len(nameList) == 0 {
		return v, false
	}
	return domain.StringList(nameList), true
}

// appendObjectId adds object ID after other properties
func (h *MD) appendObjectId(properties []yaml.Property) []yaml.Property {
	objectId := h.s.LocalDetails().GetString(bundle.RelationKeyId)
	if objectId == "" {
		return properties
	}

	idProp := yaml.Property{
		Name:   "id",
		Key:    "id",
		Format: model.RelationFormat_shorttext,
		Value:  domain.String(objectId),
	}
	return append(properties, idProp)
}

// exportPropertiesToYAML exports properties to YAML format
func (h *MD) exportPropertiesToYAML(buf writer, properties []yaml.Property, typeName string) {
	exportOptions := &yaml.ExportOptions{}

	// Add schema reference if enabled
	if h.includeSchema && typeName != "" {
		exportOptions.SchemaReference = GenerateSchemaFileName(typeName)
	}

	// Export using YAML exporter
	yamlData, err := yaml.ExportToYAML(properties, exportOptions)
	if err != nil {
		log.Errorf("failed to export properties to YAML: %v", err)
		return
	}

	// Write the YAML front matter
	_, _ = buf.Write(yamlData) // Error is ignored as buffer writes don't fail
}

// addCollectionProperty adds Collection property to the properties list if there are collection items
func (h *MD) addCollectionProperty(properties []yaml.Property) []yaml.Property {
	collectionObjects := h.s.GetStoreSlice(template.CollectionStoreKey)
	if len(collectionObjects) == 0 {
		return properties
	}

	var collectionList []string
	for _, objId := range collectionObjects {
		title, filename, ok := h.getLinkInfo(objId)
		if !ok {
			continue
		}

		// Same logic as object relations - use filename if in export, otherwise title
		if h.knownDocs[objId] != nil {
			collectionList = append(collectionList, filename)
		} else {
			collectionList = append(collectionList, title)
		}
	}

	if len(collectionList) == 0 {
		return properties
	}

	collectionProp := yaml.Property{
		Name:   "Collection",
		Key:    schema.CollectionPropertyKey,
		Format: model.RelationFormat_object,
		Value:  domain.StringList(collectionList),
	}
	return append(properties, collectionProp)
}

// addSystemProperties adds system properties that are not already included
func (h *MD) addSystemProperties(properties []yaml.Property) []yaml.Property {
	// Create a map of existing property keys
	existingKeys := make(map[string]bool)
	for _, prop := range properties {
		existingKeys[prop.Key] = true
	}

	// Add system properties if they have values and are not already included
	for _, key := range schema.SystemProperties {
		if existingKeys[key] {
			continue
		}

		v := h.s.CombinedDetails().Get(domain.RelationKey(key))
		if v.IsNull() {
			continue
		}
		if s, ok := v.TryString(); ok && s == "" {
			continue
		}

		relDetails, err := h.resolver.GetRelationByKey(key)
		if err != nil || relDetails == nil {
			continue
		}

		prop := h.processRelation(relDetails)
		if prop == nil {
			continue
		}
		properties = append(properties, *prop)
	}

	return properties
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

// Replace characters that would break or confuse file-paths.
var replacer = strings.NewReplacer(
	" ", "_", // spaces → underscores
	"/", "_", // forward slashes → underscores
	"\\", "_", // backslashes → underscores
)

// GenerateSchemaFileName creates an OS-safe schema file name from the object type name.
func GenerateSchemaFileName(typeName string) string {
	sanitised := strings.ToLower(typeName)
	sanitised = replacer.Replace(sanitised)
	return filepath.Join("schemas", sanitised+".schema.json")
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

	// Create adapter for schema.ObjectResolver
	schemaResolver := &schemaResolverAdapter{resolver: h.resolver}

	// Create schema using the schema package
	s, err := schema.SchemaFromObjectDetailsWithResolver(objectTypeDetails, relationDetailsList, schemaResolver)
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

// schemaResolverAdapter adapts md.ObjectResolver to schema.ObjectResolver
type schemaResolverAdapter struct {
	resolver ObjectResolver
}

func (a *schemaResolverAdapter) RelationById(relationId string) (*domain.Details, error) {
	return a.resolver.ResolveRelation(relationId)
}

func (a *schemaResolverAdapter) RelationByKey(relationKey string) (*domain.Details, error) {
	return a.resolver.GetRelationByKey(relationKey)
}

func (a *schemaResolverAdapter) RelationOptions(relationKey string) ([]*domain.Details, error) {
	return a.resolver.ResolveRelationOptions(relationKey)
}

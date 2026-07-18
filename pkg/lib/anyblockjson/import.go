package anyblockjson

// import.go reconstructs a snapshot from a validated AnyBlock JSON document
// (§2–§7, §9).

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

type jsonDoc struct {
	Schema      string            `json:"$schema"`
	Version     int               `json:"version"`
	Kind        string            `json:"kind"`
	Id          string            `json:"id"`
	Type        string            `json:"type"`
	TemplateFor string            `json:"templateFor"`
	Key         string            `json:"key"`
	Properties  map[string]any    `json:"properties"`
	Refs        map[string]string `json:"refs"`
	Blocks      []*jsonBlock      `json:"blocks"`
	Items       []string          `json:"items"`
	Store       map[string]any    `json:"store"`
	Root        *jsonRootEscape   `json:"root"`
}

type jsonRootEscape struct {
	Fields          map[string]any `json:"fields"`
	BackgroundColor string         `json:"backgroundColor"`
}

// jsonBlock is the union of every §5 block shape; the schema guarantees only
// type-appropriate fields are present.
type jsonBlock struct {
	Id   string `json:"id"`
	Type string `json:"type"`

	Checked   bool   `json:"checked"`
	Color     string `json:"color"`
	Text      string `json:"text"`
	Language  string `json:"language"`
	IconEmoji string `json:"iconEmoji"`
	IconImage string `json:"iconImage"`

	ObjectId    string          `json:"objectId"`
	Name        string          `json:"name"`
	MimeType    string          `json:"mimeType"`
	Size        int64           `json:"size"`
	Style       string          `json:"style"`
	AddedAt     string          `json:"addedAt"`
	Hash        string          `json:"hash"`
	Url         string          `json:"url"`
	CardStyle   string          `json:"cardStyle"`
	IconSize    string          `json:"iconSize"`
	Description string          `json:"description"`
	Properties  json.RawMessage `json:"properties"` // link: []string; dataview: []jsonDvProperty
	Processor   string          `json:"processor"`
	Key         string          `json:"key"`

	Layout    string `json:"layout"`
	Limit     int32  `json:"limit"`
	ViewId    string `json:"viewId"`
	AutoAdded bool   `json:"autoAdded"`

	Columns []jsonTableColumn `json:"columns"`
	Rows    []jsonTableRow    `json:"rows"`

	IsCollection bool       `json:"isCollection"`
	Source       []string   `json:"source"`
	Views        []jsonView `json:"views"`

	Align           string         `json:"align"`
	VerticalAlign   string         `json:"verticalAlign"`
	BackgroundColor string         `json:"backgroundColor"`
	Fields          map[string]any `json:"fields"`
	Children        []*jsonBlock   `json:"children"`
}

// Unmarshal validates data and reconstructs a snapshot (§13). Errors wrap
// *ValidationError with JSON-path-addressed issues.
func Unmarshal(data []byte, opts Options) (model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	if _, err := validateToDoc(data); err != nil {
		return 0, nil, err
	}
	var doc jsonDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, nil, fmt.Errorf("decode document: %w", err)
	}
	imp := &importer{opts: opts, doc: &doc}
	return imp.build()
}

type importer struct {
	opts Options
	doc  *jsonDoc
}

func (imp *importer) genId() string {
	if imp.opts.GenerateId != nil {
		return imp.opts.GenerateId()
	}
	return defaultGenerateId()
}

// resolveId applies the §9a total resolution rule: a refs key resolves to
// its full id; anything else is a full id already.
func (imp *importer) resolveId(s string) string {
	if s == "" {
		return ""
	}
	if full, ok := imp.doc.Refs[s]; ok {
		return full
	}
	return s
}

func (imp *importer) resolveFormat(key string) (model.RelationFormat, bool) {
	return resolveFormatWith(imp.opts, key)
}

func (imp *importer) build() (model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	doc := imp.doc
	sbType := model.SmartBlockType_Page
	switch {
	case doc.Kind != "":
		sbType = kindNames.value(doc.Kind)
	case doc.Type == "template":
		sbType = model.SmartBlockType_Template
	}

	objectId := doc.Id
	if objectId == "" {
		objectId = imp.genId()
	}

	var objectTypes []string
	if doc.Type != "" {
		objectTypes = append(objectTypes, domain.TypeKey(doc.Type).URL())
		if doc.Type == "template" && doc.TemplateFor != "" {
			objectTypes = append(objectTypes, domain.TypeKey(doc.TemplateFor).URL())
		}
	}

	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[detailKeyId] = &types.Value{Kind: &types.Value_StringValue{StringValue: objectId}}
	for key, raw := range doc.Properties {
		if key == detailKeyId || key == detailKeyType {
			continue // lifted into the envelope; a stray copy must not leak
		}
		if v := imp.propertyValue(key, raw); v != nil {
			details.Fields[key] = v
		}
	}

	root := &model.Block{
		Id:      objectId,
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}
	if doc.Root != nil {
		if len(doc.Root.Fields) > 0 {
			root.Fields = jsonMapToProtoStruct(doc.Root.Fields)
		}
		root.BackgroundColor = doc.Root.BackgroundColor
	}

	all := []*model.Block{root}
	for _, jb := range doc.Blocks {
		if jb == nil {
			continue
		}
		// top-level structural blocks are absorbed or dropped (§7)
		switch jb.Type {
		case "title":
			imp.absorbIntoProperty(details, "name", jb.Text)
			continue
		case "description":
			imp.absorbIntoProperty(details, "description", jb.Text)
			continue
		case "featuredProperties":
			continue
		}
		blocks, err := imp.blockFromJSON(jb, "")
		if err != nil {
			return 0, nil, fmt.Errorf("build blocks: %w", err)
		}
		root.ChildrenIds = append(root.ChildrenIds, blocks[0].Id)
		all = append(all, blocks...)
	}

	snapshot := &model.SmartBlockSnapshotBase{
		Blocks:      all,
		Details:     details,
		ObjectTypes: objectTypes,
		Collections: imp.buildCollections(),
		Key:         doc.Key,
	}
	return sbType, snapshot, nil
}

// absorbIntoProperty merges a top-level title/description block's text into
// the matching property when unset (§7).
func (imp *importer) absorbIntoProperty(details *types.Struct, key, md string) {
	if md == "" {
		return
	}
	if existing := details.Fields[key]; existing.GetStringValue() != "" {
		return
	}
	plain, _, err := parseInline(md)
	if err != nil || plain == "" {
		return
	}
	details.Fields[key] = &types.Value{Kind: &types.Value_StringValue{StringValue: plain}}
}

func (imp *importer) buildCollections() *types.Struct {
	doc := imp.doc
	if len(doc.Items) == 0 && len(doc.Store) == 0 {
		return nil
	}
	coll := &types.Struct{Fields: map[string]*types.Value{}}
	for k, v := range doc.Store {
		coll.Fields[k] = jsonToProtoValue(v)
	}
	if len(doc.Items) > 0 {
		vals := make([]*types.Value, 0, len(doc.Items))
		for _, id := range doc.Items {
			vals = append(vals, &types.Value{Kind: &types.Value_StringValue{StringValue: imp.resolveId(id)}})
		}
		coll.Fields[storeKeyItems] = &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
	}
	return coll
}

// propertyValue decodes a property per its resolved format (§3). Scalars of
// list-shaped formats normalize to single-element lists (§11).
func (imp *importer) propertyValue(key string, v any) *types.Value {
	format, ok := imp.resolveFormat(key)
	if !ok {
		return jsonToProtoValue(v)
	}
	switch format {
	case model.RelationFormat_date:
		if s, isStr := v.(string); isStr {
			if sec, parsed := parseDate(s); parsed {
				return &types.Value{Kind: &types.Value_NumberValue{NumberValue: float64(sec)}}
			}
		}
	case model.RelationFormat_status, model.RelationFormat_tag:
		return wrapToList(mapJSONStrings(v, func(name string) string { return imp.optionId(key, name) }))
	case model.RelationFormat_object, model.RelationFormat_file:
		return wrapToList(mapJSONStrings(v, imp.resolveId))
	}
	return jsonToProtoValue(v)
}

func wrapToList(v *types.Value) *types.Value {
	if _, isList := v.GetKind().(*types.Value_ListValue); isList {
		return v
	}
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{v}}}}
}

//
// ---- blocks ----
//

// textStyleAliases extends the canonical inventory with the §5 input aliases.
var textStyleAliases = map[string]model.BlockContentTextStyle{
	"heading4": model.BlockContentText_Header3,
	"header4":  model.BlockContentText_Header3,
}

func (imp *importer) parseText(md string) (string, *model.BlockContentTextMarks, error) {
	if md == "" {
		return "", nil, nil
	}
	text, marks, err := parseInline(md)
	if err != nil {
		return "", nil, err
	}
	imp.resolveMarkTargets(marks)
	if len(marks) == 0 {
		return text, nil, nil
	}
	return text, &model.BlockContentTextMarks{Marks: marks}, nil
}

func (imp *importer) resolveMarkTargets(marks []*model.BlockContentTextMark) {
	for _, m := range marks {
		if m != nil && (m.Type == model.BlockContentTextMark_Mention || m.Type == model.BlockContentTextMark_Object) {
			m.Param = imp.resolveId(m.Param)
		}
	}
}

// textFromJSON builds a text-family block content (§5), applying the
// heading4/header4 aliases and the per-style prop rules.
func (imp *importer) textFromJSON(jb *jsonBlock) (*model.BlockContentText, error) {
	style, isAlias := textStyleAliases[jb.Type]
	if !isAlias {
		style = textStyleNames.value(jb.Type)
	}
	text, marks, err := imp.parseText(jb.Text)
	if err != nil {
		return nil, err
	}
	t := &model.BlockContentText{Style: style, Text: text, Marks: marks, Color: jb.Color}
	if style == model.BlockContentText_Checkbox {
		t.Checked = jb.Checked
	}
	if style == model.BlockContentText_Callout {
		t.IconEmoji = jb.IconEmoji
		t.IconImage = imp.resolveId(jb.IconImage)
	}
	return t, nil
}

// fileFromJSON builds a file-family block content (§5); state is recomputed,
// never serialized.
func (imp *importer) fileFromJSON(jb *jsonBlock) *model.BlockContentFile {
	f := &model.BlockContentFile{
		Type:           fileTypeNames.value(jb.Type),
		TargetObjectId: imp.resolveId(jb.ObjectId),
		Hash:           jb.Hash,
		Name:           jb.Name,
		Mime:           jb.MimeType,
		Size_:          jb.Size,
		Style:          fileStyleNames.value(jb.Style),
	}
	if jb.AddedAt != "" {
		if sec, ok := parseDate(jb.AddedAt); ok {
			f.AddedAt = sec
		}
	}
	if f.TargetObjectId != "" || f.Hash != "" {
		f.State = model.BlockContentFile_Done
	}
	return f
}

// bookmarkFromJSON builds a bookmark content; state is recomputed (§5).
func (imp *importer) bookmarkFromJSON(jb *jsonBlock) *model.BlockContentBookmark {
	bm := &model.BlockContentBookmark{
		Url:            jb.Url,
		TargetObjectId: imp.resolveId(jb.ObjectId),
	}
	if bm.TargetObjectId != "" {
		bm.State = model.BlockContentBookmark_Done
	}
	return bm
}

// linkFromJSON builds a link content, decoding the shown-property key list.
func (imp *importer) linkFromJSON(jb *jsonBlock) (*model.BlockContentLink, error) {
	var propKeys []string
	if len(jb.Properties) > 0 {
		if err := jsonUnmarshal(jb.Properties, &propKeys); err != nil {
			return nil, fmt.Errorf("link properties: %w", err)
		}
	}
	return &model.BlockContentLink{
		TargetBlockId: imp.resolveId(jb.ObjectId),
		CardStyle:     cardStyleNames.value(jb.CardStyle),
		IconSize:      iconSizeNames.value(jb.IconSize),
		Description:   linkDescriptionNames.value(jb.Description),
		Relations:     propKeys,
	}, nil
}

// blockFromJSON converts one block and its subtree; the returned slice is in
// pre-order with the block itself first. forcedId overrides the block id
// (used for derived table cell ids).
func (imp *importer) blockFromJSON(jb *jsonBlock, forcedId string) ([]*model.Block, error) {
	id := forcedId
	if id == "" {
		id = jb.Id
	}
	if id == "" {
		id = imp.genId()
	}
	b := &model.Block{Id: id}
	var extra []*model.Block
	withChildren := true
	liftedLang := ""

	switch {
	case jb.Type == "code":
		b.Content = &model.BlockContentOfText{Text: &model.BlockContentText{
			Style: model.BlockContentText_Code,
			Text:  jb.Text, // literal (§8.4)
		}}
		liftedLang = jb.Language
	case textStyleNames.has(jb.Type) || textStyleAliases[jb.Type] != 0:
		t, err := imp.textFromJSON(jb)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", id, err)
		}
		b.Content = &model.BlockContentOfText{Text: t}
	case fileTypeNames.has(jb.Type):
		b.Content = &model.BlockContentOfFile{File: imp.fileFromJSON(jb)}
		withChildren = false
	case jb.Type == "bookmark":
		b.Content = &model.BlockContentOfBookmark{Bookmark: imp.bookmarkFromJSON(jb)}
		withChildren = false
	case jb.Type == "link":
		link, err := imp.linkFromJSON(jb)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", id, err)
		}
		b.Content = &model.BlockContentOfLink{Link: link}
		withChildren = false
	case jb.Type == "divider":
		b.Content = &model.BlockContentOfDiv{Div: &model.BlockContentDiv{
			Style: divStyleNames.value(jb.Style),
		}}
		withChildren = false
	case jb.Type == "row":
		b.Content = &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row}}
	case jb.Type == "column":
		b.Content = &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column}}
	case jb.Type == "group":
		b.Content = &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div}}
	case jb.Type == "table":
		table, tExtra, err := imp.tableFromJSON(jb, id)
		if err != nil {
			return nil, err
		}
		b = table
		extra = tExtra
		withChildren = false
	case jb.Type == "embed" || jb.Type == "equation":
		processor := processorNames.value(jb.Processor)
		text := jb.Text
		if text == "" && !sourceProcessors[processor] {
			text = jb.Url // input alias for service processors (§5.2)
		}
		b.Content = &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{
			Text:      text,
			Processor: processor,
		}}
		withChildren = false
	case jb.Type == "tableOfContents":
		b.Content = &model.BlockContentOfTableOfContents{TableOfContents: &model.BlockContentTableOfContents{}}
		withChildren = false
	case jb.Type == "property":
		b.Content = &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: jb.Key}}
		withChildren = false
	case jb.Type == "dataview":
		dv, err := imp.dataviewFromJSON(jb)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", id, err)
		}
		b.Content = &model.BlockContentOfDataview{Dataview: dv}
		withChildren = false
	case jb.Type == "widget":
		b.Content = &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
			Layout:    widgetLayoutNames.value(jb.Layout),
			Limit:     jb.Limit,
			ViewId:    jb.ViewId,
			AutoAdded: jb.AutoAdded,
		}}
	case jb.Type == "chat":
		b.Content = &model.BlockContentOfChat{Chat: &model.BlockContentChat{}}
		withChildren = false
	case jb.Type == "featuredProperties":
		b.Content = &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}}
		withChildren = false
	case jb.Type == "icon":
		b.Content = &model.BlockContentOfIcon{Icon: &model.BlockContentIcon{Name: jb.Name}}
		withChildren = false
	default:
		return nil, fmt.Errorf("block %s: unknown type %q", id, jb.Type)
	}

	imp.applyBlockCommon(b, jb, liftedLang)
	if withChildren {
		childBlocks, err := imp.childrenFromJSON(b, jb.Children)
		if err != nil {
			return nil, err
		}
		extra = append(extra, childBlocks...)
	}
	return append([]*model.Block{b}, extra...), nil
}

// applyBlockCommon writes the shared block tail: align, verticalAlign,
// backgroundColor, fields, and the lifted code language (§4, §5.1).
func (imp *importer) applyBlockCommon(b *model.Block, jb *jsonBlock, liftedLang string) {
	b.Align = alignNames.value(jb.Align)
	b.VerticalAlign = verticalAlignNames.value(jb.VerticalAlign)
	b.BackgroundColor = jb.BackgroundColor
	if len(jb.Fields) > 0 {
		b.Fields = jsonMapToProtoStruct(jb.Fields)
	}
	if liftedLang != "" {
		if b.Fields == nil {
			b.Fields = &types.Struct{Fields: map[string]*types.Value{}}
		}
		b.Fields.Fields[codeLangField] = &types.Value{Kind: &types.Value_StringValue{StringValue: liftedLang}}
	}
}

// childrenFromJSON converts the children subtrees, appending their ids to
// the parent in document order.
func (imp *importer) childrenFromJSON(parent *model.Block, children []*jsonBlock) ([]*model.Block, error) {
	var extra []*model.Block
	for _, child := range children {
		if child == nil {
			continue
		}
		childBlocks, err := imp.blockFromJSON(child, "")
		if err != nil {
			return nil, err
		}
		parent.ChildrenIds = append(parent.ChildrenIds, childBlocks[0].Id)
		extra = append(extra, childBlocks...)
	}
	return extra, nil
}

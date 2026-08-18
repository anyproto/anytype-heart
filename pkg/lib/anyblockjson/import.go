package anyblockjson

// import.go reconstructs a snapshot from a validated AnyBlock JSON document
// (§2–§7, §9).

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

type jsonDoc struct {
	Schema string `json:"$schema"`
	// json.Number for the same reason as every other schema-integer field
	// below: 1.0 and 1e0 are integers to JSON Schema, so checkVersion accepts
	// them, and a plain int would then fail to decode a document Validate
	// declared valid.
	Version     json.Number         `json:"version"`
	Kind        string              `json:"kind"`
	Id          string              `json:"id"`
	Type        string              `json:"type"`
	TemplateFor string              `json:"template_for"`
	Key         string              `json:"key"`
	Properties  map[string]any      `json:"properties"`
	TypeProps   *[]jsonTypeProperty `json:"type_properties"` // pointer: [] and absent differ (§2a)
	Refs        map[string]string   `json:"refs"`
	// PropertyKeys is the §3 slug→stored-key legend: what this document says
	// its own key spellings mean, consulted before any vocabulary so a reader
	// without the space still lands on the right relation.
	PropertyKeys map[string]string `json:"property_keys"`
	Blocks       []*jsonBlock      `json:"blocks"`
	Items        []string          `json:"items"`
	Store        map[string]any    `json:"store"`
	Root         *jsonRootEscape   `json:"root"`
}

type jsonRootEscape struct {
	Fields          map[string]any `json:"fields"`
	BackgroundColor string         `json:"background_color"`
}

// jsonBlock is the union of every §5 block shape; the schema guarantees only
// type-appropriate fields are present.
type jsonBlock struct {
	// json.Number, not int, for every schema-integer field: JSON Schema counts
	// 2048.0 and 1e3 as integers, so Validate accepts them, and a typed int
	// field would then fail to decode a document Validate declared valid —
	// reported as a bare Go decode error with no JSON pointer, outside the
	// path-addressed error contract (§13). The schema bounds each field to the
	// stored type's range, so the conversion helpers cannot truncate.
	Indent json.Number `json:"indent"`
	Id     string      `json:"id"`
	Type   string      `json:"type"`

	Checked   bool   `json:"checked"`
	Color     string `json:"color"`
	Text      string `json:"text"`
	Language  string `json:"language"`
	IconEmoji string `json:"icon_emoji"`
	IconImage string `json:"icon_image"`

	ObjectId    string          `json:"object_id"`
	Name        string          `json:"name"`
	MimeType    string          `json:"mime_type"`
	Size        json.Number     `json:"size"`
	Style       string          `json:"style"`
	AddedAt     string          `json:"added_at"`
	Hash        string          `json:"hash"`
	Url         string          `json:"url"`
	CardStyle   string          `json:"card_style"`
	IconSize    string          `json:"icon_size"`
	Description string          `json:"description"`
	Properties  json.RawMessage `json:"properties"` // link: []string; dataview: []jsonDvProperty
	Processor   string          `json:"processor"`
	Key         string          `json:"key"`

	Layout    string      `json:"layout"`
	Limit     json.Number `json:"limit"`
	ViewId    string      `json:"view_id"`
	AutoAdded bool        `json:"auto_added"`

	Columns []jsonTableColumn `json:"columns"`
	Rows    []jsonTableRow    `json:"rows"`

	IsCollection bool       `json:"is_collection"`
	Source       []string   `json:"source"`
	Views        []jsonView `json:"views"`

	Align           string         `json:"align"`
	VerticalAlign   string         `json:"vertical_align"`
	BackgroundColor string         `json:"background_color"`
	Fields          map[string]any `json:"fields"`
}

// Unmarshal validates data and reconstructs a snapshot (§13). Errors wrap
// *ValidationError with JSON-path-addressed issues.
func Unmarshal(data []byte, opts Options) (model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	if _, err := validateToDoc(data, opts.NormalizeIndent, opts.OnWarning); err != nil {
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
	// usedIds is every id this document will contain: the ones it authored
	// (envelope, blocks, table rows and columns, derived cell ids) plus the
	// ones minted since. One set for all of them, because they end up in one
	// block graph — a generated id that lands on an authored one produces two
	// blocks with the same id, which validation has already finished checking
	// by the time it happens (§9).
	usedIds map[string]struct{}
}

// claimAuthoredIds records every id the document names before anything is
// generated. It has to run before the first genId call, which is why build
// calls it first rather than leaving it to the lazy path.
func (imp *importer) claimAuthoredIds() {
	imp.usedIds = map[string]struct{}{}
	var walk func(jbs []*jsonBlock)
	walk = func(jbs []*jsonBlock) {
		for _, jb := range jbs {
			if jb == nil {
				continue
			}
			imp.claimId(jb.Id)
			for _, col := range jb.Columns {
				imp.claimId(col.Id)
			}
			for _, row := range jb.Rows {
				imp.claimId(row.Id)
				for i, cell := range row.Cells {
					// a cell's id is derived (§6.1) but it is still a block id
					if row.Id != "" && i < len(jb.Columns) && jb.Columns[i].Id != "" {
						imp.claimId(row.Id + "-" + jb.Columns[i].Id)
					}
					if cell.Block != nil {
						walk([]*jsonBlock{cell.Block})
					}
					walk(cell.Blocks)
				}
			}
		}
	}
	imp.claimId(imp.doc.Id)
	walk(imp.doc.Blocks)
}

func (imp *importer) claimId(id string) string {
	if id == "" {
		return id
	}
	if imp.usedIds == nil {
		imp.claimAuthoredIds()
	}
	imp.usedIds[id] = struct{}{}
	return id
}

func (imp *importer) idTaken(id string) bool {
	if imp.usedIds == nil {
		imp.claimAuthoredIds()
	}
	_, taken := imp.usedIds[id]
	return taken
}

// genId mints an id no other id in the document uses. The generator belongs to
// the caller — the convert wiring derives ids from a file path and a counter,
// both halves author-controlled — so its answer is disambiguated rather than
// trusted.
func (imp *importer) genId() string {
	mint := defaultGenerateId
	if imp.opts.GenerateId != nil {
		mint = imp.opts.GenerateId
	}
	return imp.claimId(uniqueLabel(mint(), imp.idTaken))
}

// propertyKey inverts a key slot: the document's own legend first (§3), then
// the vocabulary in force. The legend wins because it is the only statement
// made by the document itself — a vocabulary belongs to the reader, and two
// readers disagreeing about a slug is exactly how a property ends up pointing
// at a different relation than it was exported from.
func (imp *importer) propertyKey(slug string) string {
	if key, ok := imp.doc.PropertyKeys[slug]; ok && key != "" {
		return key
	}
	return imp.opts.propertyKey(slug)
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

// declaredFormat maps a document's format name to a stored format. "text"
// is deliberately ambiguous (§3): it names both longtext and the legacy
// shorttext, so for that one name the property's *existing* format decides,
// which is what keeps a bundled short-text property (name, iconEmoji, …)
// from being rewritten to longtext on every round-trip. An absent or
// unrecognized name resolves the same way — it too lands on longtext. Every
// other name is taken literally: the document is authoritative about which
// format a property has, and only the text/text collapse needs repairing.
func (imp *importer) declaredFormat(key, name string) model.RelationFormat {
	f := formatNames.value(name)
	if f != model.RelationFormat_longtext {
		return f
	}
	if resolved, ok := imp.resolveFormat(key); ok && resolved == model.RelationFormat_shorttext {
		return resolved
	}
	return f
}

func (imp *importer) resolveFormat(key string) (model.RelationFormat, bool) {
	return resolveFormatWith(imp.opts, key)
}

func (imp *importer) build() (model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	doc := imp.doc
	imp.claimAuthoredIds()
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
		objectTypes = append(objectTypes, domain.TypeKey(imp.opts.typeKey(doc.Type)).URL())
		if doc.Type == "template" && doc.TemplateFor != "" {
			objectTypes = append(objectTypes, domain.TypeKey(imp.opts.typeKey(doc.TemplateFor)).URL())
		}
	}

	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[detailKeyId] = &types.Value{Kind: &types.Value_StringValue{StringValue: objectId}}
	// Sorted, and a REFUSAL when two spellings canonicalize onto one stored
	// key — the mirror of the export-side collapse guard (§8.38). Ranging the
	// map made "which of two spellings wins" a per-run coin flip: the same
	// request stored a different object run to run. The API layer refuses
	// first, with a better-worded message (canonicalizeDocumentKeys), but the
	// type-create channel skips it by design and so does every direct package
	// caller (cmd/anyblockroundtrip, cmd/anyblockrecover,
	// cmd/internal/anyblockbatch) — so the codec is the backstop.
	boundBy := make(map[string]string, len(doc.Properties))
	for _, slug := range sortedPropertySlugs(doc.Properties) {
		if slug == detailKeyId || slug == detailKeyType {
			continue // lifted into the envelope; a stray copy must not leak
		}
		// the document spells slugs (§7.5a); the store binds stored keys
		key := imp.propertyKey(slug)
		// admission runs on the FINAL resolved key, here at the seam where
		// details are written (§3). Validate already refused everything its
		// bundled chain could resolve, but a caller-supplied vocabulary can
		// bind a slug to a stored key the bundled table never knew — including
		// the internal keys the deny rule exists for — and Validate takes no
		// vocabulary, deliberately (§13).
		if reason, denied := deniedPropertyKey(key); denied {
			return 0, nil, &ValidationError{Issues: []Issue{{
				Path:    "/properties/" + escapeJSONPointer(slug),
				Message: reason,
			}}}
		}
		if first, dup := boundBy[key]; dup {
			return 0, nil, &ValidationError{Issues: []Issue{{
				Path:    "/properties/" + slug,
				Message: fmt.Sprintf("%q and %q both address property %q — keep one", first, slug, key),
			}}}
		}
		boundBy[key] = slug
		if v := imp.propertyValue(key, doc.Properties[slug]); v != nil {
			details.Fields[key] = v
		}
	}
	imp.applyTypeProperties(details)

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
	jbs, indents := imp.topLevelBlocks(details)
	blocks, err := imp.flatSubtree(jbs, indents, root, -1)
	if err != nil {
		return 0, nil, fmt.Errorf("build blocks: %w", err)
	}
	all = append(all, blocks...)

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
// list-shaped formats normalize to single-element lists (§11). An explicit
// null stays a null value — presence of the key is preserved (§3).
func (imp *importer) propertyValue(key string, v any) *types.Value {
	if v == nil {
		return &types.Value{Kind: &types.Value_NullValue{}}
	}
	// layout is named in the format, stored as a number (§3). A number is
	// still accepted so legacy documents keep importing unchanged.
	if isLayoutKey(key) {
		if s, isStr := v.(string); isStr && layoutNames.has(s) {
			return &types.Value{Kind: &types.Value_NumberValue{
				NumberValue: float64(layoutNames.value(s)),
			}}
		}
	}
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

// blockIndents extracts the effective indent of every entry, clamping per
// §4's lenient rule when NormalizeIndent is set (base is the indent of the
// run's implicit parent: −1 for the document, 0 for a cell's descendants).
// Clamps are silent here — validation already reported each one as a
// warning-grade issue on the same input, with the same rule.
func (imp *importer) blockIndents(jbs []*jsonBlock, base int) []int {
	indents := make([]int, len(jbs))
	for i, jb := range jbs {
		if jb == nil || jb.Indent == "" {
			continue
		}
		if v, ok := jsonIntValue(jb.Indent); ok {
			indents[i] = int(v)
		}
	}
	if imp.opts.NormalizeIndent {
		clampIndents(indents, base, nil)
	}
	return indents
}

// dataviewBlockId is the editor's fixed id for an object's *own* dataview
// (mirrors state.DataviewBlockID, which this package must not import, §12).
// Object types, sets and collections all reconstruct their primary dataview
// at this id and merge into an existing block rather than adding a second
// one, so a document that omits the id has to resolve to it (§7).
const dataviewBlockId = "dataview"

// pinPrimaryDataview gives the document's own dataview the editor's fixed id
// (§7). The primary dataview is the first indent-0 dataview block carrying
// neither an explicit id nor an objectId — an objectId means the block is an
// inline view of some *other* set or collection (§6.2) and keeps a generated
// id, as does any dataview nested below indent 0. A block that already claims
// the id anywhere in the document wins, so an explicit "id": "dataview" stays
// authoritative and no duplicate is minted (§13).
func (imp *importer) pinPrimaryDataview(raw []*jsonBlock, indents []int) {
	// anything already using the id wins, and "anything" means the whole
	// document: a table row named "dataview" is a block too, and it is not in
	// this array. Minting the id anyway produced a duplicate *after*
	// validation had passed, and the re-export then dropped the table body
	// whole when the id resolved to the wrong block.
	if imp.idTaken(dataviewBlockId) {
		return
	}
	for i, jb := range raw {
		if jb == nil || indents[i] != 0 {
			continue
		}
		if jb.Type != "dataview" || jb.Id != "" || jb.ObjectId != "" {
			continue
		}
		jb.Id = imp.claimId(dataviewBlockId)
		return
	}
}

// topLevelBlocks resolves the document's blocks array for the tree rebuild:
// structural blocks at indent 0 are absorbed into properties or dropped,
// together with their whole subtree (§7 — matching the nested encoding, which
// never descended into them).
func (imp *importer) topLevelBlocks(details *types.Struct) ([]*jsonBlock, []int) {
	raw := imp.doc.Blocks
	indents := imp.blockIndents(raw, -1)
	imp.pinPrimaryDataview(raw, indents)
	jbs := make([]*jsonBlock, 0, len(raw))
	kept := make([]int, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		jb := raw[i]
		if jb == nil {
			continue
		}
		// structuralBlockTypes (blockvocab.go) is the one statement of which
		// types these are — the API surfaces that publish an authorable
		// vocabulary read the same map
		if indents[i] == 0 && structuralBlockTypes[jb.Type] {
			switch jb.Type {
			case "title":
				imp.absorbIntoProperty(details, "name", jb.Text)
			case "description":
				imp.absorbIntoProperty(details, "description", jb.Text)
			}
			for i+1 < len(raw) && indents[i+1] > 0 {
				i++
			}
			continue
		}
		jbs = append(jbs, jb)
		kept = append(kept, indents[i])
	}
	return jbs, kept
}

type stackEntry struct {
	b      *model.Block
	indent int
}

// flatSubtree rebuilds the tree from a flat pre-order run (§4 F6): walk with
// a stack seeded (root, rootIndent); a block at indent k attaches to the
// nearest stack entry shallower than k. Validation guarantees indents are
// monotone (or already clamped), so that entry is exactly at k−1.
func (imp *importer) flatSubtree(jbs []*jsonBlock, indents []int, root *model.Block, rootIndent int) ([]*model.Block, error) {
	var all []*model.Block
	stack := []stackEntry{{root, rootIndent}}
	for i, jb := range jbs {
		if jb == nil {
			continue
		}
		blocks, err := imp.blockFromJSON(jb, "")
		if err != nil {
			return nil, err
		}
		k := indents[i]
		for len(stack) > 1 && stack[len(stack)-1].indent >= k {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].b
		parent.ChildrenIds = append(parent.ChildrenIds, blocks[0].Id)
		all = append(all, blocks...)
		stack = append(stack, stackEntry{blocks[0], k})
	}
	return all, nil
}

// textStyleAliases extends the canonical inventory with the §5 input aliases.
var textStyleAliases = map[string]model.BlockContentTextStyle{
	"heading_4": model.BlockContentText_Header3,
	"header_4":  model.BlockContentText_Header3,
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
		Size_:          jsonInt64(jb.Size),
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
		Relations:     imp.opts.propertyKeys(propKeys),
	}, nil
}

// blockFromJSON converts one block; the returned slice has the block first,
// followed by any internal blocks it owns (the table subtree). Document
// children are attached by the flatSubtree stack rebuild, not here. forcedId
// overrides the block id (used for derived table cell ids).
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
	case jb.Type == "bookmark":
		b.Content = &model.BlockContentOfBookmark{Bookmark: imp.bookmarkFromJSON(jb)}
	case jb.Type == "link":
		link, err := imp.linkFromJSON(jb)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", id, err)
		}
		b.Content = &model.BlockContentOfLink{Link: link}
	case jb.Type == "divider":
		b.Content = &model.BlockContentOfDiv{Div: &model.BlockContentDiv{
			Style: divStyleNames.value(jb.Style),
		}}
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
	case jb.Type == "table_of_contents":
		b.Content = &model.BlockContentOfTableOfContents{TableOfContents: &model.BlockContentTableOfContents{}}
	case jb.Type == "property":
		b.Content = &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: imp.propertyKey(jb.Key)}}
	case jb.Type == "dataview":
		dv, err := imp.dataviewFromJSON(jb)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", id, err)
		}
		b.Content = &model.BlockContentOfDataview{Dataview: dv}
	case jb.Type == "widget":
		b.Content = &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
			Layout:    widgetLayoutNames.value(jb.Layout),
			Limit:     jsonInt32(jb.Limit),
			ViewId:    jb.ViewId,
			AutoAdded: jb.AutoAdded,
		}}
	case jb.Type == "chat":
		b.Content = &model.BlockContentOfChat{Chat: &model.BlockContentChat{}}
	case jb.Type == "featured_properties":
		b.Content = &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}}
	case jb.Type == "icon":
		b.Content = &model.BlockContentOfIcon{Icon: &model.BlockContentIcon{Name: jb.Name}}
	default:
		return nil, fmt.Errorf("block %s: unknown type %q", id, jb.Type)
	}

	imp.applyBlockCommon(b, jb, liftedLang)
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

// sortedPropertySlugs returns the document's property spellings in a fixed
// order, so which of two colliding spellings the refusal names — and which
// value a non-colliding document binds — never depends on map iteration.
func sortedPropertySlugs(props map[string]any) []string {
	out := make([]string, 0, len(props))
	for slug := range props {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

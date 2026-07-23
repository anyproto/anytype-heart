package anyblockjson

// export.go serializes a snapshot into canonical AnyBlock JSON (§2–§7,
// §9–§9a).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// FormatResolver reports the format of a property key, when known. Bundle
// properties are resolved internally; the resolver covers custom keys (§3).
type FormatResolver func(key domain.RelationKey) (model.RelationFormat, bool)

// OptionResolver maps select/multiSelect option ids to names on export and
// names to ids on import (creating options is the import wiring's job, §3).
type OptionResolver interface {
	OptionName(key domain.RelationKey, id string) (string, bool)
	OptionId(key domain.RelationKey, name string) (string, bool)
}

// Options configures Marshal and Unmarshal (§13).
type Options struct {
	ResolveFormat     FormatResolver   // optional; nil = bundle-only resolution (§3)
	ResolveOptions    OptionResolver   // optional; nil = option values pass through as ids
	ResolveProperties PropertyResolver // optional; nil = type documents keep raw recommended-relation ids (§2a)
	OmitIds           bool             // export only: drop every id (§9)
	CompactIds        bool             // export only: shorten ids, emit refs legend (§9a)
	GenerateId        func() string    // import only: id generator for missing ids; nil = random 24-hex
	NormalizeIndent   bool             // import only: clamp over-deep indents instead of rejecting (§4)
	OnWarning         func(Issue)      // optional sink for warning-grade issues (NormalizeIndent clamps)
}

const (
	compactIdMinLen = 5

	// well-known internal keys that get lifted into the envelope
	detailKeyId   = "id"
	detailKeyType = "type"
	storeKeyItems = "objects"
	// codeLangField is the internal fields key holding a code block's
	// language (§5.1)
	codeLangField = "lang"
)

// propertiesKeptOnExport are the internal properties the importer
// meaningfully preserves; everything else in LocalAndDerivedRelationKeys is
// stripped (§3).
var propertiesKeptOnExport = map[string]bool{
	"createdDate":      true,
	"lastModifiedDate": true,
	"creator":          true,
	"isFavorite":       true,
	"isArchived":       true,
	"resolvedLayout":   true,
}

// wellKnownPropertyOrder puts the §3 magic keys first in the properties
// object; all remaining keys follow alphabetically (canonical order
// decision).
var wellKnownPropertyOrder = []string{"name", "description", "iconEmoji", "iconImage"}

// Marshal serializes a snapshot into canonical AnyBlock JSON (§13).
func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("nil snapshot")
	}
	e := &exporter{opts: opts, snapshot: snapshot, sbType: sbType, blocks: map[string]*model.Block{}, visited: map[string]bool{}}
	e.indexBlocks()
	if opts.CompactIds {
		e.buildCompactIds()
	}
	doc, err := e.buildDoc(sbType)
	if err != nil {
		return nil, fmt.Errorf("build document: %w", err)
	}
	return marshalCanonical(doc)
}

type exporter struct {
	opts     Options
	snapshot *model.SmartBlockSnapshotBase
	sbType   model.SmartBlockType
	blocks   map[string]*model.Block
	rootId   string
	visited  map[string]bool // emitted block ids: breaks ChildrenIds cycles, dedupes shared children

	objectRefs map[string]string // full object id -> refs label (§9a)
	localIds   map[string]string // block/row/column/view id -> short label
}

func (e *exporter) detail(key string) *types.Value {
	if e.snapshot.Details == nil {
		return nil
	}
	return e.snapshot.Details.Fields[key]
}

func (e *exporter) objectId() string {
	return e.detail(detailKeyId).GetStringValue()
}

func (e *exporter) indexBlocks() {
	children := map[string]bool{}
	for _, b := range e.snapshot.Blocks {
		if b == nil || b.Id == "" {
			continue
		}
		e.blocks[b.Id] = b
		for _, c := range b.ChildrenIds {
			children[c] = true
		}
	}
	// the root block's id equals the object id (§2); fall back to the first
	// block nobody references
	if _, ok := e.blocks[e.objectId()]; ok {
		e.rootId = e.objectId()
		return
	}
	for _, b := range e.snapshot.Blocks {
		if b != nil && b.Id != "" && !children[b.Id] {
			e.rootId = b.Id
			return
		}
	}
}

// typeKeyIdPrefix is the "ot-" prefix ObjectTypes entries carry.
var typeKeyIdPrefix = domain.TypeKey("").URL()

func (e *exporter) typeKeys() []string {
	keys := make([]string, 0, len(e.snapshot.ObjectTypes))
	for _, t := range e.snapshot.ObjectTypes {
		keys = append(keys, strings.TrimPrefix(t, typeKeyIdPrefix))
	}
	return keys
}

func (e *exporter) buildDoc(sbType model.SmartBlockType) (*omap, error) {
	doc := &omap{}
	doc.set("$schema", SchemaURL)
	doc.set("version", FormatVersion)

	typeKeys := e.typeKeys()
	typeKey := ""
	if len(typeKeys) > 0 {
		typeKey = typeKeys[0]
	}

	// kind is omitted whenever derivable (§2). A Page whose type key is
	// "template" must keep its explicit kind, or import would derive
	// Template from the type.
	derivable := (sbType == model.SmartBlockType_Page && typeKey != "template") ||
		(sbType == model.SmartBlockType_Template && typeKey == "template")
	if !derivable {
		name := kindNames.name(sbType)
		if name == "" {
			return nil, fmt.Errorf("smartblock type %v has no kind mapping", sbType)
		}
		doc.set("kind", name)
	}

	doc.setNonEmpty("id", e.objectId())
	doc.setNonEmpty("type", typeKey)
	if typeKey == "template" && len(typeKeys) > 1 {
		doc.setNonEmpty("templateFor", typeKeys[1])
	}
	doc.setNonEmpty("key", e.snapshot.Key)
	doc.setNonEmpty("properties", e.buildProperties())
	if tp := e.buildTypeProperties(); tp != nil {
		doc.set("typeProperties", tp) // present even when empty (§2a)
	}

	if len(e.objectRefs) > 0 {
		refs := &omap{}
		type kv struct{ label, id string }
		var entries []kv
		for id, label := range e.objectRefs {
			entries = append(entries, kv{label, id})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].label < entries[j].label })
		for _, en := range entries {
			refs.set(en.label, en.id)
		}
		doc.set("refs", refs)
	}

	blocks, err := e.buildBlocks()
	if err != nil {
		return nil, err
	}
	doc.setNonEmpty("blocks", blocks)

	items, store := e.buildStore()
	doc.setNonEmpty("items", items)
	doc.setNonEmpty("store", store)
	doc.setNonEmpty("root", e.buildRootEscape())
	return doc, nil
}

func (e *exporter) buildStore() ([]any, *omap) {
	coll := e.snapshot.Collections
	if coll == nil || len(coll.Fields) == 0 {
		return nil, nil
	}
	// the objects key lifts into items only when it is a list; any other
	// shape stays in store so nothing is silently dropped
	var items []any
	objectsLifted := false
	if v := coll.Fields[storeKeyItems]; v != nil {
		if lv, ok := v.GetKind().(*types.Value_ListValue); ok {
			objectsLifted = true
			for _, el := range lv.ListValue.GetValues() {
				if id := el.GetStringValue(); id != "" {
					items = append(items, e.compactObjectId(id))
				}
			}
		}
	}
	store := &omap{}
	keys := make([]string, 0, len(coll.Fields))
	for k := range coll.Fields {
		if k != storeKeyItems || !objectsLifted {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		store.set(k, protoValueToJSON(coll.Fields[k]))
	}
	return items, store
}

func (e *exporter) buildRootEscape() *omap {
	root := e.blocks[e.rootId]
	if root == nil {
		return nil
	}
	m := &omap{}
	if root.Fields != nil && len(root.Fields.Fields) > 0 {
		m.set("fields", protoStructToJSON(root.Fields))
	}
	m.setNonEmpty("backgroundColor", root.BackgroundColor)
	return m
}

//
// ---- properties ----
//

// strippedDetailKeys are the internal/derived properties export removes (§3).
func strippedDetailKeys() map[string]bool {
	stripped := map[string]bool{detailKeyId: true, detailKeyType: true}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		if !propertiesKeptOnExport[string(k)] {
			stripped[string(k)] = true
		}
	}
	return stripped
}

func (e *exporter) buildProperties() *omap {
	if e.snapshot.Details == nil {
		return nil
	}
	stripped := strippedDetailKeys()
	lifted := e.typePropDetailKeys()
	var keys []string
	for k := range e.snapshot.Details.Fields {
		if !stripped[k] && !lifted[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	ordered := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, wk := range wellKnownPropertyOrder {
		for _, k := range keys {
			if k == wk {
				ordered = append(ordered, k)
				seen[k] = true
			}
		}
	}
	for _, k := range keys {
		if !seen[k] {
			ordered = append(ordered, k)
		}
	}
	m := &omap{}
	for _, k := range ordered {
		// presence of a property key is meaningful — it records that the
		// property was set on the object — so values are written verbatim,
		// including empty and default ones (§3); the omit-empty canon applies
		// to block attributes and envelope fields only
		m.set(k, e.propertyValue(k, e.snapshot.Details.Fields[k]))
	}
	return m
}

func (e *exporter) resolveFormat(key string) (model.RelationFormat, bool) {
	return resolveFormatWith(e.opts, key)
}

// resolveFormatWith applies the §3 resolution order: bundle first, then the
// caller's resolver.
func resolveFormatWith(opts Options, key string) (model.RelationFormat, bool) {
	if f, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
		return f, true
	}
	if opts.ResolveFormat != nil {
		return opts.ResolveFormat(domain.RelationKey(key))
	}
	return 0, false
}

func (e *exporter) propertyValue(key string, v *types.Value) any {
	format, ok := e.resolveFormat(key)
	if !ok {
		return protoValueToJSON(v)
	}
	switch format {
	case model.RelationFormat_date:
		if n, isNum := v.GetKind().(*types.Value_NumberValue); isNum {
			return formatDate(int64(n.NumberValue))
		}
	case model.RelationFormat_status, model.RelationFormat_tag:
		var out []any
		for _, id := range valueStringList(v) {
			out = append(out, e.optionName(key, id))
		}
		return out
	case model.RelationFormat_object, model.RelationFormat_file:
		var out []any
		for _, id := range valueStringList(v) {
			out = append(out, e.compactObjectId(id))
		}
		return out
	}
	return protoValueToJSON(v)
}

func (e *exporter) optionName(key, id string) string {
	if e.opts.ResolveOptions != nil {
		if name, ok := e.opts.ResolveOptions.OptionName(domain.RelationKey(key), id); ok {
			return name
		}
	}
	return id
}

// valueStringList reads a value as a list of strings, accepting the single
// string form.
func valueStringList(v *types.Value) []string {
	if s := v.GetStringValue(); s != "" {
		return []string{s}
	}
	var out []string
	for _, el := range v.GetListValue().GetValues() {
		if s := el.GetStringValue(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

//
// ---- blocks ----
//

// orEmpty substitutes an empty message for a nil one (proto semantics: a nil
// message equals its zero value).
func orEmpty[T any](p *T) *T {
	if p == nil {
		return new(T)
	}
	return p
}

// isStructural reports blocks that are derivable and dropped on export (§7).
func isStructural(b *model.Block) bool {
	switch c := b.Content.(type) {
	case *model.BlockContentOfLayout:
		return orEmpty(c.Layout).Style == model.BlockContentLayout_Header
	case *model.BlockContentOfText:
		style := orEmpty(c.Text).Style
		return style == model.BlockContentText_Title ||
			style == model.BlockContentText_Description
	case *model.BlockContentOfFeaturedRelations:
		return true
	}
	return false
}

func (e *exporter) buildBlocks() ([]any, error) {
	root := e.blocks[e.rootId]
	if root == nil {
		return nil, nil
	}
	var out []any
	if err := e.appendBlocksFlat(&out, root.ChildrenIds, 0, true); err != nil {
		return nil, err
	}
	return out, nil
}

// appendBlocksFlat walks a subtree in pre-order and appends each block to out
// with its depth as the indent field — the flat encoding (§4 F1–F2). A block
// dropped by blockToJSON (structural, visited, content-less leaf) drops its
// whole subtree, matching the nested encoding's semantics.
func (e *exporter) appendBlocksFlat(out *[]any, ids []string, depth int, topLevel bool) error {
	for _, id := range ids {
		b := e.blocks[id]
		if b == nil {
			continue
		}
		if topLevel && isStructural(b) {
			continue
		}
		m, withChildren, err := e.blockToJSON(b, depth)
		if err != nil {
			return err
		}
		if m == nil {
			continue
		}
		*out = append(*out, m)
		if withChildren {
			if err := e.appendBlocksFlat(out, b.ChildrenIds, depth+1, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *exporter) localId(id string) string {
	if e.opts.CompactIds {
		if short, ok := e.localIds[id]; ok {
			return short
		}
	}
	return id
}

// blockToJSON renders one block at the given depth (its indent, written
// first per the §4 canonical key order). The returned bool reports whether
// the caller should descend into the block's children.
func (e *exporter) blockToJSON(b *model.Block, depth int) (*omap, bool, error) {
	// a snapshot's block graph is untrusted: without this, a ChildrenIds
	// cycle recurses to an unrecoverable stack overflow, and a block shared
	// by two parents is emitted twice (duplicate ids fail validation)
	if b.Id != "" {
		if e.visited[b.Id] {
			return nil, false, nil
		}
		e.visited[b.Id] = true
	}
	m := &omap{}
	m.setNonEmpty("indent", depth)
	if !e.opts.OmitIds {
		m.setNonEmpty("id", e.localId(b.Id))
	}
	liftedFields := map[string]bool{}
	withChildren := true

	// nil inner messages are proto-equivalent to empty ones — never panic
	switch c := b.Content.(type) {
	case nil:
		// legacy content-less blocks exist in old accounts: relation objects
		// carry a bare wrapper around their "used in" dataview, and pages can
		// hold orphaned empty leaves. A childless one is dropped; one with
		// children exports as a transparent group so the subtree survives.
		if len(b.ChildrenIds) == 0 {
			return nil, false, nil
		}
		m.set("type", "group")
	case *model.BlockContentOfText:
		if err := e.textToJSON(m, b, orEmpty(c.Text), liftedFields); err != nil {
			return nil, false, err
		}
	case *model.BlockContentOfFile:
		// file blocks are leaves in the editor, but legacy data holds real
		// text children under them — dropping those would be silent loss
		e.fileToJSON(m, orEmpty(c.File))
	case *model.BlockContentOfBookmark:
		bm := orEmpty(c.Bookmark)
		m.set("type", "bookmark")
		m.setNonEmpty("url", bm.Url)
		m.setNonEmpty("objectId", e.compactObjectId(bm.TargetObjectId))
		withChildren = false
	case *model.BlockContentOfLink:
		l := orEmpty(c.Link)
		m.set("type", "link")
		m.setNonEmpty("objectId", e.compactObjectId(l.TargetBlockId))
		if l.CardStyle != model.BlockContentLink_Text {
			m.setNonEmpty("cardStyle", cardStyleNames.name(l.CardStyle))
		}
		if l.IconSize != model.BlockContentLink_SizeNone {
			m.setNonEmpty("iconSize", iconSizeNames.name(l.IconSize))
		}
		if l.Description != model.BlockContentLink_None {
			m.setNonEmpty("description", linkDescriptionNames.name(l.Description))
		}
		m.setNonEmpty("properties", stringsToAny(l.Relations))
		withChildren = false
	case *model.BlockContentOfDiv:
		m.set("type", "divider")
		if style := orEmpty(c.Div).Style; style != model.BlockContentDiv_Line {
			m.setNonEmpty("style", divStyleNames.name(style))
		}
		withChildren = false
	case *model.BlockContentOfLayout:
		switch orEmpty(c.Layout).Style {
		case model.BlockContentLayout_Row:
			m.set("type", "row")
		case model.BlockContentLayout_Column:
			m.set("type", "column")
		case model.BlockContentLayout_Div:
			m.set("type", "group")
		default:
			// header and stray table wrappers are structural (§7)
			return nil, false, nil
		}
	case *model.BlockContentOfTable:
		if err := e.tableToJSON(m, b); err != nil {
			return nil, false, err
		}
		withChildren = false
	case *model.BlockContentOfLatex:
		lx := orEmpty(c.Latex)
		m.set("type", "embed")
		if lx.Processor != model.BlockContentLatex_Latex {
			m.setNonEmpty("processor", processorNames.name(lx.Processor))
		}
		m.setNonEmpty("text", lx.Text)
		withChildren = false
	case *model.BlockContentOfTableOfContents:
		m.set("type", "tableOfContents")
		withChildren = false
	case *model.BlockContentOfRelation:
		m.set("type", "property")
		m.setNonEmpty("key", orEmpty(c.Relation).Key)
		withChildren = false
	case *model.BlockContentOfDataview:
		if err := e.dataviewToJSON(m, orEmpty(c.Dataview)); err != nil {
			return nil, false, err
		}
		withChildren = false
	case *model.BlockContentOfWidget:
		w := orEmpty(c.Widget)
		m.set("type", "widget")
		if w.Layout != model.BlockContentWidget_Link {
			m.setNonEmpty("layout", widgetLayoutNames.name(w.Layout))
		}
		m.setNonEmpty("limit", w.Limit)
		m.setNonEmpty("viewId", w.ViewId)
		m.setNonEmpty("autoAdded", w.AutoAdded)
	case *model.BlockContentOfChat:
		m.set("type", "chat")
		withChildren = false
	case *model.BlockContentOfFeaturedRelations:
		m.set("type", "featuredProperties")
		withChildren = false
	case *model.BlockContentOfIcon:
		m.set("type", "icon")
		m.setNonEmpty("name", orEmpty(c.Icon).Name)
		withChildren = false
	case *model.BlockContentOfSmartblock:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("block %s: content type %T has no JSON mapping", b.Id, b.Content)
	}

	e.finishBlockJSON(m, b, liftedFields)
	return m, withChildren, nil
}

// finishBlockJSON writes the common block tail: align, verticalAlign,
// backgroundColor, fields — in the §4 canonical order.
func (e *exporter) finishBlockJSON(m *omap, b *model.Block, liftedFields map[string]bool) {
	if b.Align != model.Block_AlignLeft {
		m.setNonEmpty("align", alignNames.name(b.Align))
	}
	if b.VerticalAlign != model.Block_VerticalAlignTop {
		m.setNonEmpty("verticalAlign", verticalAlignNames.name(b.VerticalAlign))
	}
	m.setNonEmpty("backgroundColor", b.BackgroundColor)
	m.setNonEmpty("fields", e.fieldsToJSON(b.Fields, liftedFields))
}

func (e *exporter) fieldsToJSON(fields *types.Struct, lifted map[string]bool) *omap {
	if fields == nil || len(fields.Fields) == 0 {
		return nil
	}
	m := &omap{}
	keys := make([]string, 0, len(fields.Fields))
	for k := range fields.Fields {
		if !lifted[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.set(k, protoValueToJSON(fields.Fields[k]))
	}
	return m
}

func (e *exporter) textToJSON(m *omap, b *model.Block, t *model.BlockContentText, liftedFields map[string]bool) error {
	style := t.Style
	// deprecated Header4 exports as heading3 (§5)
	if style == model.BlockContentText_Header4 {
		style = model.BlockContentText_Header3
	}
	typ := textStyleNames.name(style)
	if typ == "" {
		return fmt.Errorf("block %s: text style %v has no JSON mapping", b.Id, t.Style)
	}
	m.set("type", typ)

	if style == model.BlockContentText_Checkbox {
		m.setNonEmpty("checked", t.Checked)
	}
	if style == model.BlockContentText_Callout {
		m.setNonEmpty("iconEmoji", t.IconEmoji)
		m.setNonEmpty("iconImage", e.compactObjectId(t.IconImage))
	}
	if style == model.BlockContentText_Code {
		if b.Fields != nil {
			if lang := b.Fields.Fields[codeLangField].GetStringValue(); lang != "" {
				m.set("language", lang)
				liftedFields[codeLangField] = true
			}
		}
		// literal text; stored marks and color dropped (§8.4, §11)
		m.setNonEmpty("text", t.Text)
		return nil
	}
	m.setNonEmpty("color", t.Color)
	m.setNonEmpty("text", renderInline(t.Text, e.compactMarks(t.Marks.GetMarks())))
	return nil
}

// compactMarks rewrites mention/object mark targets through the refs legend
// without mutating the snapshot (§9a).
func (e *exporter) compactMarks(marks []*model.BlockContentTextMark) []*model.BlockContentTextMark {
	if !e.opts.CompactIds || len(marks) == 0 {
		return marks
	}
	out := make([]*model.BlockContentTextMark, 0, len(marks))
	for _, mk := range marks {
		switch {
		case mk == nil || mk.Param == "":
			out = append(out, mk)
		case mk.Type == model.BlockContentTextMark_Mention || mk.Type == model.BlockContentTextMark_Object:
			clone := *mk
			clone.Param = e.compactObjectId(mk.Param)
			out = append(out, &clone)
		case mk.Type == model.BlockContentTextMark_Link && strings.HasPrefix(mk.Param, objectLinkPrefix):
			// rendered as an Object mark (§8.3), so its target compacts too
			clone := *mk
			clone.Param = objectLinkPrefix + e.compactObjectId(strings.TrimPrefix(mk.Param, objectLinkPrefix))
			out = append(out, &clone)
		default:
			out = append(out, mk)
		}
	}
	return out
}

func (e *exporter) fileToJSON(m *omap, f *model.BlockContentFile) {
	typ := fileTypeNames.name(f.Type)
	if typ == "" {
		typ = "file" // Type_None (§5)
	}
	m.set("type", typ)
	objectId := f.TargetObjectId
	if objectId == "" {
		objectId = f.Hash // legacy content address migrates to objectId
	}
	m.setNonEmpty("objectId", e.compactObjectId(objectId))
	m.setNonEmpty("name", f.Name)
	m.setNonEmpty("mimeType", f.Mime)
	m.setNonEmpty("size", f.Size_)
	if f.Style != model.BlockContentFile_Auto {
		m.setNonEmpty("style", fileStyleNames.name(f.Style))
	}
	if f.AddedAt != 0 {
		m.set("addedAt", formatDate(f.AddedAt))
	}
}

func stringsToAny(ss []string) []any {
	var out []any
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

//
// ---- compact ids (§9a) ----
//

func (e *exporter) compactObjectId(id string) string {
	if e.opts.CompactIds && id != "" {
		if label, ok := e.objectRefs[id]; ok {
			return label
		}
	}
	return id
}

// buildCompactIds pre-collects every referenced object id (for the refs
// legend) and every doc-local block/row/column/view id (for suffix
// relabeling).
func (e *exporter) buildCompactIds() {
	objects := map[string]bool{}
	locals := map[string]bool{}
	addObject := func(id string) {
		if id != "" {
			objects[id] = true
		}
	}

	for _, b := range e.snapshot.Blocks {
		if b == nil {
			continue
		}
		if b.Id != "" {
			locals[b.Id] = true
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfText:
			t := orEmpty(c.Text)
			for _, mk := range t.Marks.GetMarks() {
				if mk == nil {
					continue
				}
				switch {
				case mk.Type == model.BlockContentTextMark_Mention || mk.Type == model.BlockContentTextMark_Object:
					addObject(mk.Param)
				case mk.Type == model.BlockContentTextMark_Link && strings.HasPrefix(mk.Param, objectLinkPrefix):
					// normalizes to an Object mark on render (§8.3)
					addObject(strings.TrimPrefix(mk.Param, objectLinkPrefix))
				}
			}
			addObject(t.IconImage)
		case *model.BlockContentOfFile:
			f := orEmpty(c.File)
			if f.TargetObjectId != "" {
				addObject(f.TargetObjectId)
			} else {
				addObject(f.Hash)
			}
		case *model.BlockContentOfBookmark:
			addObject(orEmpty(c.Bookmark).TargetObjectId)
		case *model.BlockContentOfLink:
			addObject(orEmpty(c.Link).TargetBlockId)
		case *model.BlockContentOfDataview:
			dv := orEmpty(c.Dataview)
			addObject(dv.TargetObjectId)
			for _, v := range dv.Views {
				if v == nil {
					continue
				}
				locals[v.Id] = true
				addObject(v.DefaultTemplateId)
				addObject(v.DefaultObjectTypeId)
				for _, f := range flattenFilters(v.Filters) {
					if format, ok := e.dvFormat(dv, f.RelationKey); ok &&
						(format == model.RelationFormat_object || format == model.RelationFormat_file) {
						for _, id := range valueStringList(f.Value) {
							addObject(id)
						}
					}
				}
				for _, s := range v.Sorts {
					if s == nil {
						continue
					}
					if format, ok := e.dvFormat(dv, s.RelationKey); ok &&
						(format == model.RelationFormat_object || format == model.RelationFormat_file) {
						for _, cv := range s.CustomOrder {
							for _, id := range valueStringList(cv) {
								addObject(id)
							}
						}
					}
				}
			}
			for _, oo := range dv.ObjectOrders {
				if oo == nil {
					continue
				}
				for _, id := range oo.ObjectIds {
					addObject(id)
				}
			}
		}
	}

	if e.snapshot.Details != nil {
		stripped := strippedDetailKeys()
		lifted := e.typePropDetailKeys()
		for key, v := range e.snapshot.Details.Fields {
			if stripped[key] || lifted[key] {
				continue // stripped/lifted properties never appear as ids, so no legend entry
			}
			format, ok := e.resolveFormat(key)
			if ok && (format == model.RelationFormat_object || format == model.RelationFormat_file) {
				for _, id := range valueStringList(v) {
					addObject(id)
				}
			}
		}
	}
	if e.snapshot.Collections != nil {
		if v := e.snapshot.Collections.Fields[storeKeyItems]; v != nil {
			for _, el := range v.GetListValue().GetValues() {
				addObject(el.GetStringValue())
			}
		}
	}
	// the envelope id is never compacted (§9a)
	delete(objects, e.objectId())

	// refs keys must not equal a full id present in the document (§9a); the
	// avoid set covers every id this export knows about
	fullIds := map[string]bool{e.objectId(): true}
	for id := range objects {
		fullIds[id] = true
	}
	for id := range locals {
		fullIds[id] = true
	}
	e.objectRefs = suffixLabels(setToSlice(objects), compactIdMinLen, func(candidate string) bool {
		return fullIds[candidate] || !isValidRefsKey(candidate)
	})
	// local relabels stay dash-free: '-' is the derived-cell-id separator
	// and forbidden in row/column ids (§6.1)
	e.localIds = suffixLabels(setToSlice(locals), compactIdMinLen, isInvalidLocalLabel)
	// short ids label as themselves; drop those the schema charsets reject
	dropInvalidLabels(e.objectRefs, isValidRefsKey)
	dropInvalidLabels(e.localIds, func(label string) bool { return !isInvalidLocalLabel(label) })
}

// isValidRefsKey reports whether s matches the schema's refs-key pattern
// ^[A-Za-z0-9_-]{1,64}$.
func isValidRefsKey(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !isLabelRune(r) && r != '-' {
			return false
		}
	}
	return true
}

// isInvalidLocalLabel rejects relabel candidates for blocks/rows/columns/
// views: the row/column charset has no dash (§6.1), which also keeps labels
// clear of derived cell ids.
func isInvalidLocalLabel(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return true
	}
	for _, r := range s {
		if !isLabelRune(r) {
			return true
		}
	}
	return false
}

func isLabelRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func dropInvalidLabels(labels map[string]string, valid func(string) bool) {
	for id, label := range labels {
		if !valid(label) {
			delete(labels, id)
		}
	}
}

func flattenFilters(filters []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	var out []*model.BlockContentDataviewFilter
	var walk func([]*model.BlockContentDataviewFilter)
	walk = func(fs []*model.BlockContentDataviewFilter) {
		for _, f := range fs {
			if f == nil {
				continue
			}
			out = append(out, f)
			walk(f.NestedFilters)
		}
	}
	walk(filters)
	return out
}

func setToSlice(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

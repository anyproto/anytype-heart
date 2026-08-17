package anyblockjson

// export.go serializes a snapshot into canonical AnyBlock JSON (§2–§7,
// §9–§9a).

import (
	"fmt"
	"sort"
	"strconv"
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
	ResolveFormat      FormatResolver   // optional; nil = bundle-only resolution (§3)
	ResolveOptions     OptionResolver   // optional; nil = option values pass through as ids
	ResolveProperties  PropertyResolver // optional; nil = type documents keep raw recommended-relation ids (§2a)
	Keys               KeyVocabulary    // optional; nil = BundledKeyVocabulary (the derived table — keyvocab.go)
	OmitIds            bool             // export only: drop every id (§9)
	CompactIds         bool             // export only: shorthand for CompactObjectRefs+CompactBlockLabels (§9a)
	CompactObjectRefs  bool             // export only: shorten object refs via the refs legend (§9a; lossless)
	CompactBlockLabels bool             // export only: relabel doc-local block/row/column/view ids to short suffixes (§9a; legend-less, lossy)
	GenerateId         func() string    // import only: id generator for missing ids; nil = random 24-hex
	NormalizeIndent    bool             // import only: clamp over-deep indents instead of rejecting (§4)
	OnWarning          func(Issue)      // optional sink for warning-grade issues, both directions (indent clamps, unrepresentable dates, …)
}

// compactObjectRefs reports whether object-ref compaction (refs legend) is on.
func (o Options) compactObjectRefs() bool { return o.CompactObjectRefs || o.CompactIds }

// compactBlockLabels reports whether doc-local id relabeling is on.
func (o Options) compactBlockLabels() bool { return o.CompactBlockLabels || o.CompactIds }

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

// MarshalPropertyValue converts one property value to its JSON form under
// the §3 rules (dates → RFC 3339, select options → names, object/file →
// id lists, scalars wrap into lists for list-shaped formats). It is the
// row-level building block for API list surfaces that carry requested
// property values (APIV2.md C5) without a full document export. The result
// marshals with encoding/json.
func MarshalPropertyValue(key string, v *types.Value, opts Options) any {
	e := &exporter{opts: opts}
	return e.propertyValue(key, v)
}

// Marshal serializes a snapshot into canonical AnyBlock JSON (§13).
func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("nil snapshot")
	}
	e := &exporter{opts: opts, snapshot: snapshot, sbType: sbType, blocks: map[string]*model.Block{}, visited: map[string]bool{}}
	e.indexBlocks()
	if opts.compactObjectRefs() || opts.compactBlockLabels() {
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

	// idLabels maps a stored block/row/column id to the id written for it, and
	// idsUsed is every id this document has written. One set for every id
	// surface, because they share one uniqueness domain (§4): a sanitized
	// column id, a compact label and a verbatim block id all land in the same
	// document, and any two of them colliding is a document Validate rejects.
	idLabels map[string]string
	idsUsed  map[string]struct{}
}

// seedIdLabels reserves the id each block will be written with, before any
// sanitizing starts. Without it the first block to need sanitizing could take
// the name of a block that was going to be written verbatim — renaming a
// perfectly good authored id, or duplicating it.
func (e *exporter) seedIdLabels() {
	e.idLabels = map[string]string{}
	e.idsUsed = map[string]struct{}{}
	for id, b := range e.blocks {
		// map order is irrelevant here: every id seeded is written as-is, so
		// no two of them compete for the same label
		want := e.localId(id)
		if e.isTableInner(b) {
			if isValidTableInnerId(want) {
				e.idLabels[id] = want
				e.idsUsed[want] = struct{}{}
			}
			continue
		}
		if isValidRefsKey(want) { // the blockId charset: [A-Za-z0-9_-]{1,64}
			e.idLabels[id] = want
			e.idsUsed[want] = struct{}{}
		}
	}
}

func (e *exporter) isTableInner(b *model.Block) bool {
	switch b.GetContent().(type) {
	case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
		return true
	}
	return false
}

// idLabel is the one place a stored id becomes the id written in the document.
// It sanitizes with the charset of the position, then disambiguates against
// every id the document has already written, and remembers its answer so the
// same stored id always renders the same way.
func (e *exporter) idLabel(stored string, sanitize func(string) string) string {
	if stored == "" {
		return ""
	}
	if e.idLabels == nil {
		e.seedIdLabels()
	}
	if got, ok := e.idLabels[stored]; ok {
		return got
	}
	base := sanitize(e.localId(stored))
	label := base
	for n := 2; ; n++ {
		if _, taken := e.idsUsed[label]; !taken {
			e.idsUsed[label] = struct{}{}
			e.idLabels[stored] = label
			return label
		}
		suffix := "_" + strconv.Itoa(n)
		trimmed := base
		if len(trimmed)+len(suffix) > maxIdLen {
			trimmed = trimmed[:maxIdLen-len(suffix)]
		}
		label = trimmed + suffix
	}
}

// blockLabel renders a block's stored id for output (§9). Stored ids are not
// guaranteed to match the schema's block charset: legacy accounts hold ids
// with dots and slashes, and Options.GenerateId belongs to the caller — the
// convert wiring derives ids from file paths. Writing one verbatim made
// Marshal emit a document its own Validate rejects, i.e. an archive that fails
// at import, discovered long after the export.
func (e *exporter) blockLabel(stored string) string {
	return e.idLabel(stored, sanitizeBlockId)
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
		keys = append(keys, e.opts.typeSlug(strings.TrimPrefix(t, typeKeyIdPrefix)))
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
		doc.setNonEmpty("template_for", typeKeys[1])
	}
	doc.setNonEmpty("key", e.snapshot.Key)
	doc.setNonEmpty("properties", e.buildProperties())
	if tp := e.buildTypeProperties(); tp != nil {
		doc.set("type_properties", tp) // present even when empty (§2a)
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
	m.setNonEmpty("background_color", root.BackgroundColor)
	return m
}

//
// ---- properties ----
//

// strippedDetailKeys are the internal/derived properties export removes (§3).
// strippedDetailKeys is the internal-property list, and it is the single
// source of truth for both directions: export removes these keys, and import
// refuses them (§3, §4a — deniedPropertyKey reads this same set). Two lists
// would drift, which is how the import surface ended up strictly wider than
// the export surface.
func strippedDetailKeys() map[string]bool {
	stripped := map[string]bool{detailKeyId: true, detailKeyType: true}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		if !propertiesKeptOnExport[string(k)] {
			stripped[string(k)] = true
		}
	}
	// the importer's own resolution vectors are not bundled relations, so the
	// list above does not cover them; they are internal all the same
	for k := range neverWritableProperties {
		stripped[k] = true
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
		if stripped[k] || lifted[k] {
			continue
		}
		// a stored detail key is not necessarily a property name: real data
		// holds an empty key and keys with control characters in them, and
		// there is no way to write those (§3). Dropping them is what keeps
		// Marshal's output valid — emitting one produced a document its own
		// Validate rejects, which is the invariant §11 states.
		if !isWritablePropertyKey(k) {
			e.warn("/properties", "property key %q cannot be written in this format and is dropped", k)
			continue
		}
		keys = append(keys, k)
	}
	// the document spells slugs (§7.5a), so the canonical alphabetical order
	// is over the SPELLINGS, not the stored keys — the reader sorts what it
	// sees. Values still resolve through the stored key.
	//
	// The collapse pass below runs over the STORED keys sorted, not over map
	// order: which holder keeps a contested spelling must not depend on Go's
	// map iteration, or the canonical form is not canonical and export∘import
	// byte-stability is a coin flip on exactly the spaces that need it most.
	sort.Strings(keys)
	stored := make(map[string]bool, len(keys))
	for _, k := range keys {
		stored[k] = true
	}
	type prop struct{ slug, key string }
	props := make([]prop, 0, len(keys))
	spelled := map[string]bool{}
	for _, k := range keys {
		slug := e.opts.propertySlug(k)
		// a spelling two stored keys agree on would collapse into one JSON key
		// and lose a value. Two ways that happens in a space holding a
		// pre-mint-check shadow: another holder already took the slug, or the
		// slug IS another stored key on this very object (chain step 1 would
		// bind it to that one on the way back). Either way the later holder
		// keeps its honest stored key.
		if slug != k && (spelled[slug] || stored[slug]) {
			slug = k
		}
		spelled[slug] = true
		props = append(props, prop{slug: slug, key: k})
	}
	sort.Slice(props, func(i, j int) bool { return props[i].slug < props[j].slug })
	ordered := make([]prop, 0, len(props))
	seen := map[string]bool{}
	for _, wk := range wellKnownPropertyOrder {
		for _, p := range props {
			if p.key == wk {
				ordered = append(ordered, p)
				seen[p.key] = true
			}
		}
	}
	for _, p := range props {
		if !seen[p.key] {
			ordered = append(ordered, p)
		}
	}
	m := &omap{}
	for _, p := range ordered {
		// presence of a property key is meaningful — it records that the
		// property was set on the object — so values are written verbatim,
		// including empty and default ones (§3); the omit-empty canon applies
		// to block attributes and envelope fields only
		m.set(p.slug, e.propertyValue(p.key, e.snapshot.Details.Fields[p.key]))
	}
	return m
}

// warn reports a warning-grade issue through the caller's sink (§13): a thing
// export had to do that the author would want to know about, but that does not
// make the output invalid. Silent when no sink is wired.
func (e *exporter) warn(path, format string, args ...any) {
	if e.opts.OnWarning == nil {
		return
	}
	e.opts.OnWarning(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
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
	// layout is stored as a number and named in the format (§3); a number
	// outside the enum falls through and exports unchanged.
	if isLayoutKey(key) {
		if n, isNum := v.GetKind().(*types.Value_NumberValue); isNum {
			if name := layoutNames.name(model.ObjectTypeLayout(int32(n.NumberValue))); name != "" {
				return name
			}
		}
	}
	format, ok := e.resolveFormat(key)
	if !ok {
		return protoValueToJSON(v)
	}
	switch format {
	case model.RelationFormat_date:
		if n, isNum := v.GetKind().(*types.Value_NumberValue); isNum {
			if s, ok := formatDateValue(n.NumberValue); ok {
				return s
			}
			// no RFC 3339 form: emitting one anyway would write a string
			// parseDate cannot read, so the value would come back as a string
			// on a date property and stay that way (byte-stable, so nothing
			// corrects it). The raw number round-trips instead.
			e.warn("/properties/"+key,
				"date %v has no RFC 3339 form (outside years 0000-9999), so it is written as a raw number; "+
					"a value this large is usually milliseconds where seconds belong", n.NumberValue)
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
		emitDepth := depth
		if depth > maxBlockIndent {
			if e.opts.OnWarning == nil {
				return fmt.Errorf("block %s: nesting depth %d exceeds the format bound %d", id, depth, maxBlockIndent)
			}
			// read path (C11): degrade rather than fail — clamp the indent and
			// keep the content instead of erroring the whole document.
			e.opts.OnWarning(Issue{Path: "/blocks", Message: fmt.Sprintf("block %s: nesting depth %d exceeds the bound %d — indent clamped", id, depth, maxBlockIndent)})
			emitDepth = maxBlockIndent
		}
		m, withChildren, err := e.blockToJSON(b, emitDepth)
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
	if e.opts.compactBlockLabels() {
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
		m.setNonEmpty("id", e.blockLabel(b.Id))
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
		m.setNonEmpty("object_id", e.compactObjectId(bm.TargetObjectId))
		withChildren = false
	case *model.BlockContentOfLink:
		l := orEmpty(c.Link)
		m.set("type", "link")
		m.setNonEmpty("object_id", e.compactObjectId(l.TargetBlockId))
		if l.CardStyle != model.BlockContentLink_Text {
			m.setNonEmpty("card_style", cardStyleNames.name(l.CardStyle))
		}
		if l.IconSize != model.BlockContentLink_SizeNone {
			m.setNonEmpty("icon_size", iconSizeNames.name(l.IconSize))
		}
		if l.Description != model.BlockContentLink_None {
			m.setNonEmpty("description", linkDescriptionNames.name(l.Description))
		}
		m.setNonEmpty("properties", stringsToAny(e.opts.propertySlugs(l.Relations)))
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
		m.set("type", "table_of_contents")
		withChildren = false
	case *model.BlockContentOfRelation:
		m.set("type", "property")
		m.setNonEmpty("key", e.opts.propertySlug(orEmpty(c.Relation).Key))
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
		m.setNonEmpty("view_id", w.ViewId)
		m.setNonEmpty("auto_added", w.AutoAdded)
	case *model.BlockContentOfChat:
		m.set("type", "chat")
		withChildren = false
	case *model.BlockContentOfFeaturedRelations:
		m.set("type", "featured_properties")
		withChildren = false
	case *model.BlockContentOfIcon:
		m.set("type", "icon")
		m.setNonEmpty("name", orEmpty(c.Icon).Name)
		withChildren = false
	case *model.BlockContentOfSmartblock:
		return nil, false, nil
	default:
		if e.opts.OnWarning != nil {
			// read path (C11): drop the unrepresentable block with a warning
			// instead of failing the whole read.
			e.opts.OnWarning(Issue{Path: "/blocks", Message: fmt.Sprintf("block %s: content type %T has no JSON mapping — dropped", b.Id, b.Content)})
			return nil, false, nil
		}
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
		m.setNonEmpty("vertical_align", verticalAlignNames.name(b.VerticalAlign))
	}
	m.setNonEmpty("background_color", b.BackgroundColor)
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
		m.setNonEmpty("icon_emoji", t.IconEmoji)
		m.setNonEmpty("icon_image", e.compactObjectId(t.IconImage))
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
	if !e.opts.compactObjectRefs() || len(marks) == 0 {
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
		case mk.Type == model.BlockContentTextMark_Link && isObjectLink(mk.Param):
			// rendered as an Object mark (§8.3), so its target compacts too
			id, _ := parseObjectLink(mk.Param)
			clone := *mk
			clone.Param = objectLinkDest(e.compactObjectId(id))
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
	m.setNonEmpty("object_id", e.compactObjectId(objectId))
	m.setNonEmpty("name", f.Name)
	m.setNonEmpty("mime_type", f.Mime)
	m.setNonEmpty("size", f.Size_)
	if f.Style != model.BlockContentFile_Auto {
		m.setNonEmpty("style", fileStyleNames.name(f.Style))
	}
	if f.AddedAt != 0 {
		// addedAt is a string in the schema, so there is no number form to
		// fall back to (§5): an unrepresentable timestamp is dropped rather
		// than written as a string no reader can parse back
		if s, ok := formatDate(f.AddedAt); ok {
			m.set("added_at", s)
		} else {
			e.warn("", "file block: added_at %d has no RFC 3339 form (outside years 0000-9999), so it is omitted", f.AddedAt)
		}
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
	if e.opts.compactObjectRefs() && id != "" {
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
				case mk.Type == model.BlockContentTextMark_Link && isObjectLink(mk.Param):
					// normalizes to an Object mark on render (§8.3)
					id, _ := parseObjectLink(mk.Param)
					addObject(id)
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
	// each half builds only when its flag is on (C4 split: object-ref
	// compaction is lossless via the legend, block relabeling is lossy);
	// the collision-avoid set always covers both id populations because
	// un-relabeled ids stay in the document verbatim
	if e.opts.compactObjectRefs() {
		e.objectRefs = suffixLabels(setToSlice(objects), compactIdMinLen, func(candidate string) bool {
			return fullIds[candidate] || !isValidRefsKey(candidate)
		})
		// short ids label as themselves; drop those the schema charsets reject
		dropInvalidLabels(e.objectRefs, isValidRefsKey)
	}
	if e.opts.compactBlockLabels() {
		// only machine-minted opaque ids relabel (isMintedLocalId); every id
		// that keeps its full spelling is reserved through the fullIds
		// avoid-set, so no label can alias a served id — and the census inside
		// mintedSuffixLabels runs over ALL local ids, so a label cannot be an
		// ambiguous suffix of one either. Labels stay dash-free as before:
		// '-' is the derived-cell-id separator and forbidden in row/column
		// ids (§6.1) — minted suffixes are hex, so the check is a backstop.
		e.localIds = mintedSuffixLabels(setToSlice(locals), compactIdMinLen, func(candidate string) bool {
			return fullIds[candidate] || isInvalidLocalLabel(candidate)
		})
	}
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

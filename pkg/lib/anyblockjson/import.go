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
	Version     json.Number `json:"version"`
	Kind        string      `json:"kind"`
	Id          string      `json:"id"`
	Type        string      `json:"type"`
	TemplateFor string      `json:"template_for"`
	Key         string      `json:"key"`
	// RelationSettings is a kind:relation document's definition group (§2d):
	// one propertyDefinition, whose three travelling members stand for the
	// stored relation-definition keys that `properties` refuses.
	RelationSettings *jsonRelationSettings `json:"relation_settings"`
	// Icon and Cover are the typed envelope fields (§2b). Each is one object
	// whose `format` member selects the variant, and each stands for a family
	// of hidden stored keys that `properties` refuses.
	Icon       *Icon          `json:"icon"`
	Cover      *Cover         `json:"cover"`
	Properties map[string]any `json:"properties"`
	// TypeSettings is a kind:object_type document's definition group (§2a):
	// the five lifted settings plus property_definitions.
	TypeSettings *jsonTypeSettings `json:"type_settings"`
	// PropertyKeys is the §3 slug→stored-key legend: what this document says
	// its own key spellings mean, consulted before any vocabulary so a reader
	// without the space still lands on the right relation. Its values are
	// AUTHORITATIVE — taken as the stored key, not liveness-checked (§3).
	PropertyKeys map[string]string `json:"property_internal_keys"`
	// TypeKeys is the same legend for the TYPE namespace — separate map,
	// because a space may slug a relation and a type onto one term (§3).
	TypeKeys map[string]string `json:"type_internal_keys"`
	// OptionIds is the §9a option legend, nested {property spelling: {option
	// name: option id}}. Unlike the two above its values are HINTS, honoured
	// only where the id still names a live option of that relation (§3).
	OptionIds map[string]map[string]string `json:"option_ids"`
	Blocks    []*jsonBlock                 `json:"blocks"`
	Items     []string                     `json:"items"`
	Store     map[string]any               `json:"store"`
	Root      *jsonRootEscape              `json:"root"`
}

// jsonRelationSettings is the decoded `relation_settings` group (§2d). The
// two RawMessage members are raw because each has THREE states the schema
// admits — absent, null, and a value — and a decoded Go pointer collapses
// the first two: member presence mirrors stored-key presence exactly, and a
// stored null is a value (§3, 80 production relations hold one).
type jsonRelationSettings struct {
	Format      string          `json:"format"`
	IncludeTime json.RawMessage `json:"include_time"`
	TargetTypes json.RawMessage `json:"object_types"`
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

	Checked  bool   `json:"checked"`
	Color    string `json:"color"`
	Text     string `json:"text"`
	Language string `json:"language"`
	Icon     *Icon  `json:"icon"`

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
	// refusal is the first key slot whose resolution named nothing — see
	// propertyKeyAt. Deferred because the slots that can produce it are deep
	// inside block construction; build turns it into the ValidationError
	// before it returns a snapshot.
	refusal *Issue
	// foldedUnrebuilt records that this document carries a FOLDED participant
	// reference (§9) this reader cannot rebuild, because it names no space.
	// Set during the walk and reported once in build: the fault is the
	// reader's wiring, not any one slot's, and a document can hold thousands
	// of them.
	foldedUnrebuilt bool
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
				// a cell's id is derived (§6.1) but it is still a block id,
				// and the table owns the whole grid whether or not the cell is
				// written: the editor materializes the missing cell at exactly
				// that id the first time it is filled, so a generated id may
				// not be sitting on it. This is the same claim validation
				// makes (§4).
				for _, col := range jb.Columns {
					if row.Id != "" && col.Id != "" {
						imp.claimId(row.Id + "-" + col.Id)
					}
				}
				for _, cell := range row.Cells {
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

// propertyKeyAt is propertyKey at a slot that can REFUSE — every key slot
// outside `/properties`, which runs its own admission at the detail seam.
//
// The rule is one sentence, and it is the same one the document side now
// carries at every slot: **a key slot has to name something**. The document
// half is the schema (`minLength: 1`); this is the resolution half, and it is
// reachable only through Options.Keys, which §3 accepts from anyone: a
// vocabulary answering ("", true) for a non-empty spelling. `/properties`,
// `/type`, `/template_for` and a property definition's `object_types` refused that
// from the start; the nine slots that did not stored the empty key and then
// LOST the slot on the way back out — a column and a sort vanish, a property
// block and a link's shown-property list come back nameless, a filter
// re-exports as a node that filters on nothing.
//
// The refusal is deferred rather than returned, because these slots sit deep
// inside block construction (dataview views, filter trees) where an error
// return would have to be threaded through a dozen signatures for a fault
// none of them can repair. build reports the first one before it hands back a
// snapshot, so no caller ever sees the damaged object.
// The `slot` names which kind of slot was reading, and the message names the
// SPELLING rather than leaning on the pointer: the fault is in the reader's
// vocabulary, not in the document, so "which spelling does your table answer
// nothing for" is the question a caller can act on. The pointer stays coarse
// (`/blocks`) because these slots are built without one and inventing a
// precise-looking pointer that is wrong is worse than a coarse true one.
func (imp *importer) propertyKeyAt(slug, slot string) string {
	key := imp.propertyKey(slug)
	if slug != "" && key == "" {
		imp.refuse("/blocks", fmt.Sprintf(
			"the vocabulary resolves the %s spelling %q to the empty key; "+
				"a key slot has to name something (§3)", slot, slug))
	}
	return key
}

// propertyKeysAt is the list form (a link block's shown properties).
func (imp *importer) propertyKeysAt(slugs []string, slot string) []string {
	if len(slugs) == 0 {
		return slugs
	}
	out := make([]string, len(slugs))
	for i, slug := range slugs {
		out[i] = imp.propertyKeyAt(slug, slot)
	}
	return out
}

// refuse records the first key-slot refusal. First and not all of them: the
// issue list §12 promises is one fault per slot, and a vocabulary answering
// "" answers "" everywhere, so collecting them would print the same fault
// once per slot in the document.
func (imp *importer) refuse(path, message string) {
	if imp.refusal == nil {
		imp.refusal = &Issue{Path: path, Message: message}
	}
}

// propertyKeys is the list form (a link block's shown properties). Like the
// singular one it belongs to the importer, not to Options: Options holds the
// reader's vocabulary, and the document's own legend outranks it.
func (imp *importer) propertyKeys(slugs []string) []string {
	if len(slugs) == 0 {
		return slugs
	}
	out := make([]string, len(slugs))
	for i, slug := range slugs {
		out[i] = imp.propertyKey(slug)
	}
	return out
}

// typeKey inverts a TYPE key slot: the document's own legend first (§3),
// then the vocabulary in force — propertyKey on the type namespace's legend.
//
// It used to carry a reservation, the mirror of writableTypeSlug's: the
// vocabulary could not move the `template` spelling in either direction,
// because the kind derivation and the /template_for gate read the same field
// through a DIFFERENT chain (docTypeKey — the document's own, deliberately
// blind to the reader's vocabulary, because Validate has no vocabulary and
// §12 requires the two to agree). A vocabulary answering
// TypeKey("template") == "69bbfc…" therefore produced a Template smartblock
// whose ObjectTypeKeys do not contain `template` — which every downstream
// template check misses, since they all test
// lo.Contains(ObjectTypeKeys, TypeKeyTemplate).
//
// `kind` answers both questions now, off a field no chain touches, so the two
// halves cannot disagree and there is nothing left to reserve. The path is
// still the SLOT being read, because the empty-key refusal below fires in
// three of them: the envelope `type`, `template_for`, and every
// `type_settings.property_definitions[i].object_types[j]` (§2a).
func (imp *importer) typeKey(slug, path string) string {
	if key, ok := imp.doc.TypeKeys[slug]; ok && key != "" {
		return key
	}
	return imp.opts.typeKey(slug)
}

// warn reports a warning-grade issue through the caller's sink (§13) — the
// import half of exporter.warn. Silent when no sink is wired.
func (imp *importer) warn(path, format string, args ...any) {
	if imp.opts.OnWarning == nil {
		return
	}
	imp.opts.OnWarning(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
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
	return declaredFormatWith(imp.opts, key, name)
}

// declaredFormatWith is that rule with nothing but Options behind it, because
// the §2a array arrives through TWO doors and the rule is the array's, not the
// document's: BuildRecommendedLists — the API's PATCH-type channel — read the
// name literally, so `{"key": "name", "format": "text"}` created the bundled
// `name` property as longtext through one door and kept it shorttext through
// the other. The whole point of the collapse is that `text` resolves per key
// (§3); a door that skips the resolution re-introduces exactly the loss the
// collapse was designed not to have, and the two doors then disagree about
// what one array means.
func declaredFormatWith(opts Options, key, name string) model.RelationFormat {
	// An ABSENT format is not a declaration of text. `format` is optional
	// in both slots that carry it, so a document that omits it has said
	// nothing about the property — and the answer to nothing is the chain
	// (§3), not longtext. Treating absence as `text` silently OVERRODE the
	// bundled table: `{"key": "due_date"}` in a dataview's property list
	// pinned a bundled DATE property to longtext, so its filters stopped
	// being dates, while omitting the list entirely resolved correctly.
	// Listing a property without its format was worse than not listing it
	// at all — the opposite of what any author would assume, and reported
	// by nothing.
	//
	// Canonical export always writes a format (formatName answers "text"
	// even for longtext), so absence only ever arrives from a hand-written
	// document — exactly the population that means "I did not say". A
	// declared "text" still stands, and still folds per key below.
	if name == "" {
		if resolved, ok := resolveFormatWith(opts, key); ok {
			return resolved
		}
		return model.RelationFormat_longtext
	}
	f := formatNames.value(name)
	if f != model.RelationFormat_longtext {
		return f
	}
	if resolved, ok := resolveFormatWith(opts, key); ok && resolved == model.RelationFormat_shorttext {
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
	// `kind` is the whole of it (§2). There is no derivation from the type
	// term any more: the spelling `template` resolved through the document's
	// own chain used to mean Template, which meant the same field answered
	// two unrelated questions — which type this object has, and what kind of
	// object it is — and only stayed consistent by forbidding every reader
	// from spelling that one type key differently. An absent `kind` on a
	// document whose type is literally `template` is the pre-v0.22 spelling
	// and Validate refuses it by name, so it never reaches here (§10).
	sbType := model.SmartBlockType_Page
	if doc.Kind != "" {
		sbType = kindNames.value(doc.Kind)
	}

	// the envelope id goes through the reference reader like any object
	// reference (§9): a stray informative suffix is trimmed, and a bare
	// identity — the participant document's own folded id — rebuilds this
	// space's participant id. Claimed so a generated block id cannot land on
	// the rebuilt form.
	objectId := imp.claimId(imp.objectRef(doc.Id))
	if objectId == "" {
		objectId = imp.genId()
	}

	var objectTypes []string
	if doc.Type != "" {
		// the seam refuses a resolution onto the empty key (§3): a
		// vocabulary can answer "" for a non-empty spelling, which became
		// the ObjectTypes entry "ot-" and re-exported as no type at all —
		// silently. That is the only refusable resolution here: a non-empty
		// stored key of any shape round-trips verbatim, unlike a property
		// key, which has to survive as a JSON member name.
		typeKey := imp.typeKey(doc.Type, "/type")
		if typeKey == "" {
			return 0, nil, &ValidationError{Issues: []Issue{{
				Path:    "/type",
				Message: unwritableKeyReason("resolved type key", typeKey),
			}}}
		}
		objectTypes = append(objectTypes, domain.TypeKey(typeKey).URL())
		if sbType == model.SmartBlockType_Template && doc.TemplateFor != "" {
			target := imp.typeKey(doc.TemplateFor, "/template_for")
			if target == "" {
				return 0, nil, &ValidationError{Issues: []Issue{{
					Path:    "/template_for",
					Message: unwritableKeyReason("resolved type key", target),
				}}}
			}
			objectTypes = append(objectTypes, domain.TypeKey(target).URL())
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
		// the §2a type-settings lift is KIND-SCOPED (typesettings.go): on a
		// TYPE document the five stored keys live in the group and their flat
		// spellings are refused with the repair named, while on every other
		// kind they stay ordinary properties (apiObjectKey is real data on
		// 9,725 relation documents)
		if isTypeSmartBlock(sbType) {
			if typeSettingsLiftedDetailKeys()[key] {
				return 0, nil, &ValidationError{Issues: []Issue{{
					Path: "/properties/" + escapeJSONPointer(slug),
					Message: fmt.Sprintf("%q is written on a type document as %s in type_settings (§2a), "+
						"not as a property", key, typeSettingsLiftedKeyRepair(key)),
				}}}
			}
			// the install-provenance keys are dropped, not refused, on a type
			// document: a document carrying one is stale rather than wrong —
			// the transientProperties policy, scoped by kind (§2a)
			if _, stale := typeProvenanceKeys[key]; stale {
				continue
			}
		}
		// transient state and the attribution keys are dropped, not refused: a
		// document carrying one is stale rather than wrong. Export writes no
		// transient key at all, and writes the attribution keys as derived
		// captions — `<id>#<name>` recovered from the tree on every rebuild,
		// which no write path could honour (§3) — so this fires on every
		// document this package produces for an object with a creator, and
		// on a stale or hand-written one for the rest.
		if isDroppedOnImport(key) {
			continue
		}
		// the seam admits only keys export could write (§3): a wider
		// vocabulary can resolve a spelling onto a key with no writable form
		// — the empty string included — which used to land details[""]
		// silently, Validate clean and Unmarshal clean, and the re-export
		// then dropped the property with only a warning.
		if !isWritablePropertyKey(key) {
			return 0, nil, &ValidationError{Issues: []Issue{{
				Path:    "/properties/" + escapeJSONPointer(slug),
				Message: unwritableKeyReason("resolved property key", key),
			}}}
		}
		if first, dup := boundBy[key]; dup {
			return 0, nil, &ValidationError{Issues: []Issue{{
				Path:    "/properties/" + escapeJSONPointer(slug),
				Message: fmt.Sprintf("%q and %q both address property %q — keep one", first, slug, key),
			}}}
		}
		boundBy[key] = slug
		if v := imp.propertyValue(key, slug, doc.Properties[slug]); v != nil {
			details.Fields[key] = v
		}
	}
	// the typed envelope fields are written after the property loop and can
	// never collide with it: the nine keys they stand for are refused in
	// `properties` on the RESOLVED stored key (deniedPropertyKey), which is
	// what keeps a space-minted relation whose own stored key is `icon_emoji`
	// an ordinary property (§2b)
	imp.applyIcon(details)
	imp.applyCover(details)
	if err := imp.applyRelationEnvelope(details, sbType); err != nil {
		return 0, nil, err
	}
	if err := imp.applyTypeSettings(details, sbType); err != nil {
		return 0, nil, err
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
	jbs, indents := imp.topLevelBlocks(details)
	blocks, err := imp.flatSubtree(jbs, indents, root, -1)
	if err != nil {
		return 0, nil, fmt.Errorf("build blocks: %w", err)
	}
	all = append(all, blocks...)

	// a key slot that resolved onto nothing refuses the document rather than
	// handing back an object with the slot missing (propertyKeyAt)
	if imp.refusal != nil {
		return 0, nil, &ValidationError{Issues: []Issue{*imp.refusal}}
	}
	// the folded participant references this reader could not rebuild (§9).
	// One line for the document, not one per slot: the fault is a reader
	// wired without a space, and every such reference in the object shares it.
	if imp.foldedUnrebuilt {
		imp.warn("", "this document was written with participants folded (§9) and "+
			"Options.SpaceId names no space: their references import as bare "+
			"identities, which address no object. Set SpaceId to the space this "+
			"document is being read into.")
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
			vals = append(vals, &types.Value{Kind: &types.Value_StringValue{StringValue: imp.objectRef(id)}})
		}
		coll.Fields[storeKeyItems] = &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
	}
	return coll
}

// propertyValue decodes a property per its resolved format (§3). Scalars of
// list-shaped formats normalize to single-element lists (§11). An explicit
// null stays a null value — presence of the key is preserved (§3).
//
// Both spellings travel: `key` is the stored key the value lands on, `slug`
// the term the document spelled it with. The slug is not decoration — the
// option legend's outer key is the SPELLING (optionrefs.go), because the
// reader that resolves it is reading the document, not the store.
func (imp *importer) propertyValue(key, slug string, v any) *types.Value {
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
		return wrapToList(mapJSONStrings(v, func(name string) string { return imp.resolveOption(key, slug, name) }))
	case model.RelationFormat_object, model.RelationFormat_file:
		return wrapToList(mapJSONStrings(v, imp.objectRef))
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
	// §7a first, and the order is load-bearing: a wrapped dataview is only
	// visible to the §7 pin at the indent-0 position the pin requires once
	// the container above it is gone (160 corpus objects gain the `dataview`
	// id under OmitIds this way, 0 lose it), and a wrapped title is only
	// absorbed into `properties.name` once the lift has put it at indent 0.
	raw, indents = liftTransparentContainers(raw, indents)
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

// liftTransparentContainers is the import half of §7a: a `group` entry
// contributes no block, and every following entry indented deeper than it
// re-bases one level shallower — recursively, so a chain of n containers
// removes n levels. Any attribute on the container is ignored, and so is its
// id: a container is not a block, so nothing can address it.
//
// It is a pre-pass over the flat run rather than a case inside the rebuild,
// which is what lets it run at all three flatSubtree entry points — the
// document body, a table cell's array form, and the fragment WRITE path. A
// path that misses it does not merely keep a `group` in the JSON: it mints a
// real Layout_Div that no read will ever show and that normalization never
// removes while it has children — a phantom indent level living in the
// object forever.
//
// Monotonicity survives by construction, so the lift can never manufacture
// an F6 violation: a container at indent g had g ≤ p+1, and its first child,
// at g+1, lands at g.
func liftTransparentContainers(jbs []*jsonBlock, indents []int) ([]*jsonBlock, []int) {
	// open holds the indents of the containers this entry is inside, so the
	// shift is just how many of them there are
	var open []int
	outJbs := make([]*jsonBlock, 0, len(jbs))
	outIndents := make([]int, 0, len(indents))
	for i, jb := range jbs {
		if jb == nil {
			continue
		}
		k := indents[i]
		for len(open) > 0 && open[len(open)-1] >= k {
			open = open[:len(open)-1]
		}
		if transparentBlockTypes[jb.Type] {
			open = append(open, k)
			continue
		}
		outJbs = append(outJbs, jb)
		outIndents = append(outIndents, k-len(open))
	}
	return outJbs, outIndents
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
	if len(marks) == 0 {
		return text, nil, nil
	}
	return text, &model.BlockContentTextMarks{Marks: marks}, nil
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
		calloutIconFrom(jb.Icon, t)
	}
	return t, nil
}

// fileFromJSON builds a file-family block content (§5); state is recomputed,
// never serialized.
func (imp *importer) fileFromJSON(jb *jsonBlock) *model.BlockContentFile {
	f := &model.BlockContentFile{
		Type:           fileTypeNames.value(jb.Type),
		TargetObjectId: imp.objectRef(jb.ObjectId),
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
		TargetObjectId: imp.objectRef(jb.ObjectId),
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
		TargetBlockId: imp.objectRef(jb.ObjectId),
		CardStyle:     cardStyleNames.value(jb.CardStyle),
		IconSize:      iconSizeNames.value(jb.IconSize),
		Description:   linkDescriptionNames.value(jb.Description),
		Relations:     imp.propertyKeysAt(propKeys, "link block `properties`"),
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
	case transparentBlockTypes[jb.Type]:
		// DEFENSIVE: unreachable through every public entry point today —
		// Unmarshal and UnmarshalBlock both validate first, and the validation
		// side refuses a container in a cell and in a single-block fragment on
		// its own (probed: mutating this line leaves the whole package green).
		// It stays as the backstop for a future caller that skips validation,
		// and it must not be deleted on the strength of a coverage report.
		//
		// §7a: a transparent container contributes no block of its own, and
		// every flat run lifts it away before this. What reaches here is a
		// caller that addresses exactly ONE block — UnmarshalBlock, or a
		// table cell, which is a position rather than a run — and a caller
		// that asked for one block must be told it named nothing, not handed
		// a wrapper no read will ever show it again.
		return nil, fmt.Errorf("block %s: %q is a transparent container (§7a) and contributes no block of its own", id, jb.Type)
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
		b.Content = &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{
			Key: imp.propertyKeyAt(jb.Key, "property block `key`")}}
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

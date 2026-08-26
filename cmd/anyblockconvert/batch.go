package main

import (
	"crypto/sha1"
	"encoding/hex"
	"github.com/globalsign/mgo/bson"
	"sort"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/lexid"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// pendingSnapshot is a Relation/RelationOption object the batch mints the
// first time a custom property or option value is referenced.
type pendingSnapshot struct {
	id       string
	sbType   model.SmartBlockType
	snapshot *model.SmartBlockSnapshotBase
}

// batch is the cross-document wiring pkg/lib/anyblockjson deliberately
// leaves to its caller (SPEC.md §2a "the import wiring's job", §3 "creating
// options is the wiring's job"): it answers anyblockjson.Options'
// FormatResolver/OptionResolver/PropertyResolver callbacks from the
// pre-scanned typeProperties table, lazily minting Relation/RelationOption
// snapshots the same way core/block/import/csv and core/block/import/notion
// build them for their own generated relations and options.
type batch struct {
	// formats is keyed by the STORED property key, which is what every
	// reader below hands it: anyblockjson resolves a `properties` key or a
	// `type_settings.property_definitions[].property` through the document's own property_internal_keys legend
	// and the bundled table (§3, importer.propertyKey) before it calls
	// ResolveFormat or builds a PropertyDefinition. anyblockbatch.ScanFormats
	// runs the same chain when it builds the table — keying it by the raw
	// spelling instead made every legend-backed property a silent miss here.
	formats map[string]anyblockbatch.FormatInfo

	relIDs map[string]string // stored property key -> minted relation object id
	// minted is the batch's key vocabulary: the SPELLING a document used ->
	// the internal key minted for it. A property an author declared by name
	// or spelling gets a fresh bson id here, the way the app mints one when
	// a user creates a property, and every document's detail keys resolve
	// through this map so they all land on that one key (§2e).
	minted map[string]string
	optIDs map[string]string // "stored key\x00name" -> minted option object id

	// optNames is optIDs read backwards — "stored key\x00option id" -> name —
	// and it is what makes OptionName answerable here. The batch is the space
	// this conversion imports into, so the set of options it has minted IS the
	// set of live options, and an id outside it names nothing the archive
	// carries (see OptionName).
	optNames map[string]string

	// optOrder is the last order id handed out per property key. Every option
	// needs one: options sort on orderId+name concatenated
	// (database.OrderMap.BuildOrder), so an option without an order id is
	// compared by name against everyone else's order id and lands
	// arbitrarily — before the declared vocabulary when its name sorts below
	// the lexid alphabet, after it otherwise.
	optOrder map[string]string

	// optColor is the next palette position per property key, and optClaimed
	// the colors that property's vocabulary names explicitly (§2a) so the
	// cycle never hands one of them out a second time. Every option needs a
	// color too: the app assigns one on creation (pkg/lib/schema.Relation
	// CreateOptionDetails, core/block/import/markdown), so an option minted
	// without one is not "default-colored", it is the only kind of option in
	// the space missing the detail entirely.
	optColor   map[string]int
	optClaimed map[string]map[string]bool

	// typeIDs maps a type key this bundle defines to the id its document
	// carries, so a property targeting it references the same id every other
	// reference in the batch uses.
	typeIDs map[string]string

	pending []pendingSnapshot
}

// newBatch pre-declares every select vocabulary the batch knows about before
// any document converts. Order matters: an option first seen as a value on
// some object is minted without an orderId, and the directory walk reaches
// objects/ before types/, so declaring lazily would leave the used values
// unordered and the unused ones ordered.
//
// The map key IS the relation key the options are declared under — it has to
// be the stored key, since that is what OptionId is later called with. That
// holds because ScanFormats resolves the term (§3) before keying; when it
// did not, a legend-backed vocabulary was pre-minted under the SPELLING, so
// the declared options sat on a relation nothing referenced while the values
// that actually arrived minted a second, order-less set under the real key.
func newBatch(formats map[string]anyblockbatch.FormatInfo, typeIds map[string]string) (b *batch) {
	b = &batch{
		formats:    formats,
		relIDs:     map[string]string{},
		minted:     map[string]string{},
		optIDs:     map[string]string{},
		optNames:   map[string]string{},
		optOrder:   map[string]string{},
		optColor:   map[string]int{},
		optClaimed: map[string]map[string]bool{},
		typeIDs:    typeIds,
	}
	keys := make([]string, 0, len(formats))
	for k := range formats {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic option ids across runs
	for _, k := range keys {
		b.declareOptions(domain.RelationKey(k), formats[k].Options)
	}
	return b
}

func (b *batch) relationCount() int { return len(b.relIDs) }
func (b *batch) optionCount() int   { return len(b.optIDs) }

// resolveFormat implements anyblockjson.FormatResolver. `key` arrives already
// resolved (importer.propertyKey), which is why formats is keyed the same way.
func (b *batch) resolveFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	if fi, ok := b.formats[string(key)]; ok {
		return fi.Format, true
	}
	return 0, false
}

// OptionId implements anyblockjson.OptionResolver: allocates a stable
// RelationOption object the first time (key, name) is seen in the batch.
func (b *batch) OptionId(key domain.RelationKey, name string) (string, bool) {
	mapKey := string(key) + "\x00" + name
	if id, ok := b.optIDs[mapKey]; ok {
		return id, true
	}
	id := b.mintOption(key, name, b.nextOptionOrder(key), b.nextOptionColor(key))
	b.optIDs[mapKey] = id
	return id, true
}

// OptionName implements anyblockjson.OptionResolver. This tool only imports,
// but the call is NOT export-only: it is also the liveness question a
// document's `option_ids` entry is checked against (§3, §9a — an id is
// honoured only where the resolver answers for it as an option of that
// relation). A resolver that stubs this out disables the legend for
// everything it converts, which is what this one used to do, silently and on
// the strength of a stale doc comment.
//
// The archive being built is the space that answers here, and its options are
// exactly the ones this batch has minted, so optNames IS the liveness table.
// That keeps the safety property the legend leans on: an id this method
// confirms always has a RelationOption object in `pending`, so honouring one
// can never write a reference the archive does not carry.
//
// What it means in practice, said plainly rather than left to be inferred: a
// bundle exported from another space carries THAT space's option ids, and
// none of them can be live here, since every id in this archive is derived
// from (property key, option name) by optionLocalKey. Those entries fail
// liveness and their values resolve by name, which is the fallback §3
// prescribes and the only thing a fresh-space converter could do with a
// foreign id anyway. Where the legend does bite is an id this batch itself
// minted, which is any id naming an option the archive already carries: the
// value lands on that option even when the name beside it has moved on, so a
// renamed vocabulary re-points its old values instead of minting a second
// option under the stale name (the resurrection §3 describes).
func (b *batch) OptionName(key domain.RelationKey, id string) (string, bool) {
	name, ok := b.optNames[string(key)+"\x00"+id]
	return name, ok
}

// PropertyId implements anyblockjson.PropertyResolver: allocates a stable
// Relation object for a typeProperties entry (§2a) the first time its key is
// seen in the batch. Bundled (system) properties resolve to their bundled
// url instead of minting a new object — installBundledRelationsAndTypes
// (core/block/import/common/objectcreator) installs those automatically.
func (b *batch) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	if bundle.HasRelation(def.Key) {
		// a bundled select still needs its options to exist in this space
		b.declareOptions(def.Key, def.Options)
		return def.Key.BundledURL(), true
	}
	key := string(def.Key)
	b.declareOptions(def.Key, def.Options)
	if id, ok := b.relIDs[key]; ok {
		return id, true
	}
	id := b.mintRelation(def)
	b.relIDs[key] = id
	return id, true
}

// PropertySlug and PropertyKey implement anyblockjson.KeyVocabulary: the
// batch's own spelling layer, laid over the bundled one.
//
// This is what makes minting safe across a bundle. A property declared once
// in properties.json by the spelling `cooking_time` is minted a bson key
// here; a recipe document that carries `"cooking_time": 90` then resolves
// that detail key through this vocabulary and lands on the SAME minted key,
// instead of writing a detail no relation object describes.
func (b *batch) PropertySlug(key string) string {
	for spelling, minted := range b.minted {
		if minted == key {
			return spelling
		}
	}
	return anyblockjson.BundledKeyVocabulary{}.PropertySlug(key)
}

func (b *batch) PropertyKey(slug string) (string, bool) {
	if minted, ok := b.minted[slug]; ok {
		return minted, true
	}
	return anyblockjson.BundledKeyVocabulary{}.PropertyKey(slug)
}

func (b *batch) TypeSlug(key string) string {
	return anyblockjson.BundledKeyVocabulary{}.TypeSlug(key)
}

func (b *batch) TypeKey(slug string) (string, bool) {
	return anyblockjson.BundledKeyVocabulary{}.TypeKey(slug)
}

// PropertyById implements anyblockjson.PropertyResolver. Only used on
// export; this tool only imports, so it's never called.
func (b *batch) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return anyblockjson.PropertyDefinition{}, false
}

// declareOptions pre-mints the vocabulary a typeProperties entry declares
// (§2a), in declaration order, so every value exists whether or not any
// record happens to use it and the order is the author's rather than
// alphabetical. Names already minted from usage are adopted, not duplicated.
// Options that declare no color take the next palette entry the vocabulary
// has not claimed, which is why the explicit colors are collected first.
func (b *batch) declareOptions(key domain.RelationKey, opts []anyblockjson.OptionDefinition) {
	for _, o := range opts {
		if o.Color == "" {
			continue
		}
		if b.optClaimed[string(key)] == nil {
			b.optClaimed[string(key)] = map[string]bool{}
		}
		b.optClaimed[string(key)][o.Color] = true
	}
	for _, o := range opts {
		mapKey := string(key) + "\x00" + o.Name
		if _, done := b.optIDs[mapKey]; done {
			continue
		}
		color := o.Color
		if color == "" {
			color = b.nextOptionColor(key)
		}
		b.optIDs[mapKey] = b.mintOption(key, o.Name, b.nextOptionOrder(key), color)
	}
}

// objectTypeIds turns the type keys a property targets into the ids a
// snapshot references them by, the same split PropertyId makes for
// relations: a type this batch defines is referenced by the id its own
// document carries, so the importer relinks it along with everything else,
// and a bundled type is referenced by its bundled url (_ot<key>), the form
// recommendedRelations already uses for bundled properties (_br<key>).
func (b *batch) objectTypeIds(def anyblockjson.PropertyDefinition) []string {
	if len(def.ObjectTypes) == 0 {
		return nil
	}
	out := make([]string, 0, len(def.ObjectTypes))
	for _, key := range def.ObjectTypes {
		if id, local := b.typeIDs[key]; local {
			out = append(out, id)
			continue
		}
		out = append(out, domain.TypeKey(key).BundledURL())
	}
	return out
}

// targetTypeId resolves a template's target type key to the id that type's own
// document carries — the value targetObjectType has to hold for the pb importer
// to relink it along with every other reference in the batch. A type the bundle
// does not define, or defines without an id, has no usable value: unlike a
// property's objectTypes, a bundled url will not do (see
// anyblockbatch.CheckTemplateTargets, which rejects that bundle up front).
func (b *batch) targetTypeId(key string) (string, bool) {
	id, defined := b.typeIDs[key]
	return id, defined && id != ""
}

// optionLexId mirrors core/block/editor/order.LexId. It is duplicated rather
// than imported because that package pulls in the whole smartblock editor;
// the two must stay in step or ids minted here will not interleave with ones
// the app generates later.
var optionLexId = lexid.Must(lexid.CharsBase64, 4, 4000)

// nextOptionOrder hands out the next order id for a property, continuing
// after whatever was assigned last. Declared vocabulary is laid down first
// (newBatch), so an option discovered later from a value nobody declared
// lands after it rather than in the middle of it.
func (b *batch) nextOptionOrder(key domain.RelationKey) string {
	last, seen := b.optOrder[string(key)]
	if !seen {
		last = optionLexId.Middle()
	} else {
		last = optionLexId.Next(last)
	}
	b.optOrder[string(key)] = last
	return last
}

// nextOptionColor hands out the next palette color for a property, skipping
// the ones its vocabulary claims explicitly. Cycling rather than picking at
// random (constant.RandomOptionColor, what the app does) gives a vocabulary
// that names no colors ten distinct ones instead of the same color three
// times, and keeps a converted bundle byte-identical across runs.
func (b *batch) nextOptionColor(key domain.RelationKey) string {
	palette := constant.OptionColors()
	claimed := b.optClaimed[string(key)]
	take := func() string {
		c := palette[b.optColor[string(key)]%len(palette)]
		b.optColor[string(key)]++
		return c.String()
	}
	for range palette {
		if c := take(); !claimed[c] {
			return c
		}
	}
	return take() // a vocabulary claiming all ten: reuse, in cycle order
}

// looksLikeMintedKey reports whether key has the shape of an app-minted
// internal key (a bson object id: 24 hex characters) — the population that
// owes no mint because it already is one.
func looksLikeMintedKey(key string) bool {
	if len(key) != 24 {
		return false
	}
	for _, r := range key {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// mintRelation builds a Relation object snapshot matching the shape
// core/block/import/csv and core/block/import/notion build for their own
// generated relations: Details{name, relationKey, relationFormat, layout},
// ObjectTypes = [ot-relation], Key = the property key (so the id/dedup
// pipeline in core/block/import/processor.go can recognize it across
// re-imports the same way it does for CSV/Notion relations).
func (b *batch) mintRelation(def anyblockjson.PropertyDefinition) string {
	// A property the document declared by SPELLING gets a fresh internal key
	// here, the way the app mints one when a user creates a property
	// (objectcreator/relation.go: bson.NewObjectId().Hex()). A property that
	// stated its `internal_key` keeps it exactly, which is the whole point of
	// stating one: a bundle re-imported elsewhere reproduces the same stored
	// key (§2e).
	//
	// The document's spelling is remembered as the api key, and bound in this
	// batch's vocabulary so every OTHER document that names the property by
	// the same spelling resolves to this one minted key.
	storedKey := string(def.Key)
	apiKey := ""
	if !def.KeyIsInternal {
		apiKey = storedKey
		if existing, ok := b.minted[apiKey]; ok {
			storedKey = existing
		} else {
			storedKey = bson.NewObjectId().Hex()
			b.minted[apiKey] = storedKey
		}
	}
	format := def.Format
	name := def.Name
	if fi, ok := b.formats[string(def.Key)]; ok {
		format = fi.Format
		if name == "" {
			name = fi.Name
		}
	}
	if name == "" {
		name = string(def.Key)
	}

	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, storedKey)
	if err != nil {
		// def.Key already passed through the schema's property-key charset
		// (§2a); NewUniqueKey only fails for an unsupported smartblock type.
		panic(err)
	}
	id := uk.Marshal()

	details := &types.Struct{Fields: map[string]*types.Value{
		detailID:             strVal(id),
		detailName:           strVal(name),
		detailRelationKey:    strVal(storedKey),
		detailRelationFormat: numVal(float64(format)),
		detailLayout:         numVal(float64(model.ObjectType_relation)),
	}}
	if apiKey != "" {
		// the spelling the document used, kept the way the app keeps it for a
		// property created through the API: the internal key is opaque, and
		// this is the name anything addressing the property by spelling asks
		// for (objectcreator/relation.go)
		details.Fields[detailApiObjectKey] = strVal(apiKey)
	}
	if format == model.RelationFormat_status {
		details.Fields[detailRelationMaxCount] = numVal(1)
	}
	if ids := b.objectTypeIds(def); len(ids) > 0 {
		details.Fields[detailRelationFormatObjectTypes] = strListVal(ids)
	}
	// the rest of the shared propertyDefinition shape (§2a): a definition may
	// state these, and a minted relation that shed them would make listing a
	// member in the document weaker than not listing it — the exact trap the
	// absent-format rule documents. Each writes only when declared, so a
	// definition that says nothing changes nothing.
	if def.Description != "" {
		details.Fields[detailDescription] = strVal(def.Description)
	}
	if def.IncludeTime != nil {
		details.Fields[detailRelationFormatIncludeTime] = boolVal(*def.IncludeTime)
	}
	if def.MaxCount > 0 {
		details.Fields[detailRelationMaxCount] = numVal(float64(def.MaxCount))
	}
	if def.Readonly {
		details.Fields[detailRelationReadonlyValue] = boolVal(true)
	}
	if def.DefaultValue != nil {
		details.Fields[detailRelationDefaultValue] = pbtypes.InterfaceToValue(def.DefaultValue)
	}

	snap := &model.SmartBlockSnapshotBase{
		Blocks:      rootOnlyBlocks(id),
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelation.URL()},
		Key:         string(def.Key),
	}
	b.pending = append(b.pending, pendingSnapshot{id: id, sbType: model.SmartBlockType_STRelation, snapshot: snap})
	return id
}

// mintOption builds a RelationOption object snapshot, matching the shape
// core/block/import/notion builds for select/multiSelect/status options:
// Details{name, relationKey, layout}, ObjectTypes = [ot-relationOption].
func (b *batch) mintOption(key domain.RelationKey, name, orderId, color string) string {
	localKey := optionLocalKey(key, name)
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelationOption, localKey)
	if err != nil {
		panic(err)
	}
	id := uk.Marshal()
	// the reverse entry is recorded HERE, at the one place an option object
	// comes into existence, so "this batch can name the id" and "this batch
	// carries the object" are the same statement — OptionName's liveness
	// answer is only safe while that holds.
	b.optNames[string(key)+"\x00"+id] = name

	details := &types.Struct{Fields: map[string]*types.Value{
		detailID:          strVal(id),
		detailName:        strVal(name),
		detailRelationKey: strVal(string(key)),
		detailLayout:      numVal(float64(model.ObjectType_relationOption)),
	}}
	// options sort on orderId+name concatenated (database.OrderMap.BuildOrder),
	// so every option needs an order id: without one it is compared by name
	// against everyone else's order id and lands arbitrarily. Declared
	// vocabulary comes first, discovered names after it.
	details.Fields[detailOrderId] = strVal(orderId)
	// an option's color is a detail like any other, and every creation path in
	// the app sets one (pkg/lib/schema.Relation.CreateOptionDetails); a
	// declared vocabulary says which, rather than leaving it to chance (§2a).
	details.Fields[detailRelationOptionColor] = strVal(color)

	snap := &model.SmartBlockSnapshotBase{
		Blocks:      rootOnlyBlocks(id),
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyRelationOption.URL()},
		Key:         localKey,
	}
	b.pending = append(b.pending, pendingSnapshot{id: id, sbType: model.SmartBlockType_STRelationOption, snapshot: snap})
	return id
}

// optionLocalKey derives a short, stable, charset-safe UniqueKey component
// from (property key, option name). Unlike relations, two different
// properties may share an option name ("Active"), so the property key has to
// be part of the hash, not just the name.
func optionLocalKey(key domain.RelationKey, name string) string {
	sum := sha1.Sum([]byte(string(key) + "\x00" + name))
	return hex.EncodeToString(sum[:])[:12]
}

func rootOnlyBlocks(id string) []*model.Block {
	return []*model.Block{{
		Id:      id,
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}}
}

const (
	detailID   = "id"
	detailName = "name"
	// the relation a type's templates are queried by
	// (core/block/template/templateimpl.queryTemplatesByType)
	detailTargetObjectType          = string(bundle.RelationKeyTargetObjectType)
	detailRelationKey               = "relationKey"
	detailRelationFormat            = "relationFormat"
	detailOrderId                   = "orderId"
	detailRelationOptionColor       = string(bundle.RelationKeyRelationOptionColor)
	detailRelationFormatObjectTypes = "relationFormatObjectTypes"
	detailRelationMaxCount          = "relationMaxCount"
	detailLayout                    = "layout"
	detailIsHidden                  = string(bundle.RelationKeyIsHidden)
	detailDescription               = string(bundle.RelationKeyDescription)
	detailRelationFormatIncludeTime = string(bundle.RelationKeyRelationFormatIncludeTime)
	detailRelationReadonlyValue     = string(bundle.RelationKeyRelationReadonlyValue)
	detailRelationDefaultValue      = string(bundle.RelationKeyRelationDefaultValue)
	// the spelling a property was created under, which is what an API caller
	// addresses it by when its internal key is an opaque minted id
	detailApiObjectKey = string(bundle.RelationKeyApiObjectKey)
)

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func numVal(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}

func boolVal(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func strListVal(ss []string) *types.Value {
	vals := make([]*types.Value, 0, len(ss))
	for _, s := range ss {
		vals = append(vals, strVal(s))
	}
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
}

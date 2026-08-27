package main

import (
	"crypto/sha1"
	"encoding/hex"
	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/lexid"
	"sort"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

// pendingSnapshot is a Relation/RelationOption object the batch mints the
// first time a custom property or option value is referenced.
type pendingSnapshot struct {
	id       string
	sbType   model.SmartBlockType
	snapshot *model.SmartBlockSnapshotBase
}

// batch is the cross-document wiring pkg/lib/anyblockjson deliberately
// leaves to its caller (SPEC.md calls this "the import wiring's job": "creating
// options is the wiring's job"): it answers anyblockjson.Options'
// FormatResolver/OptionResolver/PropertyResolver callbacks from the
// pre-scanned typeProperties table, lazily minting Relation/RelationOption
// snapshots the same way core/block/import/csv and core/block/import/notion
// build them for their own generated relations and options.
type batch struct {
	formats map[string]anyblockbatch.FormatInfo

	relIDs map[string]string // property key -> minted relation object id
	optIDs map[string]string // "key\x00name" -> minted option object id

	// optOrder is the last order id handed out per property key. Every option
	// needs one: options sort on orderId+name concatenated
	// (database.OrderMap.BuildOrder), so an option without an order id is
	// compared by name against everyone else's order id and lands
	// arbitrarily — before the declared vocabulary when its name sorts below
	// the lexid alphabet, after it otherwise.
	optOrder map[string]string

	// optColor is the next palette position per property key, and optClaimed
	// the colors that property's vocabulary names explicitly so the
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
func newBatch(formats map[string]anyblockbatch.FormatInfo, typeIds map[string]string) (b *batch) {
	b = &batch{
		formats:    formats,
		relIDs:     map[string]string{},
		optIDs:     map[string]string{},
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

// resolveFormat implements anyblockjson.FormatResolver.
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

// OptionName implements anyblockjson.OptionResolver. Only used on export;
// this tool only imports, so it's never called.
func (b *batch) OptionName(key domain.RelationKey, id string) (string, bool) {
	return "", false
}

// PropertyId implements anyblockjson.PropertyResolver: allocates a stable
// Relation object for a typeProperties entry the first time its key is
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

// PropertyById implements anyblockjson.PropertyResolver. Only used on
// export; this tool only imports, so it's never called.
func (b *batch) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return anyblockjson.PropertyDefinition{}, false
}

// declareOptions pre-mints the vocabulary a typeProperties entry declares,
// in declaration order, so every value exists whether or not any
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

// mintRelation builds a Relation object snapshot matching the shape
// core/block/import/csv and core/block/import/notion build for their own
// generated relations: Details{name, relationKey, relationFormat, layout},
// ObjectTypes = [ot-relation], Key = the property key (so the id/dedup
// pipeline in core/block/import/processor.go can recognize it across
// re-imports the same way it does for CSV/Notion relations).
func (b *batch) mintRelation(def anyblockjson.PropertyDefinition) string {
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

	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, string(def.Key))
	if err != nil {
		// def.Key already passed through the schema's property-key charset;
		// NewUniqueKey only fails for an unsupported smartblock type.
		panic(err)
	}
	id := uk.Marshal()

	details := &types.Struct{Fields: map[string]*types.Value{
		detailID:             strVal(id),
		detailName:           strVal(name),
		detailRelationKey:    strVal(string(def.Key)),
		detailRelationFormat: numVal(float64(format)),
		detailLayout:         numVal(float64(model.ObjectType_relation)),
	}}
	if format == model.RelationFormat_status {
		details.Fields[detailRelationMaxCount] = numVal(1)
	}
	if ids := b.objectTypeIds(def); len(ids) > 0 {
		details.Fields[detailRelationFormatObjectTypes] = strListVal(ids)
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
	// declared vocabulary says which, rather than leaving it to chance.
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
)

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func numVal(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}

func strListVal(ss []string) *types.Value {
	vals := make([]*types.Value, 0, len(ss))
	for _, s := range ss {
		vals = append(vals, strVal(s))
	}
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
}

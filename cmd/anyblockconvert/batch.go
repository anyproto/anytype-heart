package main

import (
	"crypto/sha1"
	"encoding/hex"
	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	formats map[string]anyblockbatch.FormatInfo

	relIDs map[string]string // property key -> minted relation object id
	optIDs map[string]string // "key\x00name" -> minted option object id

	pending []pendingSnapshot
}

func newBatch(formats map[string]anyblockbatch.FormatInfo) *batch {
	return &batch{
		formats: formats,
		relIDs:  map[string]string{},
		optIDs:  map[string]string{},
	}
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
	id := b.mintOption(key, name)
	b.optIDs[mapKey] = id
	return id, true
}

// OptionName implements anyblockjson.OptionResolver. Only used on export;
// this tool only imports, so it's never called.
func (b *batch) OptionName(key domain.RelationKey, id string) (string, bool) {
	return "", false
}

// PropertyId implements anyblockjson.PropertyResolver: allocates a stable
// Relation object for a typeProperties entry (§2a) the first time its key is
// seen in the batch. Bundled (system) properties resolve to their bundled
// url instead of minting a new object — installBundledRelationsAndTypes
// (core/block/import/common/objectcreator) installs those automatically.
func (b *batch) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	if bundle.HasRelation(def.Key) {
		return def.Key.BundledURL(), true
	}
	key := string(def.Key)
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
		// def.Key already passed through the schema's property-key charset
		// (§2a); NewUniqueKey only fails for an unsupported smartblock type.
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
func (b *batch) mintOption(key domain.RelationKey, name string) string {
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
	detailID               = "id"
	detailName             = "name"
	detailRelationKey      = "relationKey"
	detailRelationFormat   = "relationFormat"
	detailRelationMaxCount = "relationMaxCount"
	detailLayout           = "layout"
)

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func numVal(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}

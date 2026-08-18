package anyblockjson

// typeproperties.go maps a type document's typeProperties array (§2a) to and
// from the four recommended-relation id lists on the snapshot's details.

import (
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// PropertyDefinition describes a property (relation) object referenced by a
// type document (§2a).
type PropertyDefinition struct {
	Key    domain.RelationKey
	Name   string
	Format model.RelationFormat
	// Options is the declared vocabulary of a select/multiSelect property,
	// in display order (§2a). Options are otherwise only discovered from
	// values that happen to be used, so a vocabulary entry no record carries
	// would never exist, and minted options carry no orderId and fall back to
	// sorting by name. Empty means "whatever usage produces", the pre-options
	// behaviour.
	Options []OptionDefinition
	// ObjectTypes restricts which types an objects/files property may point
	// at, in priority order, given as **type keys** — the STORED spelling on
	// this struct; the document spells the slug, and the codec translates at
	// the boundary like every other key slot (§7.5a). Empty means any
	// object, which is also what an untargeted property accepts — a task
	// could be assigned to a random page. Listing the built-in `participant`
	// alongside a bundle's own people type is what makes the current-user
	// filter value available on the property (§6.2) while still allowing the
	// seeded people as values.
	ObjectTypes []string
}

// OptionDefinition is one entry of a declared select vocabulary (§2a). Color
// is an Anytype option color name (util/constant.OptionColors); empty leaves
// the choice to the import wiring, which is why the canonical JSON form of a
// colorless option is the bare name rather than an object.
//
// The color belongs to the option rather than to a parallel array on the
// property so that inserting or reordering an option cannot shift it — the
// silent-failure class SPEC goal 2 exists to avoid.
type OptionDefinition struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// UnmarshalJSON accepts both §2a forms: a bare name, or an object carrying a
// color. Same shape as jsonCell (§6.1), minus the null and array arms.
func (o *OptionDefinition) UnmarshalJSON(data []byte) error {
	if strings.HasPrefix(strings.TrimSpace(string(data)), `"`) {
		return jsonUnmarshal(data, &o.Name)
	}
	type plain OptionDefinition // shed this method, or it recurses
	return jsonUnmarshal(data, (*plain)(o))
}

// optionsToAny renders a declared vocabulary for export: the bare name when
// the option carries no color, an object otherwise. The string form is
// canonical whenever it qualifies, as for table cells (§6.1).
func optionsToAny(opts []OptionDefinition) []any {
	var out []any
	for _, o := range opts {
		if o.Name == "" {
			continue
		}
		if o.Color == "" {
			out = append(out, o.Name)
			continue
		}
		m := &omap{}
		m.set("name", o.Name)
		m.set("color", o.Color)
		out = append(out, m)
	}
	return out
}

// optionEntryName reads the name out of either §2a option form. The semantic
// checks (§12) run on the raw document, before it decodes into
// OptionDefinition, so they need this rather than the struct.
func optionEntryName(entry any) string {
	switch e := entry.(type) {
	case string:
		return e
	case map[string]any:
		name, _ := e["name"].(string)
		return name
	}
	return ""
}

// PropertyResolver maps property object ids to definitions on export and
// definitions back to ids on import. Creating missing properties is the
// import wiring's job (the OptionResolver contract, §3): PropertyId receives
// the full definition so the wiring can create-and-return in one step.
type PropertyResolver interface {
	PropertyById(id string) (PropertyDefinition, bool)
	PropertyId(def PropertyDefinition) (string, bool)
}

// recommendedListKeys are the four detail keys typeProperties replaces, in
// the §2a canonical section order. The empty section is the regular
// (sidebar) list.
var recommendedListKeys = []struct {
	detailKey string
	section   string
}{
	{"recommendedFeaturedRelations", "featured"},
	{"recommendedRelations", ""},
	{"recommendedFileRelations", "file"},
	{"recommendedHiddenRelations", "hidden"},
}

// typePropsActive reports whether this export rewrites the recommended lists
// into typeProperties: only type documents, and only with a resolver — ids
// are space-local, so without one the lists pass through in properties as
// raw ids (the same degradation as options without an option resolver).
func (e *exporter) typePropsActive() bool {
	return e.sbType == model.SmartBlockType_STType && e.opts.ResolveProperties != nil
}

// typePropDetailKeys returns the detail keys hidden from properties (and from
// the §9a legend) because typeProperties carries them, or nil when inactive.
func (e *exporter) typePropDetailKeys() map[string]bool {
	if !e.typePropsActive() {
		return nil
	}
	skip := make(map[string]bool, len(recommendedListKeys))
	for _, l := range recommendedListKeys {
		skip[l.detailKey] = true
	}
	return skip
}

// buildTypeProperties renders the §2a array: sections in canonical order,
// source order preserved within each list, unresolvable ids dropped. The
// array is emitted even when empty — its presence tells import to rebuild
// the four lists (as explicit empty lists) rather than leave them absent.
func (e *exporter) buildTypeProperties() []any {
	if !e.typePropsActive() {
		return nil
	}
	out := []any{}
	for _, l := range recommendedListKeys {
		for _, id := range valueStringList(e.detail(l.detailKey)) {
			def, ok := e.resolveTypeProperty(id)
			if !ok || def.Key == "" {
				continue
			}
			m := &omap{}
			m.set("key", e.propertySlug(string(def.Key)))
			m.setNonEmpty("name", def.Name)
			m.setNonEmpty("format", formatName(def.Format))
			m.setNonEmpty("options", optionsToAny(def.Options))
			// object_types is a TYPE key slot (§7.5a) — it names types, so it
			// speaks the same vocabulary the envelope `type` does
			m.setNonEmpty("object_types", stringsToAny(e.opts.typeSlugs(def.ObjectTypes)))
			m.setNonEmpty("section", l.section)
			out = append(out, m)
		}
	}
	return out
}

// resolveTypeProperty resolves one recommended-list entry. Entries are
// normally property object ids, but legacy type objects store bare property
// KEYS (e.g. "creator") in these lists — those resolve via the reverse
// lookup or, for system properties, the bundle.
func (e *exporter) resolveTypeProperty(id string) (PropertyDefinition, bool) {
	r := e.opts.ResolveProperties
	if def, ok := r.PropertyById(id); ok {
		return def, true
	}
	key := domain.RelationKey(id)
	if rid, ok := r.PropertyId(PropertyDefinition{Key: key}); ok {
		if def, ok := r.PropertyById(rid); ok {
			return def, true
		}
		return PropertyDefinition{Key: key}, true
	}
	if rel, err := bundle.GetRelation(key); err == nil {
		return PropertyDefinition{Key: key, Name: rel.Name, Format: rel.Format}, true
	}
	return PropertyDefinition{}, false
}

// TypeProperty is one §2a typeProperties entry in its JSON shape — exported
// so API wiring can accept typeProperties outside a full document (the
// PATCH type surface).
type TypeProperty struct {
	Key         string             `json:"key"`
	Name        string             `json:"name"`
	Format      string             `json:"format"`
	Options     []OptionDefinition `json:"options"`
	ObjectTypes []string           `json:"object_types"`
	Section     string             `json:"section"`
}

type jsonTypeProperty = TypeProperty

// RecommendedList is one of the four §2a recommended-relation lists in
// detail-key form, produced by BuildRecommendedLists.
type RecommendedList struct {
	DetailKey string
	Ids       []string
}

// BuildRecommendedLists resolves a typeProperties array into the four
// recommended-relation id lists (§2a), in canonical section order. All four
// lists are always present — type objects store empty sections as explicit
// empty lists. Keys the resolver cannot (or, on a dry run, will not) resolve
// pass through in place of ids, the same degradation as import (§2a). It
// carries the declared vocabulary and target types through to the resolver,
// so a property minted here is created with the same shape import gives it.
//
// It takes the full Options rather than a bare resolver because a
// typeProperties array carries KEY SLOTS — `key` and `objectTypes` — and this
// is the PATCH channel for the same array `applyTypeProperties` reads out of a
// document. Both must invert through the same vocabulary, or the two ways of
// writing one type's property list disagree about what a key means.
func BuildRecommendedLists(props []TypeProperty, opts Options) []RecommendedList {
	bySection := map[string][]string{}
	for _, tp := range props {
		key := opts.propertyKey(tp.Key)
		id := key
		if opts.ResolveProperties != nil {
			def := PropertyDefinition{
				Key:         domain.RelationKey(key),
				Name:        tp.Name,
				Format:      formatNames.value(tp.Format),
				Options:     tp.Options,
				ObjectTypes: opts.typeKeys(tp.ObjectTypes),
			}
			if resolved, ok := opts.ResolveProperties.PropertyId(def); ok {
				id = resolved
			}
		}
		bySection[tp.Section] = append(bySection[tp.Section], id)
	}
	out := make([]RecommendedList, 0, len(recommendedListKeys))
	for _, l := range recommendedListKeys {
		ids := bySection[l.section]
		if ids == nil {
			ids = []string{}
		}
		out = append(out, RecommendedList{DetailKey: l.detailKey, Ids: ids})
	}
	return out
}

// applyTypeProperties rebuilds the four recommended-relation lists from the
// document's typeProperties (§2a). Definitions resolve to property ids via
// the resolver; without one — or on a miss the wiring chose not to create —
// the key passes through in place of an id for the wiring to reconcile. The
// field's presence (even as an empty array) is the trigger: absent means the
// document does not carry the lists at all.
func (imp *importer) applyTypeProperties(details *types.Struct) {
	if imp.doc.TypeProps == nil {
		return
	}
	lists := map[string][]*types.Value{}
	for _, tp := range *imp.doc.TypeProps {
		key := imp.propertyKey(tp.Key)
		def := PropertyDefinition{
			Key:         domain.RelationKey(key),
			Name:        tp.Name,
			Format:      imp.declaredFormat(key, tp.Format),
			Options:     tp.Options,
			ObjectTypes: imp.opts.typeKeys(tp.ObjectTypes),
		}
		id := key
		if imp.opts.ResolveProperties != nil {
			if resolved, ok := imp.opts.ResolveProperties.PropertyId(def); ok {
				id = resolved
			}
		}
		lists[tp.Section] = append(lists[tp.Section],
			&types.Value{Kind: &types.Value_StringValue{StringValue: id}})
	}
	// all four lists are written, empty ones included: type objects carry
	// them as explicit empty lists, and leaving a key absent would break
	// export∘import byte-stability for empty sections
	for _, l := range recommendedListKeys {
		vals := lists[l.section]
		if vals == nil {
			vals = []*types.Value{}
		}
		details.Fields[l.detailKey] = &types.Value{
			Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}},
		}
	}
}

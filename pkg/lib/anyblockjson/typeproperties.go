package anyblockjson

// typeproperties.go maps a type document's typeProperties array (§2a) to and
// from the four recommended-relation id lists on the snapshot's details.

import (
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
			m.set("key", string(def.Key))
			m.setNonEmpty("name", def.Name)
			m.setNonEmpty("format", formatNames.name(def.Format))
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
	Key     string `json:"key"`
	Name    string `json:"name,omitempty"`
	Format  string `json:"format,omitempty"`
	Section string `json:"section,omitempty"`
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
// pass through in place of ids, the same degradation as import (§2a).
func BuildRecommendedLists(props []TypeProperty, resolver PropertyResolver) []RecommendedList {
	bySection := map[string][]string{}
	for _, tp := range props {
		id := tp.Key
		if resolver != nil {
			def := PropertyDefinition{
				Key:    domain.RelationKey(tp.Key),
				Name:   tp.Name,
				Format: formatNames.value(tp.Format),
			}
			if resolved, ok := resolver.PropertyId(def); ok {
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
		def := PropertyDefinition{
			Key:    domain.RelationKey(tp.Key),
			Name:   tp.Name,
			Format: formatNames.value(tp.Format),
		}
		id := tp.Key
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

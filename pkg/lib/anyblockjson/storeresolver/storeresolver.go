// Package storeresolver provides objectstore-backed resolvers for the
// anyblockjson Options: property formats, select/multiSelect option names,
// and property definitions, all read from one space's index. It is the
// standard wiring for exports/imports that run inside a full node
// (API v2 reads, cmd/anyblockroundtrip).
package storeresolver

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Resolvers implements anyblockjson.OptionResolver and
// anyblockjson.PropertyResolver over a space index, with per-instance
// caching. Instances are not safe for concurrent use; create one per
// export/import operation.
type Resolvers struct {
	index      spaceindex.Store
	optionsFor map[domain.RelationKey][]*model.RelationOption

	relsLoaded bool
	relById    map[string]anyblockjson.PropertyDefinition
	relKeyToId map[string]string

	// the §7.5a key vocabulary, primed lazily — see keyvocab.go
	relVocab  *keyMaps
	typeVocab *keyMaps
}

// New creates resolvers over one space's index.
func New(index spaceindex.Store) *Resolvers {
	return &Resolvers{index: index, optionsFor: map[domain.RelationKey][]*model.RelationOption{}}
}

// Options returns anyblockjson.Options pre-wired with the resolvers; callers
// set the remaining fields (compaction flags etc.) on the returned value.
func (r *Resolvers) Options() anyblockjson.Options {
	return anyblockjson.Options{
		ResolveFormat:     r.ResolveFormat,
		ResolveOptions:    r,
		ResolveProperties: r,
		Keys:              r,
	}
}

// loadRelations snapshots the space's relation objects once: the point
// lookups (GetRelationByKey) miss for some legacy relations that the full
// listing still returns, so the map is the primary source and the point
// lookups are the fallback.
func (r *Resolvers) loadRelations() {
	if r.relsLoaded {
		return
	}
	r.relsLoaded = true
	r.relById = map[string]anyblockjson.PropertyDefinition{}
	r.relKeyToId = map[string]string{}
	rels, err := r.index.ListAllRelations()
	if err != nil {
		return
	}
	for _, rel := range rels {
		if rel == nil || rel.Relation == nil {
			continue
		}
		def := anyblockjson.PropertyDefinition{
			Key:    domain.RelationKey(rel.Key),
			Name:   rel.Name,
			Format: rel.Format,
		}
		r.relById[rel.Id] = def
		if _, taken := r.relKeyToId[rel.Key]; !taken {
			r.relKeyToId[rel.Key] = rel.Id
		}
	}
}

// ResolveFormat implements anyblockjson.FormatResolver.
func (r *Resolvers) ResolveFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	rel, err := r.index.GetRelationByKey(string(key))
	if err != nil || rel == nil {
		return 0, false
	}
	return rel.Format, true
}

func (r *Resolvers) options(key domain.RelationKey) []*model.RelationOption {
	if cached, ok := r.optionsFor[key]; ok {
		return cached
	}
	opts, err := r.index.ListRelationOptions(key)
	if err != nil {
		opts = nil
	}
	r.optionsFor[key] = opts
	return opts
}

// OptionName implements anyblockjson.OptionResolver.
func (r *Resolvers) OptionName(key domain.RelationKey, id string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Id == id {
			return o.Text, true
		}
	}
	return "", false
}

// OptionId implements anyblockjson.OptionResolver.
func (r *Resolvers) OptionId(key domain.RelationKey, name string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Text == name {
			return o.Id, true
		}
	}
	return "", false
}

// PropertyById implements anyblockjson.PropertyResolver.
func (r *Resolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	r.loadRelations()
	if def, ok := r.relById[id]; ok {
		return def, true
	}
	rel, err := r.index.GetRelationById(id)
	if err != nil || rel == nil {
		return anyblockjson.PropertyDefinition{}, false
	}
	def := anyblockjson.PropertyDefinition{
		Key:    domain.RelationKey(rel.Key),
		Name:   rel.Name,
		Format: rel.Format,
	}
	// cache the point-lookup hit both ways: some relations resolve by id but
	// are absent from the listing AND the by-key lookup (deleted or index
	// gap — anomaly #9 class), so without this PropertyId cannot invert the
	// key export just produced and the entry is dropped on re-export
	// (resolvers must be equivalent both directions, SPEC §2a/§13)
	r.relById[id] = def
	if _, taken := r.relKeyToId[rel.Key]; !taken {
		r.relKeyToId[rel.Key] = id
	}
	return def, true
}

// PropertyId implements anyblockjson.PropertyResolver.
func (r *Resolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	r.loadRelations()
	if id, ok := r.relKeyToId[string(def.Key)]; ok {
		return id, true
	}
	rel, err := r.index.GetRelationByKey(string(def.Key))
	if err != nil || rel == nil {
		return "", false
	}
	r.relKeyToId[rel.Key] = rel.Id
	return rel.Id, true
}

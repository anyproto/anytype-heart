package v2service

// resolver.go is the Phase-2 resolver wiring (APIV2.md §2 "the substance
// of this phase"): it bridges anyblockjson's OptionResolver/PropertyResolver
// to the objectstore (read side, via storeresolver) and to the create RPCs
// (write side) — unknown select option NAMES and unknown property KEYS are
// created on import, per SPEC §3 (options) and §2a (typeProperties).
//
// The create-vs-reject policy, explicit (APIV2.md Phase-2 item on referential
// validation):
//   - select/multiSelect option names        → create-missing (SPEC §3)
//   - property keys in typeProperties        → create-missing (SPEC §2a)
//   - property keys in an object's properties → reject with did-you-mean
//   - type keys                              → reject with did-you-mean
// On a dry run (C9) nothing is created; would-be creations are recorded and
// reported in the result.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/types"
)

type optionRef struct {
	property string
	name     string
}

// creatingResolvers implements anyblockjson.OptionResolver and
// anyblockjson.PropertyResolver with create-missing semantics over one
// space. Reads go through storeresolver; misses create the missing option/
// property via the middleware RPCs (or, on a dry run, only record the
// would-be creation). Instances are per-request and not safe for concurrent
// use — same contract as storeresolver.
type creatingResolvers struct {
	ctx     context.Context
	mw      apicore.ClientCommands
	svc     *V2Service
	spaceId string
	reads   *storeresolver.Resolvers
	dryRun  bool

	// liveEntries is the once-per-resolver snapshot of the space's live
	// properties (keys.go) — the slug namespace the §7.5a-5 chain and the
	// mint-time union check resolve against
	liveEntries     []propertyEntry
	livePropsLoaded bool

	createdOptions map[optionRef]string
	// dryReported are option refs already reported as would-be-created on a
	// dry run, so resolving the same name twice (prewarm, then the op) does
	// not list it twice (review C′2).
	dryReported    map[optionRef]bool
	createdProps   map[string]anyblockjson.PropertyDefinition // key → created def
	createdPropIds map[string]string                          // key → id
	sideEffects    v2model.SideEffects
	errs           []error
}

func (s *V2Service) newCreatingResolvers(ctx context.Context, spaceId string, dryRun bool) *creatingResolvers {
	return &creatingResolvers{
		ctx:            ctx,
		mw:             s.mw,
		svc:            s,
		spaceId:        spaceId,
		reads:          storeresolver.New(s.store.SpaceIndex(spaceId)),
		dryRun:         dryRun,
		createdOptions: map[optionRef]string{},
		dryReported:    map[optionRef]bool{},
		createdProps:   map[string]anyblockjson.PropertyDefinition{},
		createdPropIds: map[string]string{},
	}
}

// Options returns anyblockjson import options wired with the creating
// resolvers.
func (r *creatingResolvers) Options() anyblockjson.Options {
	return anyblockjson.Options{
		ResolveFormat:     r.ResolveFormat,
		ResolveOptions:    r,
		ResolveProperties: r,
	}
}

// err aggregates create failures hit during resolution; the resolver
// interfaces have no error channel, so Unmarshal callers must check it.
func (r *creatingResolvers) err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
}

// created reports the side effects (real or would-be) for the response;
// nil when nothing was created.
func (r *creatingResolvers) created() *v2model.SideEffects {
	if len(r.sideEffects.Properties) == 0 && len(r.sideEffects.Options) == 0 {
		return nil
	}
	out := r.sideEffects
	return &out
}

// ResolveFormat implements anyblockjson.FormatResolver: properties created
// earlier in the same import resolve too.
func (r *creatingResolvers) ResolveFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	if def, ok := r.createdProps[string(key)]; ok {
		return def.Format, true
	}
	return r.reads.ResolveFormat(key)
}

// OptionName implements anyblockjson.OptionResolver (export side; unused on
// import but required by the interface).
func (r *creatingResolvers) OptionName(key domain.RelationKey, id string) (string, bool) {
	return r.reads.OptionName(key, id)
}

// OptionId implements anyblockjson.OptionResolver with create-missing:
// unknown names become new options of the property (SPEC §3 — the CSV/Notion
// importer behavior).
func (r *creatingResolvers) OptionId(key domain.RelationKey, name string) (string, bool) {
	ref := optionRef{property: string(key), name: name}
	if id, ok := r.createdOptions[ref]; ok {
		return id, true
	}
	if id, ok := r.reads.OptionId(key, name); ok {
		return id, true
	}
	if r.dryRun {
		// nothing is created; the name passes through verbatim in the
		// discarded snapshot. Report it once: prewarm and the op itself both
		// resolve the same name, and dry_run must preview exactly what the
		// real run reports (review C′2).
		if !r.dryReported[ref] {
			r.dryReported[ref] = true
			r.sideEffects.Options = append(r.sideEffects.Options, v2model.CreatedOption{Property: string(key), Name: name})
		}
		return "", false
	}
	r.sideEffects.Options = append(r.sideEffects.Options, v2model.CreatedOption{Property: string(key), Name: name})
	resp := r.mw.ObjectCreateRelationOption(r.ctx, &pb.RpcObjectCreateRelationOptionRequest{
		SpaceId: r.spaceId,
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String(): pbtypes.String(string(key)),
			bundle.RelationKeyName.String():        pbtypes.String(name),
			bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
		}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationOptionResponseError_NULL {
		r.errs = append(r.errs, fmt.Errorf("create option %q of property %q: %s", name, key, resp.Error.Description))
		return "", false
	}
	r.createdOptions[ref] = resp.ObjectId
	return resp.ObjectId, true
}

// prewarmCreateMissing resolves a PATCH's create-missing references BEFORE
// the object lock is taken (review B6/A6): the create surfaces in PATCH
// payloads are setProperties select/multiSelect option names and the option
// names an updateView filter or custom sort order carries, so those are
// resolved (and, on a real run, created) here; in-lock resolution then hits
// the resolver's cache and never fires a create-RPC while holding the edited
// object's lock. The M5 bound counts what this pass records, so a channel
// skipped here would also be a channel the too-many-options cap cannot see.
// The scan is deliberately lenient — every validation error still surfaces
// from the in-lock op pass, in unchanged order — and create failures ride
// resolvers.err(), exactly where the in-lock path checks them.
func (s *V2Service) prewarmCreateMissing(ops []json.RawMessage, resolvers *creatingResolvers) {
	for _, raw := range ops {
		var probe struct {
			Op  string                     `json:"op"`
			Set map[string]json.RawMessage `json:"set"`
			// add resolves option names with the same create-missing
			// semantics as set (v0.3.5), so it prewarms too — otherwise
			// option creation moves back inside the object lock
			Add map[string]json.RawMessage `json:"add"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if probe.Op == "updateView" || probe.Op == "insertView" {
			// both carry the same set channel — the ONLY channel through which
			// a view op may introduce option names. Everything else a view op
			// serializes (untouched views, copyFrom clones) is restored from
			// the live proto at commit and imported with a no-create resolver
			// (viewops.go commitDataviewBlock), so this prewarm is exhaustive:
			// a name it cannot see is a name the commit will not mint.
			s.prewarmViewOptionValues(probe.Set, resolvers)
			continue
		}
		if probe.Op != "setProperties" {
			continue
		}
		for field, values := range map[string]map[string]json.RawMessage{"set": probe.Set, "add": probe.Add} {
			for rawKey, rawValue := range values {
				// A key claimed by more than one field is rejected by the apply
				// path, so prewarming it would create an option for a PATCH
				// that cannot succeed. Skip it here and let the op error.
				if field == "add" {
					if _, alsoInSet := probe.Set[rawKey]; alsoInSet {
						continue
					}
				}
				// §7.5a-5: a slug-spelled term canonicalizes to its stored
				// key here too, or the prewarm would miss the channel AND a
				// later create would bind the option to the slug string
				key := resolvers.canonicalPropertyKey(rawKey)
				format, err := bundle.GetRelationFormat(domain.RelationKey(key))
				if err != nil {
					var ok bool
					if format, ok = resolvers.ResolveFormat(domain.RelationKey(key)); !ok {
						continue
					}
				}
				if format != model.RelationFormat_status && format != model.RelationFormat_tag {
					continue
				}
				var value any
				if err := json.Unmarshal(rawValue, &value); err != nil {
					continue
				}
				// add always takes an array; a scalar is rejected by the apply
				// path, so prewarming it would orphan an option (v0.3.5 review).
				if field == "add" {
					if _, isList := value.([]any); !isList {
						continue
					}
				}
				anyblockjson.UnmarshalPropertyValue(key, value, resolvers.Options())
			}
		}
	}
}

// viewFilterProbe decodes a §6.2 filter tree just deep enough to reach the
// select values a leaf may carry.
type viewFilterProbe struct {
	Property  string            `json:"property"`
	Condition string            `json:"condition"`
	Value     json.RawMessage   `json:"value"`
	Filters   []viewFilterProbe `json:"filters"`
}

// prewarmViewOptionValues resolves the option names an updateView's set
// channel carries — filter leaf values and custom sort orders on select
// properties, both of which the dataview import resolves with create-missing
// (SPEC §6.2/§3) — so the creates run before the object lock and the M5
// bound sees them. Same leniency contract as the setProperties pass.
func (s *V2Service) prewarmViewOptionValues(set map[string]json.RawMessage, resolvers *creatingResolvers) {
	resolveSelect := func(rawKey string, value json.RawMessage) {
		key := resolvers.canonicalPropertyKey(rawKey)
		format, err := bundle.GetRelationFormat(domain.RelationKey(key))
		if err != nil {
			var ok bool
			if format, ok = resolvers.ResolveFormat(domain.RelationKey(key)); !ok {
				return
			}
		}
		if format != model.RelationFormat_status && format != model.RelationFormat_tag {
			return
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil || decoded == nil {
			return
		}
		anyblockjson.UnmarshalPropertyValue(key, decoded, resolvers.Options())
	}
	var walkFilters func(nodes []viewFilterProbe)
	walkFilters = func(nodes []viewFilterProbe) {
		for _, node := range nodes {
			if len(node.Filters) > 0 {
				walkFilters(node.Filters)
				continue
			}
			// empty/notEmpty/exists leaves carry no meaningful value (§11) —
			// resolving one here would create an option the view never uses
			switch node.Condition {
			case "empty", "notEmpty", "exists":
				continue
			}
			if node.Property != "" && len(node.Value) > 0 {
				resolveSelect(node.Property, node.Value)
			}
		}
	}
	if raw, ok := set["filters"]; ok {
		var nodes []viewFilterProbe
		if err := json.Unmarshal(raw, &nodes); err == nil {
			walkFilters(nodes)
		}
	}
	if raw, ok := set["filter"]; ok {
		// the compact string parses to the same structured array; parse errors
		// are the apply pass's to report — here they just mean nothing to warm
		var str string
		if err := json.Unmarshal(raw, &str); err == nil && str != "" {
			parsed, err := filterstring.Parse(str, filterstring.Options{
				ResolveFormat: s.formatNameResolver(resolvers.spaceId),
			})
			if err == nil {
				var nodes []viewFilterProbe
				if err := json.Unmarshal(parsed, &nodes); err == nil {
					walkFilters(nodes)
				}
			}
		}
	}
	if raw, ok := set["sorts"]; ok {
		var sorts []struct {
			Property    string          `json:"property"`
			CustomOrder json.RawMessage `json:"customOrder"`
		}
		if err := json.Unmarshal(raw, &sorts); err == nil {
			for _, sort := range sorts {
				if sort.Property != "" && len(sort.CustomOrder) > 0 {
					resolveSelect(sort.Property, sort.CustomOrder)
				}
			}
		}
	}
}

// readOnlyOptionResolver is the no-create view of a creatingResolvers: names
// resolve through the prewarm's create cache and the store, and a miss
// passes through verbatim instead of creating. The view-op commit imports
// whole dataview blocks with it (§8.19-A): the block re-serializes content
// the op never authored — a dangling option reference exported as its raw
// id, a twin option's shared name — and a creating resolver there minted
// options UNDER THE OBJECT LOCK, past both halves of M5.
type readOnlyOptionResolver struct {
	r *creatingResolvers
}

func (ro readOnlyOptionResolver) OptionName(key domain.RelationKey, id string) (string, bool) {
	return ro.r.OptionName(key, id)
}

func (ro readOnlyOptionResolver) OptionId(key domain.RelationKey, name string) (string, bool) {
	if id, ok := ro.r.createdOptions[optionRef{property: string(key), name: name}]; ok {
		return id, true
	}
	return ro.r.reads.OptionId(key, name)
}

// PropertyById implements anyblockjson.PropertyResolver (export side).
func (r *creatingResolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return r.reads.PropertyById(id)
}

// PropertyId implements anyblockjson.PropertyResolver with create-missing:
// an unknown key in a type document's typeProperties creates the property
// (SPEC §2a). Resolution walks the §7.5a-5 chain — exact stored key, then
// the space's slug namespace (apiObjectKey), then the bundled derived table
// — and only a full miss creates. The create follows the (a) identity layer
// (ADDRESSING §7.5): a BUNDLED key keeps the derived install path
// (convergence is the install mechanism, §2.4-1); a custom key mints a BSON
// internal key and the document's key becomes the apiObjectKey slug —
// never the stored relation key, whose derivation is what made concurrent
// same-key creates merge silently and delete-then-recreate a dead end.
func (r *creatingResolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	if id, ok := r.createdPropIds[string(def.Key)]; ok {
		return id, true
	}
	// chain step 1: exact stored-key match
	if id, ok := r.reads.PropertyId(def); ok {
		return id, true
	}
	// chain step 2: the space's slug namespace — a live property whose
	// apiObjectKey equals the document's key IS that key's referent (a
	// v2-created property's stored key is BSON; its slug is its address).
	// Two live holders of one slug is the (a) failure shape: loud, never
	// resolved by store order.
	if entry, ok, err := r.slugLookup(string(def.Key)); err != nil {
		r.errs = append(r.errs, err)
		return "", false
	} else if ok {
		r.createdPropIds[string(def.Key)] = entry.Id
		return entry.Id, true
	}
	// chain step 3: the bundled derived table — `due_date` names bundled
	// dueDate; rewrite and retry so the install path (not a twin create)
	// serves it
	bundledKey := def.Key
	if key, ok := bundle.RelationKeyByApiSlug(string(def.Key)); ok && !bundle.HasRelation(def.Key) {
		bundledKey = key
		if id, ok := r.reads.PropertyId(anyblockjson.PropertyDefinition{Key: key}); ok {
			r.createdPropIds[string(def.Key)] = id
			return id, true
		}
	}

	name, format := def.Name, def.Format
	isBundled := false
	if rel, err := bundle.GetRelation(bundledKey); err == nil {
		isBundled = true
		if name == "" {
			name = rel.Name
		}
		if format == 0 {
			format = rel.Format
		}
	}
	if name == "" {
		name = string(def.Key)
	}

	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(name),
		bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(format)),
		bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}}
	reportedKey := string(def.Key)
	if isBundled {
		// bundled: keep the derived key — every device deriving rel-<key>
		// converges on the installed object, which is the intent
		details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(string(bundledKey))
		reportedKey = string(bundledKey)
	} else {
		// custom: mint-time union collision check on the NORMALIZED slug
		// (the check ships WITH the mint, §7.6-3) — `myKey` must not mint
		// slug my_key over an existing holder
		slug := bundle.ApiSlug(string(def.Key))
		if slug != string(def.Key) {
			if _, ok, err := r.slugLookup(slug); err != nil {
				r.errs = append(r.errs, err)
				return "", false
			} else if ok {
				r.errs = append(r.errs, fmt.Errorf("create property %q: its key %q is already taken by another property — reference it by that key", def.Key, slug))
				return "", false
			}
			if _, ok := bundle.RelationKeyByApiSlug(slug); ok {
				r.errs = append(r.errs, fmt.Errorf("create property %q: its key %q is a bundled property — reference it as %q", def.Key, slug, slug))
				return "", false
			}
		}
		if slug != "" {
			details.Fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(slug)
			reportedKey = slug
		}
	}

	r.sideEffects.Properties = append(r.sideEffects.Properties, v2model.PropertyRow{
		Key:    reportedKey,
		Name:   name,
		Format: anyblockjson.FormatName(format),
	})
	if r.dryRun {
		return "", false
	}
	resp := r.mw.ObjectCreateRelation(r.ctx, &pb.RpcObjectCreateRelationRequest{
		SpaceId: r.spaceId,
		Details: details,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		r.errs = append(r.errs, fmt.Errorf("create property %q: %s", def.Key, resp.Error.Description))
		return "", false
	}
	// cache under the DOCUMENT key: later references in the same import
	// resolve to the same relation whatever its stored key is
	r.createdProps[string(def.Key)] = anyblockjson.PropertyDefinition{Key: def.Key, Name: name, Format: format}
	r.createdPropIds[string(def.Key)] = resp.ObjectId
	return resp.ObjectId, true
}

// slugLookup resolves a slug against the space's LIVE apiObjectKey namespace
// (corpses vacated — §8-OQ2). Exactly one holder resolves; two or more is
// the loud ambiguity the design demands (twin slugs from concurrent creates
// are a naming problem, and naming problems must never be settled by store
// order — the D2 lesson).
func (r *creatingResolvers) slugLookup(slug string) (propertyEntry, bool, error) {
	entries := r.liveProps()
	var matches []propertyEntry
	for _, entry := range entries {
		if entry.Slug == slug && entry.Slug != "" {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return propertyEntry{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%q (id %s)", m.Name, m.Id))
		}
		return propertyEntry{}, false, fmt.Errorf("property key %q is ambiguous — held by %s; address the intended property by its id-listed key", slug, strings.Join(names, " and "))
	}
}

// canonicalPropertyKey maps an inbound term to its canonical stored key for
// prewarm purposes (§7.5a-5): bundled and exact stored keys stay verbatim; a
// live-slug term becomes its holder's stored key; a bundled-slug term its
// bundled key. Ambiguity resolves to nothing here — the in-lock op pass owns
// the loud 400 — and a full miss passes through verbatim.
func (r *creatingResolvers) canonicalPropertyKey(key string) string {
	if _, ok := r.createdProps[key]; ok {
		return key
	}
	if bundle.HasRelation(domain.RelationKey(key)) {
		return key
	}
	if _, ok := r.reads.ResolveFormat(domain.RelationKey(key)); ok {
		return key // exact stored key
	}
	if entry, ok, err := r.slugLookup(key); err == nil && ok {
		return entry.Key
	}
	if bundledKey, ok := bundle.RelationKeyByApiSlug(key); ok {
		return string(bundledKey)
	}
	return key
}

// liveProps primes the live-property snapshot once per resolver instance
// (the one-bounded-query-per-request discipline, ADDRESSING §7.5a-2).
func (r *creatingResolvers) liveProps() []propertyEntry {
	if r.livePropsLoaded {
		return r.liveEntries
	}
	r.livePropsLoaded = true
	r.liveEntries = r.svc.liveProperties(r.spaceId)
	return r.liveEntries
}

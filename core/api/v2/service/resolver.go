package v2service

// resolver.go is the write-side resolver wiring: it bridges anyblockjson's
// OptionResolver/PropertyResolver
// to the objectstore (read side, via storeresolver) and to the create RPCs
// (write side) — unknown select option NAMES and unknown property KEYS are
// created on import, per SPEC §3 (options) and §2a (typeProperties).
//
// The create-vs-reject policy is explicit:
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
	"sort"
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
	svc     *Service
	spaceId string
	reads   *storeresolver.Resolvers
	// keys is the slug vocabulary over reads (apikeyvocab.go), built once
	// per resolver in Options().
	keys   *apiKeyVocab
	dryRun bool
	// createMissingOptions is the caller's explicit ?create_missing_options=true consent.
	// Without it an unmatched select NAME is a refusal, not a mint (A2).
	createMissingOptions bool

	// liveEntries is the once-per-resolver snapshot of the space's live
	// properties (keys.go) — the slug namespace the §7.5a-5 chain and the
	// mint-time union check resolve against; livePropsErr fails write paths
	// closed when the snapshot could not load
	liveEntries     []propertyEntry
	livePropsErr    error
	livePropsLoaded bool
	// mintedSlugs are the slugs THIS request already minted (doc key by
	// slug): the live snapshot predates them, so without this a document
	// declaring two spellings of one key minted both (review cause 4)
	mintedSlugs map[string]string

	createdOptions map[optionRef]string
	// ambiguousOptions prevents repeated prewarm/apply resolution of the same
	// duplicate label from appending the identical resolver error twice.
	ambiguousOptions map[optionRef]bool
	// dryReported are option refs already reported as would-be-created on a
	// dry run, so resolving the same name twice (prewarm, then the op) does
	// not list it twice (review C′2).
	dryReported    map[optionRef]bool
	createdProps   map[string]anyblockjson.PropertyDefinition // key → created def
	createdPropIds map[string]string                          // key → id
	sideEffects    v2model.SideEffects
	errs           []error

	// echoPropertyIds are relation object ids the PATCHed type's recommended
	// lists ALREADY carry (UpdateType primes them; empty on create). They are
	// the typeProperties twin of §8.17's in-document escape: a REMOVED bundled
	// key whose holder is already referenced resolves as an identity echo —
	// refusing it would turn the documented read-modify-write loop into a
	// forced deletion of the reference (§8.34) — while the same key on a type
	// that does NOT reference it is a new landing on a removed property and
	// refuses like every other channel (§8.41).
	echoPropertyIds map[string]bool

	// removedBundled is the space's bundled-removal set (bundledRemovalSet),
	// primed lazily and only when a bundled key actually reaches PropertyId.
	removedBundled       map[string]bool
	removedBundledLoaded bool
	removedBundledErr    error
}

// removedBundledKeys primes the resolver's removal-set snapshot once. Fails
// closed: an unreadable removal set must not read as "nothing was removed".
func (r *creatingResolvers) removedBundledKeys() (map[string]bool, error) {
	if !r.removedBundledLoaded {
		r.removedBundledLoaded = true
		r.removedBundled, r.removedBundledErr = r.svc.bundledRemovalSet(r.spaceId)
	}
	return r.removedBundled, r.removedBundledErr
}

func (s *Service) newCreatingResolvers(ctx context.Context, spaceId string, dryRun, createMissingOptions bool) *creatingResolvers {
	return &creatingResolvers{
		ctx:                  ctx,
		mw:                   s.mw,
		svc:                  s,
		spaceId:              spaceId,
		reads:                storeresolver.New(s.store.SpaceIndex(spaceId)),
		dryRun:               dryRun,
		createMissingOptions: createMissingOptions,
		createdOptions:       map[optionRef]string{},
		ambiguousOptions:     map[optionRef]bool{},
		dryReported:          map[optionRef]bool{},
		createdProps:         map[string]anyblockjson.PropertyDefinition{},
		createdPropIds:       map[string]string{},
		mintedSlugs:          map[string]string{},
	}
}

// Options returns anyblockjson import options wired with the creating
// resolvers.
//
// `Keys` is the SAME vocabulary the read half exports with (storeresolver —
// the space's stored slugs over the bundled table, §7.5a-5 precedence
// included). Leaving it unset fell back to BundledKeyVocabulary, and a write
// half speaking a narrower vocabulary than the read half is not a degradation
// but a corruption: the slug of a BSON-keyed relation is unknown to the
// bundled table, so `manual_property` imported as the literal stored key
// `manual_property` — a dataview naming a relation key no relation object
// owns (columns unbind, filters match nothing, silently). In the other
// direction the bundled table over-reaches: a space holding a live relation
// STORED under `due_date` had that key rewritten to bundled `dueDate` and the
// value landed on the wrong property, even though canonicalizeDocumentKeys
// had already resolved it correctly at chain step 1. storeresolver.PropertyKey
// implements step 1 (an exact live stored key wins over the slug layer), which
// is exactly what makes it safe here.
//
// Every import channel of this service rides these Options: create.go's
// document import, schema_write.go's type import, and stateops.go's
// insert_blocks/replace_subtree fragments and whole-dataview re-import behind
// every view op.
func (r *creatingResolvers) Options() anyblockjson.Options {
	if r.keys == nil {
		// D3: the write half's vocabulary is the read half's PLUS the slug
		// table (apikeyvocab.go) — a body carrying a non-fold-derivable
		// slug still lands on the relation that owns it
		r.keys = r.svc.apiKeys(r.spaceId, r.reads)
	}
	return anyblockjson.Options{
		ResolveFormat:     r.ResolveFormat,
		ResolveOptions:    r,
		ResolveProperties: r,
		Keys:              r.keys,
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
	matches := r.reads.OptionsNamed(key, name)
	switch len(matches) {
	case 1:
		return matches[0].Id, true
	case 0:
		// continue into the create-missing policy below
	default:
		r.recordOptionAmbiguity(key, name, matches)
		return "", false
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
	if !r.createMissingOptions {
		// A2 backstop. The pre-lock guard (guardCreateMissing) is what a PATCH
		// caller actually hits, and it refuses with the whole pending list;
		// this catches the create paths, which have no such guard, and it
		// makes the rule true of the resolver itself rather than of one
		// caller. Refusing beats returning "unresolved": that would store the
		// NAME where an option id belongs — a value matching nothing that
		// reads as if it matched.
		r.errs = append(r.errs, optionConsentError(r.spaceId, string(key), name))
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

func (r *creatingResolvers) recordOptionAmbiguity(key domain.RelationKey, name string, matches []*model.RelationOption) {
	ref := optionRef{property: string(key), name: name}
	if r.ambiguousOptions[ref] {
		return
	}
	r.ambiguousOptions[ref] = true
	if r.keys == nil {
		r.Options() // prime the same served slug vocabulary as the document
	}
	r.errs = append(r.errs, optionAmbiguityError(string(key), r.keys.PropertySlug(string(key)), name, matches))
}

func optionAmbiguityError(internalKey, servedKey, name string, matches []*model.RelationOption) error {
	ordered := append([]*model.RelationOption(nil), matches...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Text != ordered[j].Text {
			return ordered[i].Text < ordered[j].Text
		}
		if ordered[i].Color != ordered[j].Color {
			return ordered[i].Color < ordered[j].Color
		}
		return ordered[i].Id < ordered[j].Id
	})
	descriptions := make([]string, 0, len(ordered))
	for _, option := range ordered {
		color := option.Color
		if color == "" {
			color = "no color"
		}
		descriptions = append(descriptions, fmt.Sprintf("%q (%s)", option.Text, color))
	}
	return v2model.AmbiguousInput(
		fmt.Sprintf("option name %q is ambiguous for property %q", name, servedKey),
		v2model.Issue{
			Path:    "/properties/" + internalKey,
			Message: fmt.Sprintf("the name matches %d options: %s", len(matches), strings.Join(descriptions, ", ")),
			Hint:    "rename or remove one duplicate option before addressing it by name; v2 has no unambiguous bare-name choice",
		})
}

// optionConsentError is the one statement of the A2 refusal, shared by the
// pre-lock guard and the resolver backstop so the two cannot word the same
// rule differently.
func optionConsentError(spaceId, property, name string) error {
	return v2model.ValidationFailed("option does not exist",
		v2model.Issue{
			Path:    "/properties/" + property,
			Message: fmt.Sprintf("property %q has no option named %q, and this request did not ask to create one", property, name),
			Hint: fmt.Sprintf("check the spelling against GET /v2/spaces/%s/properties/%s/options, "+
				"or resend with ?create_missing_options=true to create it", spaceId, property),
		})
}

// prewarmCreateMissing resolves a PATCH's create-missing references BEFORE
// the object lock is taken (review B6/A6): the create surfaces in PATCH
// payloads are set_properties select/multiSelect option names and the option
// names an update_view filter or custom sort order carries, so those are
// resolved (and, on a real run, created) here; in-lock resolution then hits
// the resolver's cache and never fires a create-RPC while holding the edited
// object's lock. The M5 bound counts what this pass records, so a channel
// skipped here would also be a channel the too-many-options cap cannot see.
// The scan is deliberately lenient — every validation error still surfaces
// from the in-lock op pass, in unchanged order — and create failures ride
// resolvers.err(), exactly where the in-lock path checks them.
func (s *Service) prewarmCreateMissing(ops []json.RawMessage, resolvers *creatingResolvers) {
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
		if probe.Op == "update_view" || probe.Op == "insert_view" {
			// both carry the same set channel — the ONLY channel through which
			// a view op may introduce option names. Everything else a view op
			// serializes (untouched views, copy_from clones) is restored from
			// the live proto at commit and imported with a no-create resolver
			// (viewops.go commitDataviewBlock), so this prewarm is exhaustive:
			// a name it cannot see is a name the commit will not mint.
			s.prewarmViewOptionValues(probe.Set, resolvers)
			continue
		}
		if probe.Op != "set_properties" {
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

// prewarmViewOptionValues resolves the option names an update_view's set
// channel carries — filter leaf values and custom sort orders on select
// properties, both of which the dataview import resolves with create-missing
// (SPEC §6.2/§3) — so the creates run before the object lock and the M5
// bound sees them. Same leniency contract as the set_properties pass.
func (s *Service) prewarmViewOptionValues(set map[string]json.RawMessage, resolvers *creatingResolvers) {
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
			CustomOrder json.RawMessage `json:"custom_order"`
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

// ambiguityAwareReadOnlyOptionResolver is the remove-value resolver. Like
// readOnlyOptionResolver it never creates an option and an unknown name is a
// harmless miss, but duplicate names are not misses: accepting one would make
// remove depend on store order while set/add reject the same bare name.
type ambiguityAwareReadOnlyOptionResolver struct {
	r *creatingResolvers
}

func (ro ambiguityAwareReadOnlyOptionResolver) OptionName(key domain.RelationKey, id string) (string, bool) {
	return ro.r.OptionName(key, id)
}

func (ro ambiguityAwareReadOnlyOptionResolver) OptionId(key domain.RelationKey, name string) (string, bool) {
	if id, ok := ro.r.createdOptions[optionRef{property: string(key), name: name}]; ok {
		return id, true
	}
	matches := ro.r.reads.OptionsNamed(key, name)
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return matches[0].Id, true
	default:
		ro.r.recordOptionAmbiguity(key, name, matches)
		return "", false
	}
}

// PropertyById implements anyblockjson.PropertyResolver (export side).
func (r *creatingResolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return r.reads.PropertyById(id)
}

// PropertyId implements anyblockjson.PropertyResolver with create-missing:
// an unknown key in a type document's typeProperties creates the property
// (SPEC §2a). Resolution IS resolvePropertyInput — the same chain
// every other channel walks (exact stored key, live slug, bundled, fold),
// corpse-aware and ambiguity-loud — so this path can never diverge from
// the route/op channels again (the review's unification demand). Only a
// full chain miss creates. A BUNDLED key keeps the derived install path
// (convergence is the install mechanism, §2.4-1); a custom key mints a
// BSON internal key and the document's key becomes the apiObjectKey slug —
// never the stored relation key, whose derivation is what made concurrent
// same-key creates merge silently and delete-then-recreate a dead end.
func (r *creatingResolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	docKey := string(def.Key)
	if id, ok := r.createdPropIds[docKey]; ok {
		return id, true
	}
	entries, err := r.liveProps()
	if err != nil {
		// fail closed: an unreadable namespace must not look empty to the
		// mint check (§7.5a-2's rule, inverted, is the fail-open bug)
		r.errs = append(r.errs, err)
		return "", false
	}
	entry, ok, ambiguous := r.svc.resolvePropertyInput(docKey, entries)
	if len(ambiguous) > 0 {
		r.errs = append(r.errs, fmt.Errorf("property key %q is ambiguous — held by %s; address the intended property by its listed key", docKey, strings.Join(ambiguous, " and ")))
		return "", false
	}
	if ok && entry.Id != "" {
		r.createdPropIds[docKey] = entry.Id
		return entry.Id, true
	}
	isBundled := ok && entry.Id == "" // bundled vocabulary, not installed
	if isBundled {
		// A bundled key that exists only through the table may exist that way
		// because this space REMOVED its relation — naming it in
		// typeProperties would point a recommended list at the corpse (or,
		// pre-§8.40, reinstall it outright). Same refusal every other write
		// channel makes (§8.41), with ONE escape: when the PATCHed type's
		// lists already reference the holder, the entry is the read's own
		// echo and resolves as an identity — see echoPropertyIds.
		removed, err := r.removedBundledKeys()
		if err != nil {
			r.errs = append(r.errs, err)
			return "", false
		}
		isRemoved, err := r.svc.bundledPropertyRemoved(r.ctx, r.spaceId, entries, removed, entry.Key)
		if err != nil {
			r.errs = append(r.errs, err)
			return "", false
		}
		if isRemoved {
			holderId, held, err := r.svc.relationObjectHoldingKey(r.ctx, r.spaceId, entry.Key)
			if err != nil {
				r.errs = append(r.errs, err)
				return "", false
			}
			if held && r.echoPropertyIds[holderId] {
				r.createdPropIds[docKey] = holderId
				return holderId, true
			}
			// spelledAs is the served slug: typeProperties documents spell
			// slugs, so that is the spelling the caller can actually remove
			r.errs = append(r.errs, v2model.ValidationFailed("removed property key",
				removedPropertyIssue(r.spaceId, entry.Key, bundle.ApiSlug(entry.Key), "/type_settings/property_definitions")))
			return "", false
		}
		// storeresolver still resolves system relations by their bundled
		// definition (synthetic listing entries) and legacy index gaps by
		// point lookup (anomaly #9) — prefer that to firing an install RPC
		// for a mere reference. Bundled identity is derived and invariant
		// (same key ⇒ same tree, §2.4), so this can never resolve to a
		// wrong object; custom corpse keys stay out — this branch is
		// bundled-only.
		if id, found := r.reads.PropertyId(anyblockjson.PropertyDefinition{Key: domain.RelationKey(entry.Key)}); found {
			r.createdPropIds[docKey] = id
			return id, true
		}
	}

	// The chain has missed, so nothing LIVE answers to this key — but a
	// relation object may still hold it: the type document's own read serves
	// corpses in typeProperties (GET resolves recommendedRelations BY ID,
	// which is unfiltered by design — a type must show the properties it
	// actually references), and the documented read-modify-write loop sends
	// that list straight back. Minting here produced a brand-new property
	// duplicating the corpse's name under a snake-cased-hex slug, once per
	// PATCH, forever. A stored key held by a relation object has never been a
	// legitimate mint request in either direction, so it resolves to its
	// holder instead: the id returned is the id the type already carries, the
	// round trip is an identity, and no value moves anywhere. This is the
	// typeProperties counterpart of the §8.29 create tolerance and it is
	// deliberately KEY-ONLY — the corpse's SLUG vacated the namespace and
	// still resolves to nothing here, so a re-minted slug can never re-aim
	// onto the corpse. The probe's error is a hard stop, not a fall-through
	// to the mint below: "could not look" must never mint a duplicate.
	holderId, held, err := r.svc.relationObjectHoldingKey(r.ctx, r.spaceId, docKey)
	if err != nil {
		r.errs = append(r.errs, err)
		return "", false
	}
	if held && holderId != "" {
		r.createdPropIds[docKey] = holderId
		return holderId, true
	}

	name, format := def.Name, def.Format
	if isBundled {
		if name == "" {
			name = entry.Name
		}
		if format == 0 {
			format = entry.Format
		}
	}
	if name == "" {
		name = docKey
	}

	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(name),
		bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(format)),
		bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}}
	reportedKey := docKey
	if isBundled {
		// bundled: keep the derived key — every device deriving rel-<key>
		// converges on the installed object, which is the intent
		details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(entry.Key)
		reportedKey = entry.Key
	} else {
		// custom mint. Document keys pass no pattern check, so the derived
		// slug is sanitized to the key grammar; when normalization CHANGED
		// the spelling, the slug's own chain runs too — fold classes differ
		// when the key holds characters fold keeps (a space: "My Key" folds
		// to "my key", never reaching the my_key holder the minted slug
		// would shadow) — and any holder, live stored key included, refuses
		// the mint loudly (the union check ships WITH the mint, §7.6-3).
		slug := sanitizeApiSlug(bundle.ApiSlug(docKey))
		if slug != "" && slug != docKey {
			if _, taken, amb := r.svc.resolvePropertyInput(slug, entries); len(amb) > 0 {
				r.errs = append(r.errs, fmt.Errorf("create property %q: its key %q is ambiguous — held by %s", docKey, slug, strings.Join(amb, " and ")))
				return "", false
			} else if taken {
				r.errs = append(r.errs, fmt.Errorf("create property %q: its key %q is already taken — reference the existing property by that key, or use a different key", docKey, slug))
				return "", false
			}
		}
		if slug != "" {
			// blind spot closed (review cause 4): the live snapshot predates
			// this request's own mints, so two spellings of one key in one
			// document would both reach here — the second is refused, not
			// silently minted as a permanent twin
			if prev, taken := r.mintedSlugs[slug]; taken {
				r.errs = append(r.errs, fmt.Errorf("typeProperties declares %q and %q — two spellings of one key %q; keep one", prev, docKey, slug))
				return "", false
			}
			r.mintedSlugs[slug] = docKey
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
	r.createdProps[docKey] = anyblockjson.PropertyDefinition{Key: def.Key, Name: name, Format: format}
	r.createdPropIds[docKey] = resp.ObjectId
	return resp.ObjectId, true
}

// canonicalPropertyKey maps an inbound term to its canonical stored key for
// prewarm purposes, through the SAME chain the in-lock pass walks
// (resolvePropertyInput — fold included; the review's M5 bypass was
// exactly a fold the prewarm lacked and the in-lock pass had, letting a
// respelled key evade the create-missing bound). Ambiguity and misses pass
// through verbatim — the in-lock op pass owns the loud 400 — and a load
// error passes through too (the prewarm is best-effort; the in-lock pass
// fails closed on the same error).
func (r *creatingResolvers) canonicalPropertyKey(key string) string {
	if _, ok := r.createdProps[key]; ok {
		return key
	}
	entries, err := r.liveProps()
	if err != nil {
		return key
	}
	entry, ok, ambiguous := r.svc.resolvePropertyInput(key, entries)
	if len(ambiguous) > 0 || !ok {
		return key
	}
	return entry.Key
}

// liveProps primes the live-property snapshot once per resolver instance
// to keep one bounded query per request. The
// load error is remembered with the snapshot: callers on write paths must
// fail closed on it.
func (r *creatingResolvers) liveProps() ([]propertyEntry, error) {
	if r.livePropsLoaded {
		return r.liveEntries, r.livePropsErr
	}
	r.livePropsLoaded = true
	r.liveEntries, r.livePropsErr = r.svc.liveProperties(r.spaceId)
	return r.liveEntries, r.livePropsErr
}

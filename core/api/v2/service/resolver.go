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

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	spaceId string
	reads   *storeresolver.Resolvers
	dryRun  bool

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

func newCreatingResolvers(ctx context.Context, mw apicore.ClientCommands, spaceId string, index spaceindex.Store, dryRun bool) *creatingResolvers {
	return &creatingResolvers{
		ctx:            ctx,
		mw:             mw,
		spaceId:        spaceId,
		reads:          storeresolver.New(index),
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
// the object lock is taken (review B6/A6): the only create surface in PATCH
// payloads is setProperties select/multiSelect option names, so those are
// resolved (and, on a real run, created) here; in-lock resolution then hits
// the resolver's cache and never fires a create-RPC while holding the edited
// object's lock. The scan is deliberately lenient — every validation error
// still surfaces from the in-lock op pass, in unchanged order — and create
// failures ride resolvers.err(), exactly where the in-lock path checks them.
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
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Op != "setProperties" {
			continue
		}
		for field, values := range map[string]map[string]json.RawMessage{"set": probe.Set, "add": probe.Add} {
			for key, rawValue := range values {
				// A key claimed by more than one field is rejected by the apply
				// path, so prewarming it would create an option for a PATCH
				// that cannot succeed. Skip it here and let the op error.
				if field == "add" {
					if _, alsoInSet := probe.Set[key]; alsoInSet {
						continue
					}
				}
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

// PropertyById implements anyblockjson.PropertyResolver (export side).
func (r *creatingResolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return r.reads.PropertyById(id)
}

// PropertyId implements anyblockjson.PropertyResolver with create-missing:
// an unknown key in a type document's typeProperties creates the property
// (SPEC §2a), pinning the document's key as the stored relation key so the
// document round-trips. Bundled keys fill name/format from the bundle when
// the document omits them.
func (r *creatingResolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	if id, ok := r.createdPropIds[string(def.Key)]; ok {
		return id, true
	}
	if id, ok := r.reads.PropertyId(def); ok {
		return id, true
	}

	name, format := def.Name, def.Format
	if rel, err := bundle.GetRelation(def.Key); err == nil {
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

	r.sideEffects.Properties = append(r.sideEffects.Properties, v2model.PropertyRow{
		Key:    string(def.Key),
		Name:   name,
		Format: anyblockjson.FormatName(format),
	})
	if r.dryRun {
		return "", false
	}
	resp := r.mw.ObjectCreateRelation(r.ctx, &pb.RpcObjectCreateRelationRequest{
		SpaceId: r.spaceId,
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String():    pbtypes.String(string(def.Key)),
			bundle.RelationKeyName.String():           pbtypes.String(name),
			bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(format)),
			bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
		}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		r.errs = append(r.errs, fmt.Errorf("create property %q: %s", def.Key, resp.Error.Description))
		return "", false
	}
	r.createdProps[string(def.Key)] = anyblockjson.PropertyDefinition{Key: def.Key, Name: name, Format: format}
	r.createdPropIds[string(def.Key)] = resp.ObjectId
	return resp.ObjectId, true
}

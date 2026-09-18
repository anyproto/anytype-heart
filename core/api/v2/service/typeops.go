package v2service

// typeops.go is the op channel of PATCH /v2/spaces/{space_id}/types/{type}:
// add_property, remove_property, move_property, in the same envelope the
// object surface takes.
//
// The whole-type body stays beside it and keeps its replace semantics. What
// it cannot carry is INTENT. `property_definitions: [X]` is ambiguous between
// "make X the only field" and "add X", and the endpoint has to pick one: it
// picks replace, and callers read it as add. Three agents running the same
// benchmark independently sent a one-element array to add one field to a type
// that already had four, and all three lost the other four — one of them
// checked the result with a read and was reassured, because a type carries two
// property lists (the definitions and the view columns) that drift apart, and
// only the definitions half had been replaced.
//
// An op names its intent, which is what lets this file do the bookkeeping the
// intent implies: a removal updates the definitions AND prunes the views, a
// direction that did not exist before (template.PruneTypeDataviewColumns).
//
// Every property here is addressed by the api key this surface serves.
// Resolution goes through resolvePropertyInput, never through a bare
// entry.Key match: the view channel matched stored keys alone and refused
// every custom property by the only spelling a caller has ever been shown.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// v2TypeOpNames is the closed type-op set, in documentation order. It is
// deliberately NOT part of v2OpNames: the two endpoints take an ops envelope
// with disjoint op sets, and an object op named here — or a type op offered
// to PATCH /objects — would be advertised where it can only be refused.
var v2TypeOpNames = []string{
	"add_property", "remove_property", "move_property",
	// the view family, shared verbatim with the object channel. A type's own
	// document already SHOWS its dataview and views (GET /types/{key} serves
	// blocks), so refusing to edit them here was a read/write asymmetry: the
	// resource showed you a view and then called the op that changes it
	// unknown. The op bodies are identical — a dataview op does not care
	// whose dataview it edits — so the schemas are the object channel's,
	// unchanged.
	"insert_view", "update_view", "move_view", "delete_view",
}

// v2TypeViewOpNames is the view family alone: the ops that run against the
// type's dataview state rather than against its property lists.
var v2TypeViewOpNames = map[string]bool{
	"insert_view": true, "update_view": true, "move_view": true, "delete_view": true,
}

// v2TypeOpsUnknownKeyHint is what an unknown key beside `ops` means here: the
// caller mixed the two bodies this endpoint takes. The object surface's hint
// (an If-Match header written as a body field) has no counterpart on a type.
const v2TypeOpsUnknownKeyHint = "this endpoint takes either an ops envelope or the type body (name, plural_name, icon, layout, default_view, default_template, property_definitions), never both in one request"

// v2TypeSections pairs each of the type's four recommended-relation detail
// keys with the section name the property list speaks. The pairing is the
// format's, restated here because this channel writes the four lists itself
// rather than through BuildRecommendedLists — the ids of the entries it keeps
// are already known, and re-resolving them would make an untouched entry's
// identity depend on the resolution chain agreeing with itself twice.
// TestTypeOpSectionsMatchTheFormat pins the table against the format.
var v2TypeSections = []struct {
	detailKey domain.RelationKey
	section   string
}{
	{bundle.RelationKeyRecommendedFeaturedRelations, "featured"},
	{bundle.RelationKeyRecommendedRelations, ""},
	{bundle.RelationKeyRecommendedFileRelations, "file"},
	{bundle.RelationKeyRecommendedHiddenRelations, "hidden"},
}

// v2TypeAuthorableSections are the sections an op may put a property in.
// `file` is absent on purpose: which list a file property belongs to is the
// format's answer, and a type that already holds one keeps it where it is.
var v2TypeAuthorableSections = []string{"featured", "hidden"}

// typeListEntry is one member of a type's property list as the ops see it:
// the relation object id the type stores, the section it sits in, and the
// spellings an error message needs. An id whose relation this space cannot
// resolve keeps its place with empty spellings — a type may reference a
// property the user removed, and dropping it here would delete a reference
// the caller never asked to touch.
type typeListEntry struct {
	id      string
	section string
	key     string // stored relation key
	served  string // the api key this surface serves
	name    string
	format  model.RelationFormat
	minting bool // this request creates the property; id arrives later
}

// typeOpsPlan is the whole batch decided before anything is written.
type typeOpsPlan struct {
	// spaceId is the space the type lives in, for the hints a refusal names.
	spaceId string
	list    []*typeListEntry
	// mints are the entries whose property does not exist yet, in op order,
	// paired with the definition to create it from.
	mints    []*typeListEntry
	mintDefs []anyblockjson.PropertyDefinition
	mintPath []string
	// optionDefs are the select vocabularies the batch declares, paired with
	// the op path each came from, so a refusal points at the op the caller
	// wrote rather than at a position in a list they never sent.
	optionDefs  []anyblockjson.TypeProperty
	optionPaths []string
	// removed names the entries remove_property took out, in op order.
	removed []typeListEntry
	// addedByOp are the entries an add_property op named, whether it appended
	// a new one or re-placed one the type already listed. Both cases ensure
	// the column, matching ObjectTypePropertyAdd: a property already listed
	// but missing from a view is the half-consistent state this heals.
	addedByOp []*typeListEntry
	// fileSectioned are the properties that sat in the file section when the
	// batch began. The one-way guard reads this rather than the live entry:
	// remove_property drops the entry, so a later add of the same property
	// would otherwise re-append it into an authorable section and perform the
	// irreversible move the guard exists to refuse.
	fileSectioned map[string]bool
	// viewOps are the view-family ops, kept in the order the caller wrote
	// them and applied after the property half. Property ops first is the
	// only order where the column prune cannot undo a view the same batch
	// just asked for.
	viewOps   []json.RawMessage
	viewPaths []string
	// touchedLists is true once a property op has planned. A batch of view
	// ops alone changes no list, and writing all four unchanged is a write
	// nobody asked for — visible now that a caller can send view ops here.
	touchedLists bool
}

// updateTypeOps implements the ops envelope of PATCH types/{type}. The type
// is already resolved: UpdateType owns the 404.
func (s *Service) updateTypeOps(ctx context.Context, spaceId string, typeObject typeEntry, typeKey string, body []byte, dryRun, createMissingOptions bool) (*v2model.CreateResult, error) {
	ops, err := parseOpsEnvelope(body, v2TypeOpNames, v2TypeOpsUnknownKeyHint)
	if err != nil {
		return nil, err
	}
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil, err
	}
	plan, err := s.planTypeOps(spaceId, typeObject.Id, ops, entries, errKeysFor(ctx))
	if err != nil {
		return nil, err
	}

	// Reject before any side effect. Both guards below run on a batch that
	// has not written anything yet, which is the whole reason the plan above
	// resolves without creating: a refused batch that had already minted a
	// property leaves it behind as a relation nothing points at.
	optionDefs, optionPaths := plan.activeOptionDefs()
	if err := typeOpsOptionBudget(optionDefs, optionPaths); err != nil {
		return nil, err
	}
	if err := s.guardDeclaredOptionsAt(spaceId, optionDefs,
		func(i int) string { return optionPaths[i] + ".options" }, createMissingOptions); err != nil {
		return nil, err
	}

	resolvers := s.newCreatingResolvers(ctx, spaceId, dryRun, createMissingOptions)
	// the echo baseline: an entry the type ALREADY references resolves as an
	// identity even when its relation was removed from the space, so a batch
	// that names one is not forced to delete the reference
	before := s.recommendedRelationIds(spaceId, typeObject.Id)
	resolvers.echoPropertyIds = before
	if err := plan.mint(resolvers); err != nil {
		return nil, err
	}
	if err := s.applyDeclaredOptionsAt(optionDefs, resolvers,
		func(i int) string { return optionPaths[i] + ".options" }); err != nil {
		return nil, err
	}

	result := &v2model.CreateResult{Id: typeObject.Id, Key: typeKey, Created: resolvers.created()}

	// what the batch detached, through the same differ the whole-type body
	// uses — a removal is never inferable from the request, so it is reported
	// rather than left for the caller to discover on the next read
	detached := s.detachedProperties(spaceId, before, plan.keptIds())
	if len(detached) > 0 {
		result.Removed = &v2model.SideEffects{Properties: detached}
	}

	// the views, computed from the live type either way: a dry run that hid
	// the destructive half would be worse than no dry run at all
	prune, err := s.typeDataviewPrunePlan(ctx, spaceId, typeObject.Id, plan.removedKeys())
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, typePruneWarnings(prune)...)

	if dryRun {
		result.DryRun = true
		return result, nil
	}
	if plan.touchedLists {
		details := plan.detailUpdates()
		resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{ContextId: typeObject.Id, Details: details})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
			return nil, fmt.Errorf("update type %s: %s", typeKey, resp.Error.Description)
		}
	}
	// the lists first, the views second: the type's own open-time reconcile
	// only ever ADDS a column for a property the type recommends, so pruning
	// before the lists were written invites it straight back
	if len(prune.Pruned) > 0 {
		if err := s.pruneTypeDataviewColumns(ctx, spaceId, typeObject.Id, plan.removedKeys()); err != nil {
			return nil, err
		}
	}
	// the view half, last. Property ops wrote the lists and the prune ran, so
	// a view op here sees the state the batch actually produced rather than
	// one the prune is about to edit underneath it.
	if len(plan.viewOps) > 0 {
		created, err := s.applyTypeViewOps(ctx, spaceId, typeObject.Id, plan, resolvers)
		if err != nil {
			return nil, err
		}
		if len(created) > 0 {
			result.CreatedViews = created
		}
	}
	// the columns last, so a view this same batch inserted gets them too. Add
	// is idempotent and never overrides a visibility someone already chose,
	// so running it after the view ops cannot undo one of them.
	if _, err := s.addTypeDataviewColumns(ctx, spaceId, typeObject.Id, plan.addedLinks()); err != nil {
		return nil, err
	}
	if read, err := s.reader.ReadObject(ctx, spaceId, typeObject.Id); err == nil {
		result.Etag = ComputeEtag(read.Heads)
	}
	return result, nil
}

// applyTypeViewOps runs the view family against the type's own dataview,
// through the SAME appliers the object channel uses — a dataview op is a
// dataview op, and a second implementation of insert_view is how the two
// surfaces would drift.
func (s *Service) applyTypeViewOps(ctx context.Context, spaceId, typeId string, plan *typeOpsPlan, resolvers *creatingResolvers) (map[string]string, error) {
	var created map[string]string
	_, err := s.mutator.MutateObject(ctx, spaceId, typeId, apicore.EditNeeds{}, func(edit apicore.ObjectEdit) error {
		applier := newV2StateApplier(s, spaceId, typeId, model.SmartBlockType_STType, edit.State, resolvers, errKeysFor(ctx))
		for i, raw := range plan.viewOps {
			if err := applier.applyAt(raw, plan.viewPaths[i]); err != nil {
				return err
			}
		}
		created = applier.createdViews
		return nil
	})
	if err != nil {
		return nil, mapWriteError(spaceId, typeId, err)
	}
	return created, nil
}

//
// ---- the plan ----
//

// planTypeOps decodes and applies every op to a copy of the type's property
// list, resolving but never creating. It returns the first refusal, addressed
// by the same ops[i].field pointer the object channel uses.
func (s *Service) planTypeOps(spaceId, typeId string, ops []json.RawMessage, entries []propertyEntry, v errKeys) (*typeOpsPlan, error) {
	list, err := s.typePropertyList(spaceId, typeId, entries)
	if err != nil {
		return nil, err
	}
	plan := &typeOpsPlan{spaceId: spaceId, list: list, fileSectioned: fileSectionedKeys(list)}
	for i, raw := range ops {
		opPath := fmt.Sprintf("/ops/%d", i)
		var probe struct {
			Op string `json:"op"`
		}
		if jsonErr := json.Unmarshal(raw, &probe); jsonErr != nil {
			return nil, v2model.ValidationFailed("an op must be a JSON object",
				v2model.Issue{Path: opPath, Message: jsonErr.Error()})
		}
		switch probe.Op {
		case "add_property":
			err = s.planAddProperty(plan, raw, opPath, entries, v)
		case "remove_property":
			err = s.planRemoveProperty(plan, raw, opPath, entries, v)
		case "move_property":
			err = s.planMoveProperty(plan, raw, opPath, entries, v)
		case "insert_view", "update_view", "move_view", "delete_view":
			// held for the state pass below; the plan above is the property
			// lists, and a view op touches neither
			plan.viewOps = append(plan.viewOps, raw)
			plan.viewPaths = append(plan.viewPaths, opPath)
		case "":
			err = v2model.ValidationFailed("an op must name itself",
				v2model.Issue{Path: opPath + ".op", Message: "missing op",
					Hint: "allowed ops: " + strings.Join(v2TypeOpNames, ", ")})
		default:
			err = v2model.ValidationFailed(fmt.Sprintf("unknown op %q on a type", probe.Op),
				v2model.Issue{Path: opPath + ".op",
					Message: fmt.Sprintf("a type takes %s", strings.Join(v2TypeOpNames, ", ")),
				}.Hintf("blocks, views and property values are edited on the object surface through %s", v2model.NewRef(v2model.OpPatchObject, "space_id", spaceId)))
		}
		if err != nil {
			return nil, err
		}
	}
	return plan, nil
}

type opTypeAddProperty struct {
	Op       string                          `json:"op"`
	Property string                          `json:"property"`
	Format   string                          `json:"format"`
	Section  json.RawMessage                 `json:"section"`
	Options  []anyblockjson.OptionDefinition `json:"options"`
	After    string                          `json:"after"`
	Before   string                          `json:"before"`
	Position string                          `json:"position"`
}

type opTypeRemoveProperty struct {
	Op       string `json:"op"`
	Property string `json:"property"`
}

type opTypeMoveProperty struct {
	Op       string `json:"op"`
	Property string `json:"property"`
	After    string `json:"after"`
	Before   string `json:"before"`
	Position string `json:"position"`
}

func (s *Service) planAddProperty(plan *typeOpsPlan, raw json.RawMessage, opPath string, entries []propertyEntry, v errKeys) error {
	plan.touchedLists = true
	var op opTypeAddProperty
	if err := decodeStrictOp(raw, "add_property", opPath, &op); err != nil {
		return err
	}
	if op.Property == "" {
		return v2model.ValidationFailed("add_property needs a property",
			v2model.Issue{Path: opPath + ".property",
				Message: "name the property by its api key, or by its display name to create it"})
	}
	place := typePlacement{after: op.After, before: op.Before, position: op.Position}
	if err := place.validate(opPath, false); err != nil {
		return err
	}
	section, sectionStated, err := typeOpSection(op.Section, opPath)
	if err != nil {
		return err
	}

	entry, resolved, ambiguous := s.resolvePropertyInput(op.Property, entries)
	if len(ambiguous) > 0 {
		return ambiguousKeyError(v.propertyWord(), op.Property, opPath+".property", ambiguous)
	}
	declared, formatKnown := anyblockjson.FormatByName(op.Format)
	if op.Format != "" && !formatKnown {
		return v2model.ValidationFailed("unknown property format",
			v2model.Issue{Path: opPath + ".format",
				Message: fmt.Sprintf("%q is not a property format", op.Format),
			}.Hintf("formats are listed on %s", v2model.RefGetSchema("property")))
	}

	// identity: the stored key once the chain has answered, the caller's own
	// spelling when it has not. The spelling is what a mint turns into the
	// property's name and api key, and it is the key every later step —
	// the option guard, the option apply — addresses this entry by.
	identity := op.Property
	target := (*typeListEntry)(nil)
	// an entry an earlier op in THIS batch already added. A mint has no id
	// yet, so findById cannot reach it; without this, two adds of the same new
	// property appended twice and wrote one relation id into two lists.
	pending := plan.findBySpelling(op.Property)
	// A declared format that disagrees with the resolved property is refused
	// WHATEVER arm follows. It used to be checked only for an installed
	// property, so `{"property":"audioAlbum","format":"number"}` in a space
	// that has not installed bundled audioAlbum (longtext) minted a relation
	// on the bundled key with the wrong format — derived from that key, so
	// every device resolves it and no v2 call undoes it. The whole-type body
	// refuses the same request one door over (validateTypePropertyFormats).
	if resolved && op.Format != "" && entry.Format != declared {
		return v2model.ValidationFailed("property format conflict",
			v2model.Issue{Path: opPath + ".format",
				Message: fmt.Sprintf("%q already exists with format %q, and this op declares %q",
					op.Property, anyblockjson.FormatName(entry.Format), op.Format),
				Hint: "omit format to use the property as it is"})
	}
	switch {
	case resolved && entry.Id != "":
		identity = entry.Key
		target = plan.findById(entry.Id)
	case resolved:
		// the bundled vocabulary, not installed here. The resolver owns the
		// question of whether this space may still land on it — a bundled
		// property the user removed is refused there, with the escape for a
		// type that already references it.
		identity = entry.Key
	default:
		// a format is required only when this op is the one CREATING the
		// property. If an earlier op in the same batch already added it, this
		// one is a placement, and demanding a format again would refuse the
		// caller's own sequence.
		if pending == nil && op.Format == "" {
			return v2model.ValidationFailed("format is required to create a property",
				v2model.Issue{Path: opPath + ".format",
					Message: fmt.Sprintf("no property in this space answers to %q, so this op would create one", op.Property),
				}.Hintf("give format, or address an existing property by the key %s lists", v2model.RefListProperties(plan.spaceId)))
		}
	}

	// a stated format must agree with the pending mint's, exactly as it must
	// agree with an existing property's. Without this the second op's format
	// was dropped on the floor, and — worse — an options vocabulary recorded
	// with the op's own empty format skipped the select-format guard and
	// attached to a property the batch was minting as text.
	optionFormat := op.Format
	if pending != nil && pending.minting {
		minted := plan.mintFormatOf(pending)
		if op.Format != "" && minted != "" && op.Format != minted {
			return v2model.ValidationFailed("property format conflict",
				v2model.Issue{Path: opPath + ".format",
					Message: fmt.Sprintf("this batch is creating %q as %q, and this op declares %q",
						op.Property, minted, op.Format),
					Hint: "state one format for a property the batch creates, or omit it here"})
		}
		if optionFormat == "" {
			optionFormat = minted
		}
	}

	if len(op.Options) > 0 {
		plan.optionDefs = append(plan.optionDefs, anyblockjson.TypeProperty{
			Property: identity, Format: optionFormat, Options: op.Options,
		})
		plan.optionPaths = append(plan.optionPaths, opPath)
	}

	if target == nil {
		target = plan.findBySpelling(identity)
	}
	if target != nil {
		// already listed: this op is a placement, or it is nothing. Reporting
		// nothing is the documented outcome; silently dropping a section the
		// caller stated is not.
		if sectionStated {
			// out of the file section is a ONE-WAY move: `file` is not
			// authorable, so no op puts it back. Refuse rather than let one
			// request make a change this API cannot undo.
			if (target.section == "file" || plan.fileSectioned[op.Property] || plan.fileSectioned[identity]) && section != "file" {
				return v2model.ValidationFailed("this property is a file field",
					v2model.Issue{Path: opPath + ".section",
						Message: fmt.Sprintf("%q sits in the file section, which this channel cannot write", op.Property),
						Hint:    "omit section to leave it there"})
			}
			target.section = section
		}
		plan.addedByOp = append(plan.addedByOp, target)
		if place.stated() {
			return plan.place(target, place, opPath, entries, s, v)
		}
		return nil
	}

	// `served` carries the spelling a LATER op in this batch can address it
	// by. Without it a mint answered only to the exact string the add used, so
	// {add "Harvest Season"} then {move_property "harvest_season"} — the
	// design doc's own example — was refused with "unknown property key
	// harvest_season", the very key the response reports under created.
	// `served` is the spelling the mint will actually get — the same
	// derivation created.key reports — not the display name the add used.
	// Setting it to the display name made this fix inert: the design doc's own
	// batch ({add "Harvest Season"} then {move_property "harvest_season"})
	// was still refused with `unknown property key "harvest_season"`, the very
	// key the same response reports under `created`.
	if sectionStated && section != "file" && (plan.fileSectioned[op.Property] || plan.fileSectioned[identity]) {
		return v2model.ValidationFailed("this property is a file field",
			v2model.Issue{Path: opPath + ".section",
				Message: fmt.Sprintf("%q sits in the file section, which this channel cannot write", op.Property),
				Hint:    "omit section to leave it there"})
	}
	added := &typeListEntry{section: section, key: identity, name: op.Property}
	added.served = identity
	if !resolved {
		if slug := sanitizeApiSlug(bundle.ApiSlug(identity)); slug != "" {
			added.served = slug
		}
	}
	if resolved {
		added.id, added.key, added.name, added.format = entry.Id, entry.Key, entry.Name, entry.Format
		added.served = servedKeyIn(entry, entries)
	}
	if added.id == "" {
		added.minting = true
		// the format the link will carry; entry.format is otherwise only set
		// for a property that already existed
		added.format = declared
		def := anyblockjson.PropertyDefinition{Key: domain.RelationKey(identity), Format: declared}
		if !resolved {
			def.Name = op.Property
		}
		plan.mints = append(plan.mints, added)
		plan.mintDefs = append(plan.mintDefs, def)
		plan.mintPath = append(plan.mintPath, opPath)
	}
	plan.list = append(plan.list, added)
	plan.addedByOp = append(plan.addedByOp, added)
	if place.stated() {
		return plan.place(added, place, opPath, entries, s, v)
	}
	return nil
}

func (s *Service) planRemoveProperty(plan *typeOpsPlan, raw json.RawMessage, opPath string, entries []propertyEntry, v errKeys) error {
	plan.touchedLists = true
	var op opTypeRemoveProperty
	if err := decodeStrictOp(raw, "remove_property", opPath, &op); err != nil {
		return err
	}
	target, err := s.targetListEntry(plan, op.Property, opPath, entries, v)
	if err != nil {
		return err
	}
	plan.removed = append(plan.removed, *target)
	plan.drop(target)
	return nil
}

func (s *Service) planMoveProperty(plan *typeOpsPlan, raw json.RawMessage, opPath string, entries []propertyEntry, v errKeys) error {
	plan.touchedLists = true
	var op opTypeMoveProperty
	if err := decodeStrictOp(raw, "move_property", opPath, &op); err != nil {
		return err
	}
	place := typePlacement{after: op.After, before: op.Before, position: op.Position}
	if err := place.validate(opPath, true); err != nil {
		return err
	}
	target, err := s.targetListEntry(plan, op.Property, opPath, entries, v)
	if err != nil {
		return err
	}
	return plan.place(target, place, opPath, entries, s, v)
}

// targetListEntry resolves a property term to the list member it names.
// A term that names no property at all and a term that names one this type
// does not list are different mistakes, and each gets its own refusal:
// a caller told "not listed" about a typo would go looking at the type.
func (s *Service) targetListEntry(plan *typeOpsPlan, term, opPath string, entries []propertyEntry, v errKeys) (*typeListEntry, error) {
	if term == "" {
		return nil, v2model.ValidationFailed("this op needs a property",
			v2model.Issue{Path: opPath + ".property", Message: "name the property this op applies to"})
	}
	entry, resolved, ambiguous := s.resolvePropertyInput(term, entries)
	if len(ambiguous) > 0 {
		return nil, ambiguousKeyError(v.propertyWord(), term, opPath+".property", ambiguous)
	}
	if resolved && entry.Id != "" {
		if target := plan.findById(entry.Id); target != nil {
			return target, nil
		}
	}
	// the chain answers for live properties. A type may still list one the
	// space removed, and that entry has to stay removable by the spelling the
	// type's own read shows for it.
	if target := plan.findBySpelling(term); target != nil {
		return target, nil
	}
	listed := plan.spellings()
	if !resolved || entry.Id == "" {
		return nil, v2model.ValidationFailed(fmt.Sprintf("unknown %s %q", v.propertyWord(), term),
			v2model.Issue{Path: opPath + ".property",
				Message: fmt.Sprintf("no property in this space answers to %q", term),
			}.Hintf("%s lists them", v2model.RefListProperties(plan.spaceId)))
	}
	return nil, v2model.ValidationFailed(
		fmt.Sprintf("this type does not list %q", term),
		v2model.Issue{Path: opPath + ".property",
			Message: fmt.Sprintf("the property exists in the space, and this type's fields are: %s", listKeys(listed)),
			Hint:    "add_property puts it on the type"})
}

// typePlacement is the after/before/position vocabulary the block and view
// ops already speak, on the property list.
type typePlacement struct {
	after    string
	before   string
	position string
}

func (p typePlacement) stated() bool {
	return p.after != "" || p.before != "" || p.position != ""
}

func (p typePlacement) validate(opPath string, required bool) error {
	given := 0
	for _, value := range []string{p.after, p.before, p.position} {
		if value != "" {
			given++
		}
	}
	if given > 1 {
		return v2model.ValidationFailed("give at most one of after, before and position",
			v2model.Issue{Path: opPath, Message: "these are three spellings of one destination"})
	}
	if given == 0 && required {
		return v2model.ValidationFailed("this op needs a destination",
			v2model.Issue{Path: opPath, Message: "give one of after, before or position"})
	}
	if p.position != "" && p.position != "first" && p.position != "last" {
		return v2model.ValidationFailed("unknown position",
			v2model.Issue{Path: opPath + ".position",
				Message: fmt.Sprintf("%q is not a position", p.position),
				Hint:    `position is "first" or "last"`})
	}
	return nil
}

// typeOpSection decodes the section member. Absent leaves an existing entry
// where it is and puts a new one in the plain field list; an explicit null
// is how a caller asks for that list by name, the same way a null clears a
// field to its default on the view channel.
func typeOpSection(raw json.RawMessage, opPath string) (section string, stated bool, err error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	if isJSONNull(raw) {
		return "", true, nil
	}
	var name string
	if jsonErr := json.Unmarshal(raw, &name); jsonErr != nil {
		return "", false, v2model.ValidationFailed("section is a string or null",
			v2model.Issue{Path: opPath + ".section", Message: jsonErr.Error()})
	}
	for _, allowed := range v2TypeAuthorableSections {
		if name == allowed {
			return name, true, nil
		}
	}
	return "", false, v2model.ValidationFailed("unknown section",
		v2model.Issue{Path: opPath + ".section",
			Message: fmt.Sprintf("%q is not a section this channel writes", name),
			Hint:    "use " + strings.Join(v2TypeAuthorableSections, " or ") + ", or null for the plain field list"})
}

//
// ---- list mechanics ----
//

// typePropertyList reads the type's four recommended lists into one ordered
// list. Order within a section is what the ops move things around in; the
// sections themselves are four separate details, so a cross-section move is
// refused rather than silently doing nothing.
func (s *Service) typePropertyList(spaceId, typeId string, entries []propertyEntry) ([]*typeListEntry, error) {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		// fail closed. Every op is a delta against this list, and the write
		// replaces all four stored lists — so a list that read as empty
		// because the read failed would not edit the type's fields, it would
		// delete them.
		return nil, fmt.Errorf("read type %s: %w", typeId, err)
	}
	// spaceindex.GetDetails answers an unknown id with an EMPTY row and no
	// error, so neither this nor the err check above fired for the hazard the
	// comment describes: the ops would plan against an empty list and the
	// write would replace all four stored lists with nothing. A type always
	// carries details (requireLiveType just resolved it), so an empty row here
	// is a failed read, not a type with no fields.
	if details == nil || details.Len() == 0 {
		return nil, fmt.Errorf("read type %s: no details", typeId)
	}
	byId := make(map[string]propertyEntry, len(entries))
	for _, entry := range entries {
		byId[entry.Id] = entry
	}
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	var list []*typeListEntry
	seen := map[string]bool{}
	for _, section := range v2TypeSections {
		for _, id := range details.GetStringList(section.detailKey) {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			member := &typeListEntry{id: id, section: section.section}
			if entry, live := byId[id]; live {
				member.key, member.name, member.format = entry.Key, entry.Name, entry.Format
				member.served = servedKey(entry.Key, entry.Slug, keyTaken, slugHolders)
			} else {
				// a reference to a property this space cannot resolve. Its id
				// is the only spelling left, so that is what addresses it —
				// without this the entry answered to nothing and no op could
				// take a dangling reference off the type.
				member.served = id
			}
			list = append(list, member)
		}
	}
	return list, nil
}

// fileSectionedKeys is the set of spellings that began the batch in the file
// section, by every name an op could address them with.
func fileSectionedKeys(list []*typeListEntry) map[string]bool {
	out := map[string]bool{}
	for _, member := range list {
		if member.section != "file" {
			continue
		}
		for _, spelling := range []string{member.served, member.key, member.name} {
			if spelling != "" {
				out[spelling] = true
			}
		}
	}
	return out
}

// mintFormatOf is the format the batch will create this pending entry with,
// by the name the caller would state it as.
func (p *typeOpsPlan) mintFormatOf(target *typeListEntry) string {
	for i, pending := range p.mints {
		if pending == target {
			return anyblockjson.FormatName(p.mintDefs[i].Format)
		}
	}
	return ""
}

func (p *typeOpsPlan) findById(id string) *typeListEntry {
	if id == "" {
		return nil
	}
	for _, member := range p.list {
		if member.id == id {
			return member
		}
	}
	return nil
}

// findBySpelling matches the spellings the type's own read shows for a member
// — the served api key, the stored key, the display name.
func (p *typeOpsPlan) findBySpelling(term string) *typeListEntry {
	for _, member := range p.list {
		if term == member.served || term == member.key {
			return member
		}
		// the display name is an address only for a member this batch is
		// MINTING, where no stored spelling exists yet. On a listed member it
		// would reach a hidden relation that resolvePropertyInput deliberately
		// excludes from the name step (keys.go), and a caller naming a visible
		// property would silently edit the hidden twin.
		if member.minting && member.name != "" && term == member.name {
			return member
		}
	}
	return nil
}

func (p *typeOpsPlan) indexOf(target *typeListEntry) int {
	for i, member := range p.list {
		if member == target {
			return i
		}
	}
	return -1
}

func (p *typeOpsPlan) drop(target *typeListEntry) {
	if i := p.indexOf(target); i >= 0 {
		p.list = append(p.list[:i], p.list[i+1:]...)
	}
	// and cancel the mint it was going to perform. mint() iterates p.mints
	// unconditionally, so [add "Harvest Season", remove "Harvest Season"]
	// returned 200, wrote the lists unchanged, and still created the relation.
	for i, pending := range p.mints {
		if pending != target {
			continue
		}
		p.mints = append(p.mints[:i], p.mints[i+1:]...)
		p.mintDefs = append(p.mintDefs[:i], p.mintDefs[i+1:]...)
		p.mintPath = append(p.mintPath[:i], p.mintPath[i+1:]...)
		break
	}
}

// activeOptionDefs are the declared vocabularies whose property the batch
// still lists. A vocabulary declared on an entry a later op removed has
// nothing to attach to, and creating its options anyway is the same orphan
// class as the cancelled mint above.
func (p *typeOpsPlan) activeOptionDefs() ([]anyblockjson.TypeProperty, []string) {
	defs := make([]anyblockjson.TypeProperty, 0, len(p.optionDefs))
	paths := make([]string, 0, len(p.optionPaths))
	for i, def := range p.optionDefs {
		if p.findBySpelling(def.Property) == nil {
			continue
		}
		defs = append(defs, def)
		paths = append(paths, p.optionPaths[i])
	}
	return defs, paths
}

// place moves one member to the destination the op names. after and before
// name an anchor in the SAME section: the four sections are four stored
// lists, so ordering one member against a member of another list changes
// nothing, and a move that silently does nothing is the failure this whole
// channel exists to end.
func (p *typeOpsPlan) place(target *typeListEntry, place typePlacement, opPath string, entries []propertyEntry, s *Service, v errKeys) error {
	insert := func(at int) {
		from := p.indexOf(target)
		if from >= 0 {
			p.list = append(p.list[:from], p.list[from+1:]...)
			if at > from {
				at--
			}
		}
		if at > len(p.list) {
			at = len(p.list)
		}
		p.list = append(p.list, nil)
		copy(p.list[at+1:], p.list[at:])
		p.list[at] = target
	}

	if place.position != "" {
		first, last := -1, -1
		for i, member := range p.list {
			if member == target || member.section != target.section {
				continue
			}
			if first < 0 {
				first = i
			}
			last = i
		}
		if first < 0 {
			return nil // alone in its section: every position is the same one
		}
		if place.position == "first" {
			insert(first)
		} else {
			insert(last + 1)
		}
		return nil
	}

	ref := place.after
	field := ".after"
	if ref == "" {
		ref, field = place.before, ".before"
	}
	anchor, err := s.targetListEntry(p, ref, opPath, entries, v)
	if err != nil {
		return err
	}
	if anchor == target {
		return v2model.ValidationFailed("a property cannot be placed relative to itself",
			v2model.Issue{Path: opPath + field, Message: fmt.Sprintf("%q is the property this op moves", ref)})
	}
	if anchor.section != target.section {
		return v2model.ValidationFailed("the two properties are in different sections",
			v2model.Issue{Path: opPath + field,
				Message: fmt.Sprintf("%s and %s are stored as separate lists, so ordering one against the other changes nothing",
					sectionWord(target.section), sectionWord(anchor.section)),
				Hint: "add_property takes section, which is how a property moves between them"})
	}
	at := p.indexOf(anchor)
	if place.after != "" {
		at++
	}
	insert(at)
	return nil
}

// sectionWord names one section the way a caller reads it.
func sectionWord(section string) string {
	if section == "" {
		return "the plain field list"
	}
	return "the " + section + " section"
}

// spellings lists the type's members by the key a caller can address them
// with, for the refusal that says what the type does carry.
func (p *typeOpsPlan) spellings() []string {
	out := make([]string, 0, len(p.list))
	for _, member := range p.list {
		switch {
		case member.served != "":
			out = append(out, member.served)
		case member.key != "":
			out = append(out, member.key)
		}
	}
	sort.Strings(out)
	return out
}

// listKeys renders a bounded key list for an error message.
func listKeys(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	if len(keys) > maxListedKeys {
		return strings.Join(keys[:maxListedKeys], ", ") +
			fmt.Sprintf(" and %d more", len(keys)-maxListedKeys)
	}
	return strings.Join(keys, ", ")
}

// keptIds are the relation ids the batch leaves on the type — the input the
// detached-property differ compares the before state against.
func (p *typeOpsPlan) keptIds() map[string]bool {
	kept := map[string]bool{}
	for _, member := range p.list {
		if member.id != "" {
			kept[member.id] = true
		}
	}
	return kept
}

// removedKeys are the stored relation keys remove_property took off the type
// — the keys the view prune works in. An entry whose relation this space
// cannot resolve has none, and its columns stay.
func (p *typeOpsPlan) removedKeys() []string {
	// the NET effect, not the op log: a batch that removes a property and adds
	// it back still lists it, so its columns must stay. Reading p.removed
	// alone pruned them and then reported a removal the same response
	// contradicted.
	kept := map[string]bool{}
	for _, member := range p.list {
		if member.key != "" {
			kept[member.key] = true
		}
	}
	var keys []string
	seen := map[string]bool{}
	for _, member := range p.removed {
		if member.key == "" || kept[member.key] || seen[member.key] {
			continue
		}
		seen[member.key] = true
		keys = append(keys, member.key)
	}
	return keys
}

// addedLinks are the dataview links for the properties add_property named and
// the batch still lists, in op order. Like removedKeys this reads the NET
// effect: an entry a later remove_property dropped is gone from p.list, so a
// batch that adds a property and takes it away again adds no column.
//
// An entry with no stored key is skipped — a dry run mints nothing, so it has
// none, and there is no column to add for a property that does not exist.
func (p *typeOpsPlan) addedLinks() []*model.RelationLink {
	listed := map[*typeListEntry]bool{}
	for _, member := range p.list {
		listed[member] = true
	}
	var links []*model.RelationLink
	seen := map[string]bool{}
	for _, member := range p.addedByOp {
		if !listed[member] || member.key == "" || seen[member.key] {
			continue
		}
		seen[member.key] = true
		links = append(links, &model.RelationLink{Key: member.key, Format: member.format})
	}
	return links
}

// detailUpdates rebuilds the four recommended lists from the planned order.
// All four are always written, empty ones included: that is how a type stores
// an empty section, and it is what the whole-type body writes too.
func (p *typeOpsPlan) detailUpdates() []*model.Detail {
	bySection := map[string][]string{}
	for _, member := range p.list {
		if member.id == "" {
			continue // a mint the resolver did not perform (a dry run)
		}
		bySection[member.section] = append(bySection[member.section], member.id)
	}
	details := make([]*model.Detail, 0, len(v2TypeSections))
	for _, section := range v2TypeSections {
		ids := bySection[section.section]
		if ids == nil {
			ids = []string{}
		}
		details = append(details, &model.Detail{
			Key:   string(section.detailKey),
			Value: pbtypes.StringList(ids),
		})
	}
	return details
}

// mint creates the properties the batch named and nothing in the space
// answers to. It runs after every refusal the batch can raise, so a failed
// batch leaves no orphan relation behind.
func (p *typeOpsPlan) mint(resolvers *creatingResolvers) error {
	for i, member := range p.mints {
		id, ok := resolvers.PropertyId(p.mintDefs[i])
		if err := resolvers.err(); err != nil {
			return v2model.ValidationFailed(err.Error(),
				v2model.Issue{Path: p.mintPath[i] + ".property", Message: err.Error()})
		}
		if ok {
			member.id, member.minting = id, false
			// the STORED key the mint assigned, which is not the spelling the
			// caller sent: it is what the view prune works in, so a batch that
			// adds and then removes the same property addresses the right one
			if key, known := resolvers.PropertyStoredKey(string(p.mintDefs[i].Key)); known {
				member.key = key
			}
		}
		// a dry run mints nothing and resolves nothing: the entry keeps an
		// empty id, detailUpdates skips it, and created() reports it
	}
	return nil
}

// typeOpsOptionBudget bounds how many select options one batch may create,
// the way the object channel bounds its own (v2MaxCreatedOptionsPerPatch).
// Creating an option is irreversible and syncs to every device, and 512 ops
// each declaring a full vocabulary is a hallucinated array rather than
// intent. The per-op cap is the maxItems the served schema advertises, which
// nothing enforced.
func typeOpsOptionBudget(defs []anyblockjson.TypeProperty, paths []string) error {
	total := 0
	for i, def := range defs {
		if len(def.Options) > maxV2PropertyOptions {
			return v2model.ValidationFailed("too many options",
				v2model.Issue{
					Path:    paths[i] + ".options",
					Message: fmt.Sprintf("%d options — the cap is %d (the advertised maxItems)", len(def.Options), maxV2PropertyOptions),
				})
		}
		total += len(def.Options)
	}
	if total > v2MaxCreatedOptionsPerPatch {
		return v2model.ValidationFailed("too many options in one batch",
			v2model.Issue{
				Path:    "/ops",
				Message: fmt.Sprintf("this batch declares %d options (limit %d)", total, v2MaxCreatedOptionsPerPatch),
			}.Hintf("split the vocabulary across several requests, or create the property with %s", v2model.NewRef(v2model.OpCreateProperty)))
	}
	return nil
}

// servedKeyIn is servedKey for one entry over a primed live set.
func servedKeyIn(entry propertyEntry, entries []propertyEntry) string {
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	return servedKey(entry.Key, entry.Slug, keyTaken, slugHolders)
}

//
// ---- the views ----
//

// typeDataviewPrunePlan reads the type's own dataview and works out what
// removing these properties does to it, without writing. A dry run and a
// real run both go through this, so the preview is the edit.
func (s *Service) typeDataviewPrunePlan(ctx context.Context, spaceId, typeId string, keys []string) (template.TypeDataviewColumnPlan, error) {
	var plan template.TypeDataviewColumnPlan
	if len(keys) == 0 {
		return plan, nil
	}
	read, err := s.reader.ReadObject(ctx, spaceId, typeId)
	if err != nil {
		return plan, mapReadError(spaceId, typeId, err)
	}
	if read.Snapshot == nil {
		return plan, nil
	}
	for _, block := range read.Snapshot.Blocks {
		if dv := block.GetDataview(); dv != nil {
			return template.PlanTypeDataviewColumnPrune(dv, keys), nil
		}
	}
	return plan, nil
}

// pruneTypeDataviewColumns applies the prune to the live type.
func (s *Service) pruneTypeDataviewColumns(ctx context.Context, spaceId, typeId string, keys []string) error {
	// a view edit needs neither restriction axis: a type carries the blocks
	// restriction, and the native view surface does not check it either —
	// which is how the app edits a type's views at all
	_, err := s.mutator.MutateObject(ctx, spaceId, typeId, apicore.EditNeeds{}, func(edit apicore.ObjectEdit) error {
		block := typeDataviewBlock(edit.State)
		if block == nil {
			return nil
		}
		edited := block.Copy()
		dv := edited.Model().GetDataview()
		changed := template.PruneTypeDataviewColumns(dv, keys)
		// the link that had no column to begin with — the half-consistent
		// state a client's own delete leaves — which the prune above cannot
		// reach because it only drops links whose columns it pruned
		for _, key := range keys {
			if template.DropUnreferencedTypeDataviewLink(dv, key) {
				changed = true
			}
		}
		if !changed {
			return nil
		}
		edit.State.Set(edited)
		return nil
	})
	if err != nil {
		return mapWriteError(spaceId, typeId, err)
	}
	return nil
}

// addTypeDataviewColumns is the add direction, and the reason this channel no
// longer leans on the open-time reconcile: a property added here becomes a
// column of every view now, rather than whenever someone next opens the type.
//
// It is template.AddTypeDataviewColumn, the same helper ObjectTypePropertyAdd
// applies, so the two surfaces cannot drift on what adding a property does to
// a type's views. Unlike ReconcileTypeDataviewColumns it reads no evidence
// about whether a human arranged the view — an explicit add was asked for —
// and a column a view already has keeps the visibility its owner gave it.
//
// Returns the ids of the views that gained a column.
func (s *Service) addTypeDataviewColumns(ctx context.Context, spaceId, typeId string, links []*model.RelationLink) ([]string, error) {
	if len(links) == 0 {
		return nil, nil
	}
	var gained []string
	_, err := s.mutator.MutateObject(ctx, spaceId, typeId, apicore.EditNeeds{}, func(edit apicore.ObjectEdit) error {
		block := typeDataviewBlock(edit.State)
		if block == nil {
			return nil
		}
		edited := block.Copy()
		dv := edited.Model().GetDataview()
		seen := map[string]bool{}
		touched := false
		for _, link := range links {
			viewIds, changed := template.AddTypeDataviewColumn(dv, link, true)
			touched = touched || changed
			for _, viewId := range viewIds {
				if !seen[viewId] {
					seen[viewId] = true
					gained = append(gained, viewId)
				}
			}
		}
		// a batch whose adds find every column and link already in place
		// writes nothing, the way a batch of view ops alone leaves the four
		// lists untouched
		if !touched {
			return nil
		}
		edit.State.Set(edited)
		return nil
	})
	if err != nil {
		return nil, mapWriteError(spaceId, typeId, err)
	}
	return gained, nil
}

// typeDataviewBlock finds a type's dataview block: at the fixed id first,
// which is where every type built by this app carries it, then by content for
// anything older.
func typeDataviewBlock(st *state.State) simple.Block {
	if block := st.Pick(state.DataviewBlockID); block != nil && block.Model().GetDataview() != nil {
		return block
	}
	var found simple.Block
	st.Iterate(func(block simple.Block) bool {
		if found == nil && block.Model().GetDataview() != nil {
			found = block
		}
		return found == nil
	})
	return found
}

// typePruneWarnings states both halves of what the prune did: the views that
// lost a column, and the views that kept one because they arrange themselves
// by it. Neither is inferable from the request, which named a property and
// said nothing about views.
func typePruneWarnings(plan template.TypeDataviewColumnPlan) []v2model.Issue {
	var issues []v2model.Issue
	if len(plan.Pruned) > 0 {
		issues = append(issues, v2model.Issue{
			Path:    "/ops",
			Message: fmt.Sprintf("columns dropped: %s", prunedPerView(plan.Pruned)),
		})
	}
	if len(plan.InUse) > 0 {
		issues = append(issues, v2model.Issue{
			Path: "/ops",
			Message: fmt.Sprintf("%s group, sort or filter by a removed property and were left as they are",
				namedViews(plan.InUse)),
		}.Hintf("change those views with update_view through %s", v2model.NewRef(v2model.OpPatchObject)))
	}
	return issues
}

// namedViews renders "the view \"All\"" / "2 views (All, Board)".
// prunedPerView names what left each view, rather than letting a reader pair
// every removed property with every view.
func prunedPerView(views []template.ViewColumnPrune) string {
	parts := make([]string, 0, len(views))
	for _, view := range views {
		name := view.ViewName
		if name == "" {
			name = view.ViewId
		}
		parts = append(parts, fmt.Sprintf("%s from %q", listKeys(view.Keys), name))
	}
	return strings.Join(parts, "; ")
}

func namedViews(views []template.ViewColumnPrune) string {
	names := make([]string, 0, len(views))
	for _, view := range views {
		name := view.ViewName
		if name == "" {
			name = view.ViewId
		}
		names = append(names, name)
	}
	if len(names) == 1 {
		return fmt.Sprintf("the view %q", names[0])
	}
	return fmt.Sprintf("%d views (%s)", len(names), strings.Join(names, ", "))
}

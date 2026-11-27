package v2service

// templates.go owns the two halves of the template surface a create needs:
// which template POST /objects starts an object from
// (resolveCreateTemplate), and how a caller finds one in the first place
// (ListTemplates, GET /v2/spaces/{space_id}/templates).
//
// The create half exists because the choice is made in three places and only
// one of them is the caller's: the request may name a template, the type may
// name one in `default_template`, and neither may be applicable any more.
// Heart's own resolution (templateimpl.resolveValidTemplateId) answers all
// three the same way — it falls back to the blank template in silence — which
// is exactly the failure R6-2 recorded: a stored default that never applies,
// with nothing in any response to say so. Here the three answers are kept
// apart: a template the CALLER named and cannot be applied is a refusal, a
// TYPE default that cannot be applied is a warning on an object that is still
// created, and whatever is applied is named in the result.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/types"
)

const (
	// createTemplateNone is the `template` value that starts an object from
	// nothing. A create with no `template` at all takes the type's default,
	// so without a word for "no template" a type with a default template
	// would have no opt-out at all.
	//
	// An EMPTY string is not that word: it reads as absent, and takes the
	// default. A body generated against a schema tends to carry every member
	// it can see, empty ones included, and reading `""` as an opt-out would
	// let that habit quietly switch the type's default off.
	createTemplateNone = "none"

	// templateSourceRequest / templateSourceTypeDefault are the `source`
	// values of an applied template: who chose it.
	templateSourceRequest     = "request"
	templateSourceTypeDefault = "type_default"
)

// resolveCreateTemplate answers which template a create starts from, for a
// document whose (stored) type key is typeKey and whose `template` member was
// requested (empty when the body carried none).
//
// It returns the template to apply (nil for none), the warnings the result
// carries, and a refusal. The refusal is reserved for a template the CALLER
// named: a create that asked for a specific starting point and silently got
// another is the failure mode this whole surface is here to end. A type
// default that cannot be applied warns instead, because the caller did not
// choose it and refusing would make every create of that type fail until
// someone repaired the type.
func (s *Service) resolveCreateTemplate(ctx context.Context, spaceId, typeKey, requested string) (*v2model.AppliedTemplate, []v2model.Issue, error) {
	if requested == createTemplateNone {
		return nil, nil, nil
	}
	// a store error here is an error, never a missing type: reading "this
	// type has no row" out of a failed read would apply no template and say
	// nothing, which is the silence this surface exists to end
	typeId, typeFound, err := s.liveTypeIdInSpace(spaceId, typeKey)
	if err != nil {
		return nil, nil, err
	}
	if requested != "" {
		// a bundled type this space has not installed yet passes
		// validateDocumentRefs while holding no store row. Its id is still
		// knowable — a derived object's id is a pure function of space and
		// key — and it has to be, because it is the id any template of that
		// type carries as its target: without it the one check that catches
		// a template of ANOTHER type would be skipped exactly where a caller
		// cannot see the difference either.
		targetId := typeId
		if !typeFound {
			derived, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(typeKey))
			if err != nil {
				return nil, nil, fmt.Errorf("derive type id for %s: %w", typeKey, err)
			}
			targetId = derived
		}
		name, problem, err := s.inspectTemplate(ctx, spaceId, requested, targetId)
		if err != nil {
			return nil, nil, err
		}
		if problem != "" {
			return nil, nil, v2model.ValidationFailed("the template cannot be applied",
				v2model.Issue{Path: "/template", Message: problem}.
					Hintf("list the templates of this type with %s, or send \"none\" to start from nothing",
						v2model.RefListTemplates(spaceId).With("type", s.servedTypeSpelling(spaceId, typeKey, targetId))))
		}
		return &v2model.AppliedTemplate{Id: requested, Name: name, Source: templateSourceRequest}, nil, nil
	}
	if !typeFound {
		// no row, so no default_template to read; a type this space has yet
		// to install carries no settings of its own
		return nil, nil, nil
	}
	defaultId, err := s.typeDefaultTemplate(spaceId, typeId)
	if err != nil || defaultId == "" {
		return nil, nil, err
	}
	name, problem, err := s.inspectTemplate(ctx, spaceId, defaultId, typeId)
	if err != nil {
		return nil, nil, err
	}
	if problem != "" {
		// the stale-default case R6-2 found: deleting a template clears the
		// default only when it was stored as a bare string, so a default set
		// through this API outlives its own template. The object is created
		// without a template, and the warning says so rather than leaving the
		// caller to wonder where the template went.
		// the type is named the way every other response names it: the
		// canonicalized key here is a stored one, which for a space-minted
		// type is a 24-hex id the caller never sent and cannot look up
		spelling := s.servedTypeSpelling(spaceId, typeKey, typeId)
		return nil, []v2model.Issue{v2model.Issue{
			Path:    "/type",
			Message: fmt.Sprintf("the default template of type %q was not applied: %s", spelling, problem),
		}.Hintf("point default_template at a live template, or clear it, with %s",
			v2model.RefUpdateType(spaceId, spelling))}, nil
	}
	return &v2model.AppliedTemplate{Id: defaultId, Name: name, Source: templateSourceTypeDefault}, nil, nil
}

// resolveDocumentTemplate is resolveCreateTemplate with the document kinds
// that cannot start from a template taken out of its way.
//
// A template document is the one create POST /objects makes that produces a
// template rather than an object of a type, and a template of a template is
// not a thing this product has. Saying so is better than resolving: the type
// key of such a document is `template`, whose own default_template would
// otherwise be consulted and, if some space ever set one, applied.
func (s *Service) resolveDocumentTemplate(ctx context.Context, spaceId string, envelope *docEnvelope, requested string) (*v2model.AppliedTemplate, []v2model.Issue, error) {
	if envelope.Kind == "template" || envelope.Type == string(bundle.TypeKeyTemplate) {
		if requested != "" && requested != createTemplateNone {
			return nil, nil, v2model.ValidationFailed("a template does not start from a template",
				v2model.Issue{Path: "/template", Message: "this body creates a template, and a template has no template of its own"}.
					Hintf("drop template, and name the type it starts an object of in template_for"))
		}
		return nil, nil, nil
	}
	return s.resolveCreateTemplate(ctx, spaceId, envelope.Type, requested)
}

// liveTypeIdInSpace is typeIdInSpace with the store error kept apart from the
// miss. The nil-error twin is right where a lookup failure can only widen a
// refusal (a key that resolves elsewhere in the chain); here it would decide
// silently that a type has no settings, so this path takes the error.
func (s *Service) liveTypeIdInSpace(spaceId, typeKey string) (string, bool, error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, typeKey)
	if err != nil {
		return "", false, nil // not a type key shape; the type gate owns that verdict
	}
	details, err := s.store.SpaceIndex(spaceId).GetObjectByUniqueKey(uk)
	if err != nil {
		if errors.Is(err, spaceindex.ErrObjectNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read type %s of space %s: %w", typeKey, spaceId, err)
	}
	id := details.GetString(bundle.RelationKeyId)
	return id, id != "", nil
}

// liveType is one type row a write decision may rest on: found by its exact
// stored key or id, and live (not archived, deleted or uninstalled).
type liveType struct {
	Key  string // the stored type key
	Id   string
	Name string // the display name, so a warning never falls back to the key
}

// liveTypeRow reports whether a type row may decide anything.
func liveTypeRow(row *domain.Details) bool {
	return row.GetString(bundle.RelationKeyId) != "" &&
		!row.GetBool(bundle.RelationKeyIsArchived) &&
		!row.GetBool(bundle.RelationKeyIsDeleted) &&
		!row.GetBool(bundle.RelationKeyIsUninstalled)
}

// liveTypeByKey reads the live type row of one stored key, exactly. No slug,
// fold or name step runs: a decision about a type must never land on another
// type that merely answers to the same spelling. found=false is a confirmed
// absence; a store failure is an error, never a miss.
func (s *Service) liveTypeByKey(spaceId, typeKey string) (liveType, bool, error) {
	if typeKey == "" {
		return liveType{}, false, nil
	}
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, typeKey)
	if err != nil {
		return liveType{}, false, nil // not a type key shape
	}
	row, err := s.store.SpaceIndex(spaceId).GetObjectByUniqueKey(uk)
	if err != nil {
		if errors.Is(err, spaceindex.ErrObjectNotFound) {
			return liveType{}, false, nil
		}
		return liveType{}, false, fmt.Errorf("read type %s of space %s: %w", typeKey, spaceId, err)
	}
	if !liveTypeRow(row) {
		return liveType{}, false, nil
	}
	return liveType{Key: typeKey, Id: row.GetString(bundle.RelationKeyId), Name: row.GetString(bundle.RelationKeyName)}, true, nil
}

// templateTarget is the type a template object is for. Key is its stored
// key, empty when nothing names one; Live is its live row, set only when
// IsLive. Only a live target decides anything (a name's scope, an off-type
// warning); the key alone spells the read's template_for.
type templateTarget struct {
	Key    string
	Live   liveType
	IsLive bool
	// DetailUnresolved: targetObjectType is present but names no type row
	// this space holds. The target is unknown, and the second type key it
	// supersedes is not served in its place.
	DetailUnresolved bool
}

// isTemplateObject reports whether a stored object is a template: by its
// smartblock type, the kind its read serves. Its own type is then
// `template`, whatever its first type key says (withTemplateTarget).
func isTemplateObject(sbType model.SmartBlockType) bool {
	return sbType == model.SmartBlockType_Template
}

// templateTargetOf resolves the type a template is for, from its details and
// stored type keys. targetObjectType decides first: heart's own template
// application reads it, and a retarget moves it while the type keys stay
// behind. The second type key answers only when the detail is absent. A
// detail naming no type row this space holds is an unknown target, not a
// reason to trust a second key a retarget may have left behind.
func (s *Service) templateTargetOf(spaceId string, details *types.Struct, typeKeys []domain.TypeKey) (templateTarget, error) {
	// heart stores the detail as one id; the object channel writes a
	// reference as a one-element list, so both spellings are the same target
	switch ids := pbtypes.GetStringList(details, bundle.RelationKeyTargetObjectType.String()); {
	case len(ids) > 1:
		return templateTarget{DetailUnresolved: true}, nil
	case len(ids) == 1:
		id := ids[0]
		row, err := s.store.SpaceIndex(spaceId).GetDetails(id)
		if err != nil {
			return templateTarget{}, fmt.Errorf("read template target %s of space %s: %w", id, spaceId, err)
		}
		if row.GetString(bundle.RelationKeyId) != id {
			return templateTarget{DetailUnresolved: true}, nil // a miss reads as empty details
		}
		key, err := domain.GetTypeKeyFromRawUniqueKey(row.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			return templateTarget{DetailUnresolved: true}, nil // the row is not a type
		}
		target := templateTarget{Key: string(key)}
		if liveTypeRow(row) {
			target.Live = liveType{Key: target.Key, Id: id, Name: row.GetString(bundle.RelationKeyName)}
			target.IsLive = true
		}
		return target, nil
	}
	if len(typeKeys) < 2 || typeKeys[1] == "" {
		return templateTarget{}, nil
	}
	target := templateTarget{Key: string(typeKeys[1])}
	live, found, err := s.liveTypeByKey(spaceId, target.Key)
	if err != nil {
		return templateTarget{}, fmt.Errorf("resolve template target: %w", err)
	}
	target.Live, target.IsLive = live, found
	return target, nil
}

// withTemplateTarget is a template snapshot whose type keys say what its
// document serves: `template` as its own type, and the target
// templateTargetOf resolved in the second slot, the one the format reads
// template_for from, however many slots the object stored. Keys past the
// target still reach the codec and its loss warning, except beside a target
// the detail moved out of this space's sight: no stale slot is served then,
// and templateRenderLoss reports the keys that go with it.
func withTemplateTarget(snapshot *model.SmartBlockSnapshotBase, target templateTarget) *model.SmartBlockSnapshotBase {
	if snapshot == nil {
		return snapshot
	}
	objectTypes := []string{bundle.TypeKeyTemplate.URL()}
	switch {
	case target.Key != "":
		objectTypes = append(objectTypes, domain.TypeKey(target.Key).URL())
		if len(snapshot.ObjectTypes) > 2 {
			objectTypes = append(objectTypes, snapshot.ObjectTypes[2:]...)
		}
	case !target.DetailUnresolved && len(snapshot.ObjectTypes) > 1:
		objectTypes = append(objectTypes, snapshot.ObjectTypes[1:]...)
	}
	if slices.Equal(objectTypes, snapshot.ObjectTypes) {
		return snapshot
	}
	out := *snapshot
	out.ObjectTypes = objectTypes
	return &out
}

// templateRenderLoss is the loss withTemplateTarget cannot leave to the
// codec: type keys past the target slot of a template whose target is
// unresolved, which its document does not carry. A read reports it, and a
// PATCH refuses as for any other loss (C11).
func templateRenderLoss(snapshot *model.SmartBlockSnapshotBase, target templateTarget) []v2model.Issue {
	if snapshot == nil || !target.DetailUnresolved || len(snapshot.ObjectTypes) <= 2 {
		return nil
	}
	return []v2model.Issue{{
		Path: "/type",
		Message: fmt.Sprintf("object types %s are dropped: this template's target cannot be resolved, so its document carries no template_for and no key past it",
			strings.Join(snapshot.ObjectTypes[2:], ", ")),
	}}
}

// templateScope is the property scope of a template's own keys: its live
// target's list, and the template type's own when the target is unknown or
// has no live row. A store failure is an error, never a fallback.
func (s *Service) templateScope(spaceId string, target templateTarget) (map[string]bool, error) {
	if target.IsLive {
		scope, err := s.typePropertyScopeOf(spaceId, target.Live.Id)
		if err != nil {
			return nil, fmt.Errorf("read the template target's properties: %w", err)
		}
		return scope, nil
	}
	own, found, err := s.liveTypeByKey(spaceId, string(bundle.TypeKeyTemplate))
	if err != nil || !found {
		return nil, err
	}
	return s.typePropertyScopeOf(spaceId, own.Id)
}

// inspectTemplate reads one template candidate and reports its display name
// plus, when it cannot start an object of typeId, a caller-facing phrase
// saying why. A store failure is an error, never an empty verdict: an
// unreadable row must not pass as a usable template.
//
// The checks are the ones a caller cannot make from the outside, in the order
// a wrong id most often fails them: the object is gone, it is not a template,
// it belongs to another type. An empty typeId skips the last one and only the
// last one: the type has no store row to compare against, which is not the
// template's fault.
func (s *Service) inspectTemplate(ctx context.Context, spaceId, templateId, typeId string) (name string, problem string, err error) {
	row, err := s.store.SpaceIndex(spaceId).GetDetails(templateId)
	if err != nil {
		return "", "", fmt.Errorf("read template %s: %w", templateId, err)
	}
	// a miss yields empty details rather than an error, so identity is the
	// existence test
	if row.GetString(bundle.RelationKeyId) != templateId {
		return "", fmt.Sprintf("no object %q exists in this space", templateId), nil
	}
	name = row.GetString(bundle.RelationKeyName)
	if deletedRow(row) {
		return name, fmt.Sprintf("template %q was deleted permanently", templateId), nil
	}
	if row.GetBool(bundle.RelationKeyIsDeleted) || row.GetBool(bundle.RelationKeyIsArchived) || row.GetBool(bundle.RelationKeyIsUninstalled) {
		return name, fmt.Sprintf("template %q is deleted", templateId), nil
	}
	templateTypeId, _, err := s.liveTypeIdInSpace(spaceId, string(bundle.TypeKeyTemplate))
	if err != nil {
		return name, "", err
	}
	target, isTemplate, err := s.templateRowTargetId(ctx, spaceId, row, templateTypeId)
	if err != nil {
		return "", "", err
	}
	if !isTemplate {
		return name, fmt.Sprintf("object %q is not a template", templateId), nil
	}
	if typeId == "" {
		return name, "", nil
	}
	if target != typeId {
		if targetKey := s.servedTypeKeyById(spaceId, target); targetKey != "" {
			return name, fmt.Sprintf("template %q starts an object of type %q", templateId, targetKey), nil
		}
		return name, fmt.Sprintf("template %q starts an object of another type", templateId), nil
	}
	return name, "", nil
}

// templateRowTargetId answers, for an index row, whether its object is a
// template and which type id it is for. The index carries an object's first
// type key, not its kind, so the usual shape is read off the row: a row of
// the template type that names one target is one, and a row that is neither
// of the template type nor names a target is not. Any other row is answered
// by its object: its smartblock type decides, and its target is the one GET
// serves (templateTargetOf). A target this space has not installed yet is
// its derivable id, the one a create of that type compares against.
func (s *Service) templateRowTargetId(ctx context.Context, spaceId string, row *domain.Details, templateTypeId string) (targetId string, isTemplate bool, err error) {
	ids := row.WrapToStringList(bundle.RelationKeyTargetObjectType)
	ofTemplateType := templateTypeId != "" && row.GetString(bundle.RelationKeyType) == templateTypeId
	switch {
	case ofTemplateType && len(ids) > 1:
		return "", true, nil // a template is for one type; several is unresolved
	case ofTemplateType && len(ids) == 1:
		return ids[0], true, nil
	case !ofTemplateType && len(ids) == 0:
		// neither the template type nor a target: no path finds it as a
		// template, and nothing reads its object to say otherwise
		return "", false, nil
	}
	templateId := row.GetString(bundle.RelationKeyId)
	read, err := s.reader.ReadObject(ctx, spaceId, templateId)
	if err != nil {
		return "", false, fmt.Errorf("read template %s: %w", templateId, err)
	}
	if read.Snapshot == nil || !isTemplateObject(read.SbType) {
		return "", false, nil
	}
	target, err := s.templateTargetOf(spaceId, read.Snapshot.GetDetails(), readTypeKeys(read.Snapshot.GetObjectTypes()))
	if err != nil {
		return "", true, err
	}
	switch {
	case target.IsLive:
		return target.Live.Id, true, nil
	case target.Key != "" && bundle.HasObjectTypeByKey(domain.TypeKey(target.Key)):
		derived, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(target.Key))
		if err != nil {
			return "", true, fmt.Errorf("derive type id for %s: %w", target.Key, err)
		}
		return derived, true, nil
	}
	return "", true, nil
}

// typeDefaultTemplate reads a type's default_template.
//
// The detail is read as a LIST because it is stored both ways: the format
// accepts a single object reference either as a string or as a one-element
// list, this API writes the list form (which is what the clients read), and
// older types hold the bare string. Reading one spelling would make the
// default silently absent for half the types in a space.
func (s *Service) typeDefaultTemplate(spaceId, typeId string) (string, error) {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return "", fmt.Errorf("read type %s: %w", typeId, err)
	}
	for _, id := range details.WrapToStringList(bundle.RelationKeyDefaultTemplateId) {
		if id != "" {
			return id, nil
		}
	}
	return "", nil
}

// servedTypeSpelling is the api spelling of a type a refusal or warning has
// to name: by its id when one resolves, and by the stored key only when it
// does not. A canonicalized key is the STORE's spelling, and for a type this
// space minted that is a 24-hex id — the caller sent a slug, and quoting the
// id back is the leak the search refusals were rebuilt to close.
func (s *Service) servedTypeSpelling(spaceId, storedKey, typeId string) string {
	if served := s.servedTypeKeyById(spaceId, typeId); served != "" {
		return served
	}
	// no live row: a bundled key still has a served spelling of its own, and
	// they differ — the stored `set` is served as `query`
	if bundle.HasObjectTypeByKey(domain.TypeKey(storedKey)) {
		if slug := bundle.TypeApiSlug(storedKey); slug != "" {
			return slug
		}
	}
	return storedKey
}

// servedTypeKeyById spells one type id the way every other v2 response spells
// it (the api slug), or "" when the id resolves to no live type.
func (s *Service) servedTypeKeyById(spaceId, typeId string) string {
	if typeId == "" {
		return ""
	}
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return ""
	}
	keyTaken, slugHolders := servedTypeKeySets(entries)
	for _, entry := range entries {
		if entry.Id == typeId {
			return servedTypeKeyOf(entry.Key, entry.Slug, keyTaken, slugHolders)
		}
	}
	return ""
}

// ListTemplates implements GET /v2/spaces/{space_id}/templates: the read half
// of the collection POST /templates writes to.
//
// It is a separate operation rather than a search filter because search
// excludes templates from every result by design (they are not content a
// search should return), which left their ids reachable only from the
// response that created one. typeTerm narrows the list to the templates of
// one type, which is the question a create actually asks.
func (s *Service) ListTemplates(ctx context.Context, spaceId, typeTerm string, offset, limit int) ([]v2model.TemplateRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	// archived and deleted rows are excluded by the store's query defaults;
	// these two are not, and a listing that offers a template the create path
	// then refuses is the same broken loop in the other direction
	filters := []database.FilterRequest{
		{
			RelationKey: "type.uniqueKey",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(bundle.TypeKeyTemplate.URL()),
		},
		{
			RelationKey: bundle.RelationKeyIsUninstalled,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	listedFor := ""
	if typeTerm != "" {
		entry, ok, ambiguous, err := s.resolveTypeInput(spaceId, typeTerm, entries)
		if err != nil {
			return nil, 0, false, err
		}
		if len(ambiguous) > 0 {
			return nil, 0, false, ambiguousKeyError("type key", typeTerm, "type", ambiguous)
		}
		if !ok {
			return nil, 0, false, s.unknownTypeKeyError(spaceId, typeTerm, "type", errKeysFor(ctx))
		}
		targetId := entry.Id
		if targetId == "" {
			// a bundled type this space has not installed yet: its id is
			// derivable and is what a template of it carries as its target,
			// which is how the CREATE path resolves it. A listing that
			// refused here would refuse to show templates a create accepts.
			// Deriving it needs the loaded space, the one step of this
			// store read that does: bounded here, and only on this branch
			if err := s.ensureSpaceLoaded(ctx, spaceId); err != nil {
				return nil, 0, false, err
			}
			derived, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(entry.Key))
			if err != nil {
				return nil, 0, false, fmt.Errorf("derive type id for %s: %w", entry.Key, err)
			}
			targetId = derived
		}
		listedFor = targetId
	}
	byDate := []database.SortRequest{{
		RelationKey: bundle.RelationKeyLastModifiedDate,
		Type:        model.BlockContentDataviewSort_Desc,
		IncludeTime: true,
	}}
	// the index carries an object's first type key, not its kind: the rows
	// of the template type and the rows elsewhere that name a target are the
	// candidates, each answered by templateRowTargetId, and the page is cut
	// here (templates are few)
	templateTypeId, _, err := s.liveTypeIdInSpace(spaceId, string(bundle.TypeKeyTemplate))
	if err != nil {
		return nil, 0, false, err
	}
	ofTemplateType, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: filters, Sorts: byDate})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query templates in space %s: %w", spaceId, err)
	}
	elsewhere, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: append([]database.FilterRequest{
		{RelationKey: "type.uniqueKey", Condition: model.BlockContentDataviewFilter_NotEqual, Value: domain.String(bundle.TypeKeyTemplate.URL())},
		{RelationKey: bundle.RelationKeyTargetObjectType, Condition: model.BlockContentDataviewFilter_NotEmpty},
	}, filters[1:]...), Sorts: byDate})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query templates in space %s: %w", spaceId, err)
	}
	targetOf := map[string]string{}
	var all []database.Record
	for _, row := range append(ofTemplateType, elsewhere...) {
		target, isTemplate, err := s.templateRowTargetId(ctx, spaceId, row.Details, templateTypeId)
		if err != nil {
			return nil, 0, false, err
		}
		if !isTemplate || (listedFor != "" && target != listedFor) {
			continue
		}
		targetOf[row.Details.GetString(bundle.RelationKeyId)] = target
		all = append(all, row)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Details.GetInt64(bundle.RelationKeyLastModifiedDate) > all[j].Details.GetInt64(bundle.RelationKeyLastModifiedDate)
	})
	total := len(all)
	var records []database.Record
	if offset < len(all) {
		records = all[offset:min(len(all), offset+limit+1)]
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	keyTaken, slugHolders := servedTypeKeySets(entries)
	keyById := make(map[string]string, len(entries))
	for _, entry := range entries {
		keyById[entry.Id] = servedTypeKeyOf(entry.Key, entry.Slug, keyTaken, slugHolders)
	}
	// one read per DISTINCT target type in the page, not one per row: a type
	// with twenty templates is the common case
	defaults := map[string]string{}
	rows := make([]v2model.TemplateRow, 0, len(records))
	for _, record := range records {
		targetId := targetOf[record.Details.GetString(bundle.RelationKeyId)]
		if _, seen := defaults[targetId]; !seen && targetId != "" {
			defaultId, err := s.typeDefaultTemplate(spaceId, targetId)
			if err != nil {
				return nil, 0, false, err
			}
			defaults[targetId] = defaultId
		}
		id := record.Details.GetString(bundle.RelationKeyId)
		rows = append(rows, v2model.TemplateRow{
			Id:          id,
			Name:        record.Details.GetString(bundle.RelationKeyName),
			TemplateFor: keyById[targetId],
			Default:     id != "" && defaults[targetId] == id,
		})
	}
	return rows, total, hasMore, nil
}

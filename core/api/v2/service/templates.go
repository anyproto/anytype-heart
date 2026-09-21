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

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		name, problem, err := s.inspectTemplate(spaceId, requested, targetId)
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
	name, problem, err := s.inspectTemplate(spaceId, defaultId, typeId)
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
func (s *Service) inspectTemplate(spaceId, templateId, typeId string) (name string, problem string, err error) {
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
	if row.GetBool(bundle.RelationKeyIsDeleted) || row.GetBool(bundle.RelationKeyIsArchived) || row.GetBool(bundle.RelationKeyIsUninstalled) {
		return name, fmt.Sprintf("template %q is deleted", templateId), nil
	}
	templateTypeId, ok, err := s.liveTypeIdInSpace(spaceId, string(bundle.TypeKeyTemplate))
	if err != nil {
		return name, "", err
	}
	if !ok || row.GetString(bundle.RelationKeyType) != templateTypeId {
		return name, fmt.Sprintf("object %q is not a template", templateId), nil
	}
	if target := row.GetString(bundle.RelationKeyTargetObjectType); typeId != "" && target != typeId {
		if targetKey := s.servedTypeKeyById(spaceId, target); targetKey != "" {
			return name, fmt.Sprintf("template %q starts an object of type %q", templateId, targetKey), nil
		}
		return name, fmt.Sprintf("template %q starts an object of another type", templateId), nil
	}
	return name, "", nil
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
			// refused here would refuse to show templates a create accepts
			derived, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(entry.Key))
			if err != nil {
				return nil, 0, false, fmt.Errorf("derive type id for %s: %w", entry.Key, err)
			}
			targetId = derived
		}
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyTargetObjectType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(targetId),
		})
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		Filters: filters,
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
		Offset: offset,
		Limit:  limit + 1, // one extra record detects has_more without a second scan
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query templates in space %s: %w", spaceId, err)
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
		targetId := record.Details.GetString(bundle.RelationKeyTargetObjectType)
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

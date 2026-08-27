package v2service

// schema_write.go implements the Phase-2 schema write surface (APIV2.md
// §2): POST/PATCH/DELETE for types (kind:"object_type" AnyBlock documents —
// typeProperties creates missing properties, SPEC §2a) and properties
// ({key?, name, format, options?}).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/types"
)

// detailKeyId mirrors the anyblockjson envelope lift: the minted id detail
// never travels into create RPC payloads.
const v2DetailKeyId = "id"

// The bounds the §5 discovery schemas advertise on the typed Phase-2
// bodies (schemas.go: the property, set, collection and file kinds),
// enforced here so the strict schemas stay true (surface review M6): an
// advertised bound the endpoint never checks either steers schema-obedient
// agents into rejections or lets a hallucinated megabyte through
// unchecked. The drift test in schemas_test.go pins the served schema JSON
// to these constants — change one and the test names the other.
const (
	maxV2NameLength        = 4096 // name fields and option names (maxLength)
	maxV2KeyLength         = 256  // property/type keys and object ids (maxLength)
	maxV2PropertyOptions   = 100  // property options (maxItems)
	maxV2OptionColorLength = 64   // option color (maxLength)
	maxV2FilterLength      = 4096 // the compact filter string (maxLength)
	maxV2SetSorts          = 10   // set sorts (maxItems)
	maxV2SetViews          = 10   // set views (maxItems)
	maxV2CollectionItems   = 1000 // collection items (maxItems)
	maxV2UrlLength         = 4096 // file source url (maxLength)
)

// v2PropertyKeyPattern is the advertised key pattern (the property kind's
// `pattern` on /key).
var v2PropertyKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// validateV2FieldLength enforces one advertised maxLength, counted in
// Unicode code points (JSON Schema maxLength semantics — the space-field
// precedent, space.go).
func validateV2FieldLength(path, value string, max int) error {
	if length := utf8.RuneCountInString(value); length > max {
		return v2model.ValidationFailed(strings.TrimPrefix(path, "/")+" is too long",
			v2model.Issue{Path: path,
				Message: fmt.Sprintf("%d characters — the cap is %d (the advertised maxLength)", length, max)})
	}
	return nil
}

// validateV2ArrayCount enforces one advertised maxItems on a raw JSON array
// field. A non-array shape is left for the field's own decoder, which
// rejects it with its targeted message.
func validateV2ArrayCount(path string, raw json.RawMessage, max int) error {
	if len(raw) == 0 {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	if len(items) > max {
		return v2model.ValidationFailed(strings.TrimPrefix(path, "/")+" has too many items",
			v2model.Issue{Path: path,
				Message: fmt.Sprintf("%d items — the cap is %d (the advertised maxItems)", len(items), max)})
	}
	return nil
}

// CreateType implements POST /v2/spaces/{space_id}/types: a kind:"object_type"
// AnyBlock document; typeProperties creates missing properties atomically
// with the type (SPEC §2a create-missing).
func (s *Service) CreateType(ctx context.Context, spaceId string, body []byte, dryRun, createMissingOptions bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// envelope normalization like every other create: a pasted GetType read
	// carries etag (and possibly warnings) — without the strip, POST types
	// 400ed on the etag of its own read; the ?block= subtree marker is
	// refused by name instead of as an anonymous unknown field
	body, err := normalizeCreateBody(body)
	if err != nil {
		return nil, err
	}
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
	}

	// the endpoint IS the kind: inject/enforce kind object_type and default
	// the version so a bare {type_settings} document works
	if raw, ok := fields["kind"]; ok {
		var kind string
		if err := json.Unmarshal(raw, &kind); err != nil || kind != "object_type" {
			return nil, v2model.ValidationFailed("not a type document",
				v2model.Issue{Path: "/kind", Message: "POST types accepts kind \"object_type\" documents only"})
		}
	} else if fields["kind"], err = rawJSON("object_type"); err != nil {
		return nil, err
	}
	if _, ok := fields["version"]; !ok {
		if fields["version"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
			return nil, err
		}
	}
	if _, ok := fields["blocks"]; ok {
		// deferred: a type's dataview block on create (the editor generates
		// default views at first open — SPEC §2a); explicit beats silent loss
		return nil, v2model.ValidationFailed("type blocks are not supported on create",
			v2model.Issue{Path: "/blocks", Message: "omit blocks — the editor generates the type's default views", Hint: "customize views in the app after creating the type"})
	}
	if body, err = encodeEnvelope(fields); err != nil {
		return nil, err
	}
	if err := s.rejectInvalidDocument(body); err != nil {
		return nil, err
	}

	var envelope docEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, v2model.ValidationFailed("decode document envelope: " + err.Error())
	}

	// The (a) identity layer (ADDRESSING §7.5): the document's key never
	// becomes the type's uniqueKey — the create mints a BSON internal key
	// and the caller's key lives in the apiObjectKey slug, snake-normalized.
	// The union collision check ships WITH the mint (§7.6-3): bundled keys,
	// bundled-derived slugs (a custom "objectType"/"object_type" cannot
	// shadow the bundled type), live internal keys and live slugs — with
	// corpses vacated (§8-OQ2), so delete-then-recreate mints cleanly.
	var slug string
	// §2a moved the caller's proposed slug out of the envelope `key` and into
	// `type_settings.api_key` — the same value under the name it always had
	// in the store (apiObjectKey).
	apiKey := envelope.apiKey()
	keyPath := "/type_settings/api_key"
	if apiKey != "" {
		if err := validateV2FieldLength(keyPath, apiKey, maxV2KeyLength); err != nil {
			return nil, err
		}
		if !v2PropertyKeyPattern.MatchString(apiKey) {
			return nil, v2model.ValidationFailed("invalid type key",
				v2model.Issue{Path: keyPath,
					Message: fmt.Sprintf("key %q does not match the advertised pattern ^[a-zA-Z0-9_]+$", apiKey),
					Hint:    "use letters, digits and underscores — or omit api_key to derive one from the name"})
		}
		slug = bundle.ApiSlug(apiKey)
	}

	if slug == "" {
		// no explicit api_key: the slug derives from the document's name (the
		// same transform objectcreator would apply, sanitized to the key
		// grammar) — read from the ENVELOPE so the union check can run
		// BEFORE the resolver creates anything
		if raw, ok := envelope.Properties["name"]; ok {
			var name string
			if err := json.Unmarshal(raw, &name); err == nil {
				slug = sanitizeApiSlug(bundle.ApiSlugFromName(name))
			}
		}
		keyPath = "/properties/name"
	}
	if slug != "" {
		// the union collision check runs before Unmarshal: a refused type
		// create must not leave the typeProperties it would have carried as
		// orphan relations (the M5 lesson — reject before any side effect)
		typeEntries, err := s.liveTypes(spaceId)
		if err != nil {
			return nil, err
		}
		if holder, taken := s.typeSlugConflict(slug, typeEntries); taken {
			if holder.Kind == "bundled type" {
				return nil, v2model.ValidationFailed("type key is reserved",
					v2model.Issue{Path: keyPath,
						Message: fmt.Sprintf("key %q is taken by bundled type %q — it already exists", slug, holder.Name)})
			}
			return nil, v2model.ValidationFailed("type key already exists",
				v2model.Issue{Path: keyPath,
					Message: fmt.Sprintf("key %q is taken by %s %q in space %s", slug, holder.Kind, holder.Name, spaceId),
					Hint:    fmt.Sprintf("update it with PATCH /v2/spaces/%s/types/%s, or pick a different key", spaceId, holder.Key)})
		}
	}

	// the SPEC §2a format check, at the wiring and BEFORE anything is
	// created (§7.5-requirement-4)
	if len(envelope.propertyDefinitions()) > 0 {
		var declared []anyblockjson.TypeProperty
		if err := json.Unmarshal(envelope.propertyDefinitions(), &declared); err == nil {
			if err := s.validateTypePropertyFormats(spaceId, declared); err != nil {
				return nil, err
			}
		}
	}

	// Unmarshal rebuilds the four recommended-relation lists from
	// typeProperties, creating missing properties through the resolver
	resolvers := s.newCreatingResolvers(ctx, spaceId, dryRun, createMissingOptions)
	_, snapshot, err := anyblockjson.Unmarshal(body, resolvers.Options())
	if err != nil {
		return nil, mapUnmarshalError(body, err)
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve type properties: %w", err)
	}

	result := &v2model.CreateResult{Key: slug, Created: resolvers.created()}
	if dryRun {
		result.DryRun = true
		return result, nil
	}

	details, err := typeDetailsFromSnapshot(snapshot, slug)
	if err != nil {
		return nil, err
	}
	if len(envelope.propertyDefinitions()) > 0 {
		ensureRegularRecommendedList(details, resolvers)
		if err := resolvers.err(); err != nil {
			return nil, fmt.Errorf("resolve default recommended properties: %w", err)
		}
	}
	resp := s.mw.ObjectCreateObjectType(ctx, &pb.RpcObjectCreateObjectTypeRequest{
		SpaceId: spaceId,
		Details: details,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateObjectTypeResponseError_NULL {
		return nil, fmt.Errorf("create type in space %s: %s", spaceId, resp.Error.Description)
	}
	result.Id = resp.ObjectId
	// the same read-back as CreateProperty: the mint may have suffixed or
	// dropped the slug v2 proposed, and the key a 201 returns must be one the
	// key routes accept
	result.Key = storedApiKeyOf(resp.Details, result.Key)
	if result.Key == "" && resp.Details != nil {
		if uk := pbtypes.GetString(resp.Details, bundle.RelationKeyUniqueKey.String()); uk != "" {
			if key, err := domain.GetTypeKeyFromRawUniqueKey(uk); err == nil {
				result.Key = string(key)
			}
		}
	}
	if read, err := s.reader.ReadObject(ctx, spaceId, resp.ObjectId); err == nil {
		result.Etag = ComputeEtag(read.Heads)
	}
	return result, nil
}

// ensureRegularRecommendedList keeps ObjectCreateObjectType's
// FillRecommendedRelations on its "already filled" path: it detects filled
// lists by the FIRST entry of recommendedRelations being a space-local id,
// so a type document whose typeProperties are all featured/hidden/file
// (regular section empty) would get its lists clobbered by layout defaults.
// Seed the regular list with the system default sidebar properties
// (relationutils.defaultRecommendedRelationKeys) — the set every type in
// the product carries.
func ensureRegularRecommendedList(details *types.Struct, resolvers *creatingResolvers) {
	key := bundle.RelationKeyRecommendedRelations.String()
	if v, ok := details.Fields[key]; ok && len(v.GetListValue().GetValues()) > 0 {
		return
	}
	var ids []string
	for _, defKey := range []domain.RelationKey{
		bundle.RelationKeyCreatedDate,
		bundle.RelationKeyCreator,
		bundle.RelationKeyLinks,
	} {
		if id, ok := resolvers.PropertyId(anyblockjson.PropertyDefinition{Key: defKey}); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		details.Fields[key] = pbtypes.StringList(ids)
	}
}

// storedApiKeyOf reads the apiObjectKey the MINT actually stored out of a
// create response's details, falling back to the proposal when the response
// carries no details at all (nothing to read back from — the proposal is then
// the best available answer). An empty stored slug is authoritative and
// returns "": it means the mint found no free spelling and the internal key
// is the only address, which the caller must be told rather than handed a
// key that resolves to nothing.
func storedApiKeyOf(details *types.Struct, proposed string) string {
	if details == nil {
		return proposed
	}
	return pbtypes.GetString(details, bundle.RelationKeyApiObjectKey.String())
}

// typeDetailsForbiddenKeys are identity- and permission-bearing details a
// type DOCUMENT must never supply: uniqueKey rides through the details
// channel into getUniqueKeyOrGenerate and DeriveTreeObject — a forged
// "ot-page" occupies the id a later bundled install converges to, which is
// strategy (b)'s silent merge reachable under (a) through a channel the
// union check never inspects. relationKey/isReadonly/restrictions are the
// system's own flags. Export strips all of them (derived/local source), so
// no legitimate round-tripped document carries one — rejection is loud,
// path-addressed, and breaks nothing real.
var typeDetailsForbiddenKeys = []string{
	bundle.RelationKeyUniqueKey.String(),
	bundle.RelationKeyRelationKey.String(),
	bundle.RelationKeyIsReadonly.String(),
	bundle.RelationKeyRestrictions.String(),
}

// typeDetailsDroppedKeys are system-managed details the create path
// computes itself; a round-tripped document may legitimately carry them
// (details-source, so export emits them), so they are dropped in favor of
// the system's value rather than rejected. apiObjectKey in particular is
// derived from the document's key/name and union-checked — a document-
// supplied value would bypass the check.
var typeDetailsDroppedKeys = map[string]bool{
	bundle.RelationKeyApiObjectKey.String():  true,
	bundle.RelationKeyOrigin.String():        true,
	bundle.RelationKeySpaceId.String():       true,
	bundle.RelationKeyIsArchived.String():    true,
	bundle.RelationKeyIsDeleted.String():     true,
	bundle.RelationKeyIsUninstalled.String(): true,
}

// typeDetailsFromSnapshot converts the unmarshaled type snapshot into the
// ObjectCreateObjectType details: the minted id dropped, the caller's key
// stored as the apiObjectKey slug (NEVER as the unique key — the create
// mints a BSON internal key, ADDRESSING §7.5), identity-bearing details
// rejected (see typeDetailsForbiddenKeys), and recommendedLayout accepting
// the layout NAME (the §2a worked example's form) as well as the stored
// number.
func typeDetailsFromSnapshot(snapshot *model.SmartBlockSnapshotBase, slug string) (*types.Struct, error) {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	if snapshot.Details != nil {
		for _, forbidden := range typeDetailsForbiddenKeys {
			if _, ok := snapshot.Details.Fields[forbidden]; ok {
				return nil, v2model.ValidationFailed("property is not writable on a type",
					v2model.Issue{Path: "/properties/" + forbidden,
						Message: fmt.Sprintf("%q is system-managed and cannot be supplied", forbidden),
						Hint:    "identity comes from the document's key (and name); remove this property"})
			}
		}
		for k, v := range snapshot.Details.Fields {
			if k == v2DetailKeyId || typeDetailsDroppedKeys[k] {
				continue
			}
			details.Fields[k] = v
		}
	}
	if slug != "" {
		details.Fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(slug)
	}
	if v, ok := details.Fields[bundle.RelationKeyRecommendedLayout.String()]; ok {
		if name := v.GetStringValue(); name != "" {
			layout, ok := model.ObjectTypeLayout_value[name]
			if !ok {
				return nil, v2model.ValidationFailed("unknown layout name",
					v2model.Issue{Path: "/properties/recommendedLayout", Message: fmt.Sprintf("unknown layout %q", name), Hint: "common layouts: basic, todo, note, profile"})
			}
			details.Fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Int64(int64(layout))
		}
	}
	details.Fields[bundle.RelationKeyOrigin.String()] = pbtypes.Int64(int64(model.ObjectOrigin_api))
	return details, nil
}

// validateTypePropertyFormats implements the format check SPEC §2a promises
// at the wiring (ADDRESSING §2.3-5, §7.5-requirement-4): a typeProperties
// entry whose DECLARED format contradicts the format of the relation its
// key resolves to is a path-addressed 400 — before this, the declared
// format was silently ignored on a key hit and the entry's objects held
// wrong-shaped values. Under the (a) identity layer the check covers the
// remaining sequential-declaration case; the concurrent case no longer
// exists, because keys no longer collide. Entries that resolve to nothing
// (the creation case — the declared format IS the property's format) or
// carry no format pass through; unknown format names and ambiguous keys
// stay with the layers that own those refusals.
func (s *Service) validateTypePropertyFormats(spaceId string, props []anyblockjson.TypeProperty) error {
	var entries []propertyEntry
	for i, tp := range props {
		// §2e split `key` into the document-facing SPELLING (`property`) and
		// the STORED internal key (`internal_key`). An entry may state either,
		// so resolve on the spelling first — that is the term this API's
		// vocabulary accepts — and fall back to the internal key.
		term := tp.Property
		if term == "" {
			term = tp.InternalKey
		}
		if term == "" || tp.Format == "" {
			continue
		}
		declared, ok := anyblockjson.FormatByName(tp.Format)
		if !ok {
			continue
		}
		if entries == nil {
			var err error
			if entries, err = s.liveProperties(spaceId); err != nil {
				return err
			}
		}
		entry, ok, ambiguous := s.resolvePropertyInput(term, entries)
		if len(ambiguous) > 0 || !ok {
			continue
		}
		if entry.Format != declared {
			return v2model.ValidationFailed("property format conflict",
				v2model.Issue{
					Path: fmt.Sprintf("/type_properties/%d/format", i),
					Message: fmt.Sprintf("%q declares format %q but the existing property %q has format %q",
						term, tp.Format, entry.Name, anyblockjson.FormatName(entry.Format)),
					Hint: "omit format to use the existing property as it is, or create a new property under a different key",
				})
		}
	}
	return nil
}

// storedDetailKey maps a wire property key to its stored spelling through
// the bundled derived table — enough for every key in this file's maps,
// which are all bundled, and free of any store lookup.
func storedDetailKey(key string) string {
	if stored, ok := bundle.RelationKeyByApiSlug(key); ok {
		return string(stored)
	}
	return key
}

// updatableTypeDetailKeys is the explicit PATCH surface for a type's own
// `properties`; anything else is rejected (never silently dropped). §2a and
// §2b emptied this of everything but the two that are still ordinary
// properties: the layout moved to type_settings.layout and the icon became
// the typed envelope `icon`, and each is patched through the member it now
// lives in rather than through a flat spelling this endpoint alone would
// keep alive (C2 — a caller learns ONE vocabulary, and POST /types already
// speaks this one).
var updatableTypeDetailKeys = map[string]bool{
	"name": true, "description": true,
}

// typeSettingsPatchKeys is the PATCH surface of the §2a type_settings
// subtree, mapped to the stored detail key each member carries.
var typeSettingsPatchKeys = map[string]string{
	"layout":      "recommendedLayout",
	"plural_name": "pluralName",
}

// v2TypePatch is the PATCH types/{type} body: partial type-document
// semantics — `properties` updates the type's own details, `type_settings`
// updates the §2a settings subtree, and its `property_definitions` (when
// present) rebuilds the recommended lists, creating missing properties.
type v2TypePatch struct {
	Properties   map[string]json.RawMessage `json:"properties"`
	TypeSettings *v2TypeSettingsPatch       `json:"type_settings"`
	// Icon is §2b's typed envelope icon. It is decoded by the FORMAT rather
	// than by a variant table restated here: which detail keys a variant sets
	// is the format's rule, and a second statement of it is how the two
	// surfaces drift (the §2b lift exists because nine flat keys had no
	// single owner). See iconPatchDetails.
	Icon json.RawMessage `json:"icon"`
}

// v2TypeSettingsPatch is the patchable slice of type_settings. api_key is
// deliberately absent: the slug is identity, minted and union-checked at
// create, and re-pointing it would silently break every URL that names the
// type.
type v2TypeSettingsPatch struct {
	Layout              json.RawMessage              `json:"layout"`
	PluralName          json.RawMessage              `json:"plural_name"`
	PropertyDefinitions *[]anyblockjson.TypeProperty `json:"property_definitions"`
}

// propertyDefinitions is the patch's §2a definition array, nil-safe.
func (p v2TypePatch) propertyDefinitions() *[]anyblockjson.TypeProperty {
	if p.TypeSettings == nil {
		return nil
	}
	return p.TypeSettings.PropertyDefinitions
}

// UpdateType implements PATCH /v2/spaces/{space_id}/types/{type}.
func (s *Service) UpdateType(ctx context.Context, spaceId, typeKey string, body []byte, dryRun, createMissingOptions bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// live lookup, slug-aware — a UI-deleted type must 404, never steer the
	// caller into patching a corpse (§2.3-6)
	entry, err := s.requireLiveType(spaceId, typeKey, "/key")
	if err != nil {
		return nil, err
	}
	typeId := entry.Id

	var patch v2TypePatch
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return nil, v2model.ValidationFailed("invalid type patch",
			v2model.Issue{Message: err.Error(), Hint: "the patch accepts properties and type_settings"})
	}

	var detailUpdates []*model.Detail
	// two spellings of one key in one body: sortedKeys makes the winner
	// deterministic, which is not the same as correct — the caller asked for
	// two values on one detail and one of them is being dropped. Refuse, as
	// canonicalizeDocumentKeys does on the object channel.
	//
	// Unreachable while `properties` accepts only name and description, which
	// have one spelling each: a duplicate of anything else is refused as
	// not-updatable first, and that is the better error — a caller told
	// "duplicate" would fix it only to hear "not updatable" next. Kept for
	// the key that gains a second spelling, not deleted as dead.
	spelledBy := map[string]string{}
	for _, raw := range sortedKeys(patch.Properties) {
		// this channel does not go through canonicalizeDocumentKeys, so it
		// translates its own: the served schema advertises slugs (§7.5a) and
		// the keys below are stored spellings
		key := storedDetailKey(raw)
		if first, dup := spelledBy[key]; dup {
			return nil, v2model.ValidationFailed("duplicate property key",
				v2model.Issue{Path: "/properties/" + raw,
					Message: fmt.Sprintf("%q and %q both address %q — keep one", first, raw, key)})
		}
		spelledBy[key] = raw
		if !updatableTypeDetailKeys[key] {
			return nil, v2model.ValidationFailed("property not updatable on a type",
				v2model.Issue{Path: "/properties/" + raw, Message: fmt.Sprintf("cannot update %q", raw),
					Hint: "properties takes name and description; the layout is type_settings.layout and the icon is the typed envelope icon (§2a, §2b)"})
		}
		// the map is keyed by the WIRE spelling — reading it back with the
		// STORED one handed typeDetailValue a nil body for every key the two
		// vocabularies spell differently, so icon_emoji and recommended_layout
		// (the only two of the four that differ) 400'd on a value they carried
		value, err := typeDetailValue(key, "/properties/"+raw, patch.Properties[raw])
		if err != nil {
			return nil, err
		}
		detailUpdates = append(detailUpdates, &model.Detail{Key: key, Value: value})
	}

	// §2b's typed icon, decoded through the format itself (iconPatchDetails).
	if len(patch.Icon) > 0 {
		iconDetails, err := iconPatchDetails(patch.Icon)
		if err != nil {
			return nil, err
		}
		detailUpdates = append(detailUpdates, iconDetails...)
	}

	// the §2a settings subtree: each member maps to the stored detail key it
	// was lifted from, and reuses typeDetailValue so `layout` accepts a
	// layout NAME exactly as the create path does.
	if patch.TypeSettings != nil {
		for _, member := range sortedKeys(map[string]json.RawMessage{
			"layout": patch.TypeSettings.Layout, "plural_name": patch.TypeSettings.PluralName,
		}) {
			raw := map[string]json.RawMessage{
				"layout": patch.TypeSettings.Layout, "plural_name": patch.TypeSettings.PluralName,
			}[member]
			if len(raw) == 0 {
				continue
			}
			key := typeSettingsPatchKeys[member]
			value, err := typeDetailValue(key, "/type_settings/"+member, raw)
			if err != nil {
				return nil, err
			}
			detailUpdates = append(detailUpdates, &model.Detail{Key: key, Value: value})
		}
	}

	resolvers := s.newCreatingResolvers(ctx, spaceId, dryRun, createMissingOptions)
	if defs := patch.propertyDefinitions(); defs != nil {
		// the echo baseline (§8.41): entries this type ALREADY references
		// resolve as identities even when their relation is removed — the
		// GET/PATCH loop must not force-delete a reference the read served
		resolvers.echoPropertyIds = s.recommendedRelationIds(spaceId, typeId)
		// the SPEC §2a format check, before the resolver can create
		if err := s.validateTypePropertyFormats(spaceId, *defs); err != nil {
			return nil, err
		}
		lists, err := anyblockjson.BuildRecommendedLists(*defs, resolvers.Options())
		if err != nil {
			return nil, fmt.Errorf("build recommended lists: %w", err)
		}
		if err := resolvers.err(); err != nil {
			return nil, fmt.Errorf("resolve type properties: %w", err)
		}
		for _, list := range lists {
			detailUpdates = append(detailUpdates, &model.Detail{Key: list.DetailKey, Value: pbtypes.StringList(list.Ids)})
		}
	}

	result := &v2model.CreateResult{Id: typeId, Key: typeKey, Created: resolvers.created()}
	if dryRun {
		result.DryRun = true
		return result, nil
	}
	if len(detailUpdates) > 0 {
		resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{ContextId: typeId, Details: detailUpdates})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
			return nil, fmt.Errorf("update type %s: %s", typeKey, resp.Error.Description)
		}
	}
	if read, err := s.reader.ReadObject(ctx, spaceId, typeId); err == nil {
		result.Etag = ComputeEtag(read.Heads)
	}
	return result, nil
}

// iconPatchDetails turns §2b's typed `icon` into the stored detail keys it
// implies, by handing a minimal type document to the format's own importer
// and reading back what it set. The variant rules — which of iconEmoji /
// iconImage / iconName / iconOption a format selects, and what an
// out-of-vocabulary variant earns — stay stated once, in the format.
func iconPatchDetails(icon json.RawMessage) ([]*model.Detail, error) {
	doc, err := encodeEnvelope(map[string]json.RawMessage{
		"version": json.RawMessage(fmt.Sprintf("%d", anyblockjson.FormatVersion)),
		"kind":    json.RawMessage(`"object_type"`),
		"icon":    icon,
	})
	if err != nil {
		return nil, err
	}
	if err := anyblockjson.Validate(doc); err != nil {
		return nil, iconPatchError(err)
	}
	_, snapshot, err := anyblockjson.Unmarshal(doc, anyblockjson.Options{})
	if err != nil {
		return nil, iconPatchError(err)
	}
	var out []*model.Detail
	for _, key := range sortedKeys(snapshot.GetDetails().GetFields()) {
		// the probe document's own minted id is not part of the patch
		if key == v2DetailKeyId {
			continue
		}
		out = append(out, &model.Detail{Key: key, Value: snapshot.Details.Fields[key]})
	}
	if len(out) == 0 {
		return nil, v2model.ValidationFailed("icon sets nothing",
			v2model.Issue{Path: "/icon", Message: "the icon carries no value the store can hold"})
	}
	return out, nil
}

// iconPatchError re-addresses the probe document's issues onto /icon — the
// caller sent an icon, not a document.
func iconPatchError(err error) error {
	var ve *anyblockjson.ValidationError
	if !errors.As(err, &ve) {
		return v2model.ValidationFailed("invalid icon", v2model.Issue{Path: "/icon", Message: err.Error()})
	}
	issues := make([]v2model.Issue, 0, len(ve.Issues))
	for _, iss := range ve.Issues {
		path := "/icon"
		if trimmed := strings.TrimPrefix(iss.Path, "/icon"); trimmed != iss.Path && trimmed != "" {
			path += trimmed
		}
		issues = append(issues, v2model.Issue{Path: path, Message: iss.Message})
	}
	return v2model.ValidationFailed("invalid icon", issues...)
}

// typeDetailValue decodes one PATCH type value. `key` is the stored spelling
// the value is decoded FOR; `path` is the pointer INTO THE REQUEST the value
// arrived at, and every issue uses it — an error naming a slot the request
// never sent is unactionable (the old paths said /properties/iconEmoji for
// icon_emoji, and would now say /properties/type_settings/layout).
func typeDetailValue(key, path string, raw json.RawMessage) (*types.Value, error) {
	if key == "recommendedLayout" {
		var name string
		if err := json.Unmarshal(raw, &name); err == nil {
			layout, ok := model.ObjectTypeLayout_value[name]
			if !ok {
				return nil, v2model.ValidationFailed("unknown layout name",
					v2model.Issue{Path: path, Message: fmt.Sprintf("unknown layout %q", name), Hint: "common layouts: basic, todo, note, profile"})
			}
			return pbtypes.Int64(int64(layout)), nil
		}
		var number int64
		if err := json.Unmarshal(raw, &number); err == nil {
			return pbtypes.Int64(number), nil
		}
		return nil, v2model.ValidationFailed("invalid recommendedLayout",
			v2model.Issue{Path: path, Message: "expected a layout name or number"})
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return nil, v2model.ValidationFailed("invalid property value",
			v2model.Issue{Path: path, Message: "expected a string"})
	}
	return pbtypes.String(str), nil
}

// DeleteType implements DELETE /v2/spaces/{space_id}/types/{type} (archive —
// v1 parity; hard delete is deferred with ?permanent).
func (s *Service) DeleteType(ctx context.Context, spaceId, typeKey string, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// live lookup, slug-aware — deleting a corpse is a 404, not a re-archive
	entry, err := s.requireLiveType(spaceId, typeKey, "/key")
	if err != nil {
		return nil, err
	}
	typeId := entry.Id
	result := &v2model.CreateResult{Id: typeId, Key: typeKey}
	if dryRun {
		result.DryRun = true
		return result, nil
	}
	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{ContextId: typeId, IsArchived: true})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return nil, fmt.Errorf("archive type %s: %s", typeKey, resp.Error.Description)
	}
	return result, nil
}

// CreateProperty implements POST /v2/spaces/{space_id}/properties.
func (s *Service) CreateProperty(ctx context.Context, spaceId string, req v2model.CreatePropertyRequest, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, v2model.ValidationFailed("name is required",
			v2model.Issue{Path: "/name", Message: "a property needs a display name"})
	}
	if err := validateV2FieldLength("/name", req.Name, maxV2NameLength); err != nil {
		return nil, err
	}
	format, ok := anyblockjson.FormatByName(req.Format)
	if !ok {
		return nil, v2model.ValidationFailed("unknown property format",
			v2model.Issue{Path: "/format", Message: fmt.Sprintf("unknown format %q", req.Format), Hint: "allowed: text, number, select, multi_select, date, files, checkbox, url, email, phone, objects"})
	}
	isSelect := format == model.RelationFormat_status || format == model.RelationFormat_tag
	if len(req.Options) > 0 && !isSelect {
		return nil, v2model.ValidationFailed("options need a select format",
			v2model.Issue{Path: "/options", Message: fmt.Sprintf("options apply to select and multi_select properties, not %q", req.Format)})
	}
	// the bounds the property kind advertises (M6): option count, option
	// fields, and the key's length + pattern
	if len(req.Options) > maxV2PropertyOptions {
		return nil, v2model.ValidationFailed("too many options",
			v2model.Issue{Path: "/options",
				Message: fmt.Sprintf("%d options — the cap is %d (the advertised maxItems)", len(req.Options), maxV2PropertyOptions)})
	}
	for i, opt := range req.Options {
		optPath := fmt.Sprintf("/options/%d", i)
		if opt.Name == "" {
			return nil, v2model.ValidationFailed("an option needs a name",
				v2model.Issue{Path: optPath + "/name", Message: "name is required on every option"})
		}
		if err := validateV2FieldLength(optPath+"/name", opt.Name, maxV2NameLength); err != nil {
			return nil, err
		}
		if err := validateV2FieldLength(optPath+"/color", opt.Color, maxV2OptionColorLength); err != nil {
			return nil, err
		}
	}
	// The (a) identity layer (ADDRESSING §7.5): the caller's key never
	// becomes the stored relation key — the create mints a BSON internal key
	// and the caller's key lives in the apiObjectKey slug, snake-normalized
	// at mint. The union collision check ships WITH the mint (§7.6-3): the
	// proposed slug is tested against bundled keys, bundled-derived slugs,
	// live stored keys and live stored slugs — so a custom "dueDate2" and
	// "due_date2" collide by normalization, and a custom "Due Date" can
	// never shadow bundled due_date. Corpses vacate the namespace (§8-OQ2),
	// which is what makes delete-then-recreate mint cleanly instead of
	// dying on the surviving derived tree (§7.5-2).
	var slug string
	if req.Key != "" {
		if err := validateV2FieldLength("/key", req.Key, maxV2KeyLength); err != nil {
			return nil, err
		}
		if !v2PropertyKeyPattern.MatchString(req.Key) {
			return nil, v2model.ValidationFailed("invalid property key",
				v2model.Issue{Path: "/key",
					Message: fmt.Sprintf("key %q does not match the advertised pattern ^[a-zA-Z0-9_]+$", req.Key),
					Hint:    "use letters, digits and underscores — or omit key to derive one from the name"})
		}
		slug = bundle.ApiSlug(req.Key)
	} else {
		// derived from the display name, which no pattern ever checked —
		// sanitize to the advertised key grammar (empty = no derivable slug)
		slug = sanitizeApiSlug(bundle.ApiSlugFromName(req.Name))
	}
	if slug != "" {
		propEntries, err := s.liveProperties(spaceId)
		if err != nil {
			return nil, err
		}
		if holder, taken := s.propertySlugConflict(slug, propEntries); taken {
			path, hint := "/key", fmt.Sprintf("update it with PATCH /v2/spaces/%s/properties/%s, or pick a different key", spaceId, holder.Key)
			if req.Key == "" {
				path = "/name"
				hint = fmt.Sprintf("use the existing property %q, or pass an explicit different key", holder.Key)
			}
			return nil, v2model.ValidationFailed("property key already exists",
				v2model.Issue{Path: path,
					Message: fmt.Sprintf("key %q is taken by %s %q", slug, holder.Kind, holder.Name),
					Hint:    hint})
		}
	}

	result := &v2model.CreateResult{Key: slug}
	if dryRun {
		result.DryRun = true
		result.Created = &v2model.SideEffects{
			Properties: []v2model.PropertyRow{{Key: slug, Name: req.Name, Format: req.Format}},
		}
		for _, opt := range req.Options {
			result.Created.Options = append(result.Created.Options, v2model.CreatedOption{Property: slug, Name: opt.Name})
		}
		return result, nil
	}

	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(req.Name),
		bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(format)),
		bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}}
	if slug != "" {
		// the slug detail, never the relation key: objectcreator mints the
		// BSON internal key and respects a caller-set apiObjectKey
		details.Fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(slug)
	}
	resp := s.mw.ObjectCreateRelation(ctx, &pb.RpcObjectCreateRelationRequest{SpaceId: spaceId, Details: details})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		return nil, fmt.Errorf("create property in space %s: %s", spaceId, resp.Error.Description)
	}
	result.Id = resp.ObjectId
	// The MINT owns the final slug, not the proposal above. Its namespace and
	// v2's pre-check are deliberately not the same set — the mint counts
	// hidden holders, v2's request namespace excludes them (§7.5a /
	// propertyEntry.Hidden) — so a slug that was free here can be suffixed
	// there, and a walk that ran out gives up and stores nothing at all.
	// Returning the proposal handed the caller a 201 {"key": "manual_property"}
	// whose very next GET .../properties/manual_property 404'd. Read back what
	// was STORED.
	result.Key = storedApiKeyOf(resp.Details, result.Key)
	if result.Key == "" {
		result.Key = resp.Key // no derivable slug: the minted BSON is the only address
	}
	publicKey := result.Key

	for _, opt := range req.Options {
		optDetails := &types.Struct{Fields: map[string]*types.Value{
			// options bind to the STORED relation key (the minted BSON)
			bundle.RelationKeyRelationKey.String(): pbtypes.String(resp.Key),
			bundle.RelationKeyName.String():        pbtypes.String(opt.Name),
			bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
		}}
		if opt.Color != "" {
			optDetails.Fields[bundle.RelationKeyRelationOptionColor.String()] = pbtypes.String(opt.Color)
		}
		optResp := s.mw.ObjectCreateRelationOption(ctx, &pb.RpcObjectCreateRelationOptionRequest{SpaceId: spaceId, Details: optDetails})
		if optResp.Error != nil && optResp.Error.Code != pb.RpcObjectCreateRelationOptionResponseError_NULL {
			return nil, fmt.Errorf("create option %q of property %s: %s", opt.Name, publicKey, optResp.Error.Description)
		}
		if result.Created == nil {
			result.Created = &v2model.SideEffects{}
		}
		result.Created.Options = append(result.Created.Options, v2model.CreatedOption{Property: publicKey, Name: opt.Name})
	}
	return result, nil
}

// UpdateProperty implements PATCH /v2/spaces/{space_id}/properties/{key}.
func (s *Service) UpdateProperty(ctx context.Context, spaceId, propertyKey string, req v2model.UpdatePropertyRequest, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// live lookup, slug-aware (§7.5a-5) — a UI-deleted property must 404,
	// never steer the caller into patching a corpse (§2.3-6)
	entry, err := s.requireLiveProperty(spaceId, propertyKey)
	if err != nil {
		return nil, err
	}
	if bundled, err := bundle.PickRelation(domain.RelationKey(entry.Key)); err == nil && bundled.ReadOnly {
		return nil, v2model.ValidationFailed("property is read-only",
			v2model.Issue{Path: "/name", Message: fmt.Sprintf("bundled property %q cannot be updated", propertyKey)})
	}

	result := &v2model.CreateResult{Id: entry.Id, Key: propertyKey}
	if req.Name == nil {
		return nil, v2model.ValidationFailed("nothing to update",
			v2model.Issue{Message: "the patch accepts name"})
	}
	if err := validateV2FieldLength("/name", *req.Name, maxV2NameLength); err != nil {
		return nil, err
	}
	if dryRun {
		result.DryRun = true
		return result, nil
	}
	resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
		ContextId: entry.Id,
		Details:   []*model.Detail{{Key: bundle.RelationKeyName.String(), Value: pbtypes.String(*req.Name)}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return nil, fmt.Errorf("update property %s: %s", propertyKey, resp.Error.Description)
	}
	return result, nil
}

// DeleteProperty implements DELETE /v2/spaces/{space_id}/properties/{key}
// (archive).
func (s *Service) DeleteProperty(ctx context.Context, spaceId, propertyKey string, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// live lookup, slug-aware — deleting an already-archived or uninstalled
	// property is a 404, not a second archive of a corpse (§7.5-2)
	entry, err := s.requireLiveProperty(spaceId, propertyKey)
	if err != nil {
		return nil, err
	}
	result := &v2model.CreateResult{Id: entry.Id, Key: propertyKey}
	if dryRun {
		result.DryRun = true
		return result, nil
	}
	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{ContextId: entry.Id, IsArchived: true})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return nil, fmt.Errorf("archive property %s: %s", propertyKey, resp.Error.Description)
	}
	return result, nil
}

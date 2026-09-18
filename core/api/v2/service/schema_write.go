package v2service

// schema_write.go implements POST/PATCH/DELETE for types (kind:"object_type"
// AnyBlock documents —
// typeProperties creates missing properties, SPEC §2a) and properties
// ({key?, name, format, options?}).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/types"
	"golang.org/x/text/unicode/norm"
)

// detailKeyId mirrors the anyblockjson envelope lift: the minted id detail
// never travels into create RPC payloads.
const v2DetailKeyId = "id"

// The bounds the §5 discovery schemas advertise on the typed
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
	maxV2QuerySorts        = 10   // query sorts (maxItems)
	maxV2QueryViews        = 10   // query views (maxItems)
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
func (s *Service) CreateType(ctx context.Context, spaceId string, body []byte, dryRun, createMissingOptions bool) (result *v2model.CreateResult, err error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// envelope normalization like every other create: a pasted GetType read
	// carries etag (and possibly warnings) — without the strip, POST types
	// 400ed on the etag of its own read; the ?block= subtree marker is
	// refused by name instead of as an anonymous unknown field
	body, err = normalizeCreateBody(body)
	if err != nil {
		return nil, err
	}

	// the flat body (typeshortcut.go), discriminated the way POST /objects
	// discriminates its own: no formatVersion and no kind means the caller sent
	// the shape they would have guessed, and it is translated into the document
	// the rest of this function already handles.
	// the flat body's members sit at the root; the document's sit under
	// type_settings and properties. Every refusal below is addressed to the
	// body the caller sent, so a flat body's issues are rebased back at the
	// return boundary — whichever check produced them
	flat := false
	if fields, perr := parseEnvelope(body); perr == nil && !isTypeDocument(fields) {
		doc, derr := typeShortcutDocument(fields)
		if derr != nil {
			return nil, derr
		}
		if body, err = encodeEnvelope(doc); err != nil {
			return nil, err
		}
		flat = true
	}
	defer func() {
		if err != nil && flat {
			err = rebaseIssuePaths(err, flatTypeBodyPath)
		}
	}()
	kind := typeBodyKind(flat)
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
	}

	// the endpoint IS the kind: inject/enforce kind object_type and default
	// formatVersion so a bare {type_settings} document works
	if raw, ok := fields["kind"]; ok {
		var kind string
		if err := json.Unmarshal(raw, &kind); err != nil || kind != "object_type" {
			return nil, v2model.ValidationFailed("not a type document",
				v2model.Issue{Path: "/kind", Message: "POST types accepts kind \"object_type\" documents only"})
		}
	} else if fields["kind"], err = rawJSON("object_type"); err != nil {
		return nil, err
	}
	if _, ok := fields["formatVersion"]; !ok {
		if fields["formatVersion"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
			return nil, err
		}
	}
	if _, ok := fields["blocks"]; ok {
		// deferred: a type's dataview block on create (the editor generates
		// default views at first open — SPEC §2a); explicit beats silent loss
		return nil, v2model.ValidationFailed("type blocks are not supported on create",
			v2model.Issue{
				Path:    "/blocks",
				Message: "omit blocks — a type gets its views generated for it",
			}.Hintf("to shape them, create the type first, then edit its views through %s, addressing the type by its object id",
				v2model.NewRef(v2model.OpPatchObject, "space_id", spaceId)))
	}
	if body, err = encodeEnvelope(fields); err != nil {
		return nil, err
	}
	// the two member guesses a definition invites (F10), named before the
	// format prunes one of them — but after the format's own version gate,
	// which is the one verdict a definition repair must not pre-empt
	docErr := s.rejectInvalidDocument(body, kind)
	if v2Err := (*v2model.Error)(nil); errors.As(docErr, &v2Err) && v2Err.Code == v2model.CodeVersionUnsupported {
		return nil, docErr
	}
	if issues := typeDefinitionMemberIssues(rawTypeDefinitions(fields), "/type_settings/property_definitions", kind); len(issues) > 0 {
		return nil, v2model.ValidationFailed("the document failed AnyBlock validation", issues...)
	}
	if docErr != nil {
		return nil, docErr
	}

	var envelope docEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, v2model.ValidationFailed("decode document envelope: " + err.Error())
	}

	// The document's key never
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
		if holder, taken := s.typeSlugConflict(spaceId, slug, typeEntries); taken {
			if holder.Kind == "bundled type" {
				return nil, v2model.ValidationFailed("type key is reserved",
					v2model.Issue{Path: keyPath,
						Message: fmt.Sprintf("key %q is taken by bundled type %q — it already exists", slug, holder.Name)})
			}
			if holder.Kind == "types" {
				// several types answer to this slug already: an update by
				// that slug would be refused as ambiguous, so it is not offered
				return nil, v2model.ValidationFailed("type key already exists",
					v2model.Issue{Path: keyPath,
						Message: fmt.Sprintf("key %q is answered by several types in space %s: %s", slug, spaceId, holder.Name),
					}.WithHint(v2model.Plain("pick a different key")))
			}
			return nil, v2model.ValidationFailed("type key already exists",
				v2model.Issue{Path: keyPath,
					Message: fmt.Sprintf("key %q is taken by %s %q in space %s", slug, holder.Kind, holder.Name, spaceId),
				}.Hintf("update it with %s, or pick a different key", v2model.RefUpdateType(spaceId, holder.Key)))
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

	// consent is decided BEFORE Unmarshal mints a single property: the gate
	// used to fire after, so a refused request still left the properties it
	// had created behind (the M5 rule this function's own comment states).
	if raw := envelope.propertyDefinitions(); len(raw) > 0 {
		var declared []anyblockjson.TypeProperty
		if err := json.Unmarshal(raw, &declared); err == nil {
			if err := s.guardDeclaredOptions(spaceId, declared,
				"/type_settings/property_definitions", createMissingOptions); err != nil {
				return nil, err
			}
		}
	}

	// Unmarshal rebuilds the four recommended-relation lists from
	// typeProperties, creating missing properties through the resolver
	resolvers := s.newCreatingResolvers(ctx, spaceId, dryRun, createMissingOptions)
	_, snapshot, err := anyblockjson.Unmarshal(body, resolvers.Options())
	if err != nil {
		return nil, mapUnmarshalError(body, err, kind)
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve type properties: %w", err)
	}
	// the declared select vocabulary, before the dry-run return: a dry run's
	// job is to preview what the real run does, and options it never mentions
	// are options a caller does not know they are about to create
	if raw := envelope.propertyDefinitions(); len(raw) > 0 {
		var declared []anyblockjson.TypeProperty
		if err := json.Unmarshal(raw, &declared); err == nil {
			if err := s.applyDeclaredOptions(declared, resolvers, "/type_settings/property_definitions"); err != nil {
				return nil, err
			}
		}
	}

	result = &v2model.CreateResult{Key: slug, Created: resolvers.created()}
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
// mints a BSON internal key), identity-bearing details
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
// at the wiring: a typeProperties
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
		if term == "" {
			// the served property_definitions item names its property by
			// `name` alone, and the codec resolves such an entry through the
			// NFC name (identityForResolution) — which resolvePropertyInput's
			// display-name step answers with the existing property. This guard
			// used to skip name-only entries, so the codec then matched
			// {"name":"Condition","format":"select"} to the stored text
			// property and kept it text, silently. Same identity, same guard.
			term = norm.NFC.String(tp.Name)
		}
		if term == "" || tp.Format == "" {
			continue
		}
		// the value is unused — the comparison below is on format NAMES — but
		// the lookup still rejects a spelling no format answers to
		if _, known := anyblockjson.FormatByName(tp.Format); !known {
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
		// compared through the SERVED vocabulary, not the raw enum. FormatName
		// folds longtext and shorttext to one word, "text", while FormatByName
		// answers only longtext — so a shorttext property (the object title,
		// 16 other bundled keys, and every property a markdown import minted
		// before it defaulted to longtext) could never satisfy any format
		// string, and the refusal read `declares format "text" but the
		// existing property has format "text"`. Folding both sides keeps the
		// guard agreeing with the read by construction.
		if anyblockjson.FormatName(entry.Format) != tp.Format {
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
	"layout":           "recommendedLayout",
	"plural_name":      "pluralName",
	"default_view":     "defaultViewType",
	"default_template": "defaultTemplateId",
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
	DefaultView         json.RawMessage              `json:"default_view"`
	DefaultTemplate     json.RawMessage              `json:"default_template"`
	PropertyDefinitions *[]anyblockjson.TypeProperty `json:"property_definitions"`
}

// patchableSettings is the ONE place the patchable §2a members are paired with
// their raw values. It used to be restated twice inline in the apply loop,
// which is how default_view and default_template came to be declared on the
// struct and silently ignored by the loop.
func (p v2TypeSettingsPatch) patchableSettings() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"layout":           p.Layout,
		"plural_name":      p.PluralName,
		"default_view":     p.DefaultView,
		"default_template": p.DefaultTemplate,
	}
}

// propertyDefinitions is the patch's §2a definition array, nil-safe.
func (p v2TypePatch) propertyDefinitions() *[]anyblockjson.TypeProperty {
	if p.TypeSettings == nil {
		return nil
	}
	return p.TypeSettings.PropertyDefinitions
}

// UpdateType implements PATCH /v2/spaces/{space_id}/types/{type}.
func (s *Service) UpdateType(ctx context.Context, spaceId, typeKey, ifMatch string, body []byte, dryRun, createMissingOptions bool) (result *v2model.CreateResult, err error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	// live lookup, slug-aware — a UI-deleted type must 404, never steer the
	// caller into patching a corpse (§2.3-6)
	entry, err := s.requireLiveType(spaceId, typeKey, "/key", errKeysFor(ctx))
	if err != nil {
		return nil, err
	}
	typeId := entry.Id

	// C7 concurrency precondition, advisory like the object channel's: absent
	// If-Match is last-write-wins, a stale one is a 409. It matters more here
	// than it looks — the ops path is a server-side read-modify-write of all
	// four lists across several RPCs, so a featured-list change landing in
	// that window is silently reverted, and a reverted featured list cascades
	// to every object of the type.
	if ifMatch != "" {
		read, rerr := s.reader.ReadObject(ctx, spaceId, typeId)
		if rerr != nil {
			return nil, fmt.Errorf("read type %s: %w", typeId, rerr)
		}
		if !EtagMatches(ifMatch, read.Heads) {
			return nil, v2model.EtagMismatch(ComputeEtag(read.Heads))
		}
	}

	// the op channel (typeops.go), discriminated FIRST. An ops envelope
	// carries none of the members isTypeDocument answers on, so left to fall
	// through it would be read as a flat body and refused as an unknown key —
	// and a body carrying a bare `op` routes here too, so the refusal names
	// the missing wrapper instead of calling `op` an unknown field.
	if fields, perr := parseEnvelope(body); perr == nil {
		_, wrapped := fields["ops"]
		_, unwrapped := fields["op"]
		if wrapped || unwrapped {
			return s.updateTypeOps(ctx, spaceId, entry, typeKey, body, dryRun, createMissingOptions)
		}
	}

	// the same flat body the create verb takes: one shape for the resource,
	// rather than a create body and an update body that reject each other
	flat := false
	if fields, perr := parseEnvelope(body); perr == nil && !isTypeDocument(fields) {
		if _, nested := fields["type_settings"]; !nested {
			if _, valued := fields["properties"]; !valued {
				translated, terr := typeShortcutPatch(fields)
				if terr != nil {
					return nil, terr
				}
				if body, err = encodeEnvelope(translated); err != nil {
					return nil, err
				}
				flat = true
			}
		}
	}
	defer func() {
		if err != nil && flat {
			err = rebaseIssuePaths(err, flatTypeBodyPath)
		}
	}()
	// the definition member guesses (F10): the patch decoder below would
	// silently drop a `type` member (TypeProperty's own decoder ignores
	// unknown members), and the property would be minted as text
	if fields, perr := parseEnvelope(body); perr == nil {
		if issues := typeDefinitionMemberIssues(rawTypeDefinitions(fields), "/type_settings/property_definitions", typeBodyKind(flat)); len(issues) > 0 {
			return nil, v2model.ValidationFailed("invalid type patch", issues...)
		}
	}

	var patch v2TypePatch
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return nil, v2model.ValidationFailed("invalid type patch",
			v2model.Issue{Message: err.Error(), Hint: "the patch accepts properties and type_settings"})
	}

	var detailUpdates []*model.Detail
	var detached []v2model.PropertyRow
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
					Hint: "properties takes name and description; the layout is type_settings.layout and the icon is the typed envelope icon"})
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
		settings := patch.TypeSettings.patchableSettings()
		for _, member := range sortedKeys(settings) {
			raw := settings[member]
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
	// the dataview half of a list replacement (round-two eval F2): the op
	// channel keeps the type's views in step with its lists — a column for
	// every property added, none for one removed — and a replaced list must
	// do the same, or the type is half-updated behind a 200
	var addedIds, removedKeys []string
	if defs := patch.propertyDefinitions(); defs != nil {
		// the echo baseline (§8.41): entries this type ALREADY references
		// resolve as identities even when their relation is removed — the
		// GET/PATCH loop must not force-delete a reference the read served
		resolvers.echoPropertyIds = s.recommendedRelationIds(spaceId, typeId)
		detachedBefore := s.recommendedRelationIds(spaceId, typeId)
		// the corpses the type still lists, whose slugs its read served: a
		// definition echoing one resolves to it (no namesake is minted)
		corpses := s.referencedCorpses(spaceId, detachedBefore)
		resolvers.rememberCorpses(corpses)
		// the SPEC §2a format check, before the resolver can create
		if err := s.validateTypePropertyFormats(spaceId, *defs); err != nil {
			return nil, err
		}
		// UpdateType validates no document, so the rules CreateType gets from
		// the format layer have to be stated here: options belong to a select
		// property, and creating them needs consent. Both run before
		// BuildRecommendedLists, which is what mints.
		if err := s.guardDeclaredOptions(spaceId, *defs,
			"/type_settings/property_definitions", createMissingOptions); err != nil {
			return nil, err
		}
		lists, err := anyblockjson.BuildRecommendedLists(*defs, resolvers.Options())
		if err != nil {
			return nil, fmt.Errorf("build recommended lists: %w", err)
		}
		if err := resolvers.err(); err != nil {
			return nil, fmt.Errorf("resolve type properties: %w", err)
		}
		kept := map[string]bool{}
		for _, list := range lists {
			detailUpdates = append(detailUpdates, &model.Detail{Key: list.DetailKey, Value: pbtypes.StringList(list.Ids)})
			for _, id := range list.Ids {
				kept[id] = true
				if !detachedBefore[id] {
					addedIds = append(addedIds, id)
				}
			}
		}
		detached = s.detachedProperties(spaceId, detachedBefore, kept)
		// the stored keys the prune works in, read BEFORE the write while
		// the detached entries are still what the type listed — the live
		// ones and the already-removed ones alike, since a removed
		// property's column is exactly the one a replacement should drop
		entries, eerr := s.liveProperties(spaceId)
		if eerr != nil {
			return nil, eerr
		}
		for _, e := range entries {
			if detachedBefore[e.Id] && !kept[e.Id] {
				removedKeys = append(removedKeys, e.Key)
			}
		}
		for _, e := range corpses {
			if !kept[e.Id] {
				removedKeys = append(removedKeys, e.Key)
			}
		}
		// the declared select vocabulary, which nothing used to apply
		if err := s.applyDeclaredOptions(*defs, resolvers, "/type_settings/property_definitions"); err != nil {
			return nil, err
		}
	}

	result = &v2model.CreateResult{Id: typeId, Key: typeKey, Created: resolvers.created()}
	// a replaced list detaches whatever it omitted. Report it in BOTH channels:
	// `removed` so a client can act on it, and a warning so a human reading the
	// response sees it without knowing to look for a new field. A dry run says
	// the same thing, because a dry run that hides the destructive half is
	// worse than none.
	if len(detached) > 0 {
		result.Removed = &v2model.SideEffects{Properties: detached}
		names := make([]string, 0, len(detached))
		for _, row := range detached {
			names = append(names, row.Key)
		}
		warning := v2model.Issue{
			Path: "/type_settings/property_definitions",
			Message: fmt.Sprintf("property_definitions replaces the type's whole field list: %d no longer listed (%s)",
				len(detached), strings.Join(names, ", ")),
		}.Hintf("send the complete list to keep a field, omit property_definitions to leave the list untouched, or change one field at a time with the add_property, remove_property and move_property ops (%s)",
			v2model.RefGetOpSchema("add_property"))
		result.Warnings = append(result.Warnings, warning)
	}
	// the views' half of the report, computed from the live type before the
	// dry-run return — as the op channel does — so a rehearsal names the
	// columns a real run would drop
	if len(removedKeys) > 0 {
		plan, perr := s.typeDataviewPrunePlan(ctx, spaceId, typeId, removedKeys)
		if perr != nil {
			return nil, perr
		}
		result.Warnings = append(result.Warnings, typePruneWarnings(plan, "/type_settings/property_definitions", s.servedKeySpeller(spaceId))...)
	}
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
	// the views, in the op channel's order: the lists are written, then the
	// columns of removed properties go, then the added properties gain
	// their columns
	if len(removedKeys) > 0 {
		if err := s.pruneTypeDataviewColumns(ctx, spaceId, typeId, removedKeys); err != nil {
			return nil, err
		}
	}
	if len(addedIds) > 0 {
		links, err := s.relationLinksOf(spaceId, addedIds)
		if err != nil {
			return nil, err
		}
		if _, err := s.addTypeDataviewColumns(ctx, spaceId, typeId, links); err != nil {
			return nil, err
		}
	}
	if read, err := s.reader.ReadObject(ctx, spaceId, typeId); err == nil {
		result.Etag = ComputeEtag(read.Heads)
	}
	return result, nil
}

// relationLinksOf builds the dataview links for relation object ids, in the
// order given, from the space's live properties — read after the write, so a
// property this same request created is among them.
func (s *Service) relationLinksOf(spaceId string, ids []string) ([]*model.RelationLink, error) {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil, fmt.Errorf("reconcile type columns: %w", err)
	}
	byId := make(map[string]propertyEntry, len(entries))
	for _, e := range entries {
		byId[e.Id] = e
	}
	links := make([]*model.RelationLink, 0, len(ids))
	for _, id := range ids {
		if e, ok := byId[id]; ok {
			links = append(links, &model.RelationLink{Key: e.Key, Format: e.Format})
		}
	}
	return links, nil
}

// iconPatchDetails turns §2b's typed `icon` into the stored detail keys it
// implies, by handing a minimal type document to the format's own importer
// and reading back what it set. The variant rules — which of iconEmoji /
// iconImage / iconName / iconOption a format selects, and what an
// out-of-vocabulary variant earns — stay stated once, in the format.
func iconPatchDetails(icon json.RawMessage) ([]*model.Detail, error) {
	doc, err := encodeEnvelope(map[string]json.RawMessage{
		"formatVersion": json.RawMessage(`"` + anyblockjson.FormatVersion + `"`),
		"kind":          json.RawMessage(`"object_type"`),
		"icon":          icon,
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
	if key == "defaultViewType" {
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return nil, v2model.ValidationFailed("invalid default_view",
				v2model.Issue{Path: path, Message: "expected a view type name"})
		}
		// matched case-insensitively against heart's own enum rather than a
		// fourth hand-written copy of the vocabulary: the served schema, the
		// view ops and the format each already spell this list
		for enumName, value := range model.BlockContentDataviewViewType_value {
			if strings.EqualFold(enumName, name) {
				return pbtypes.Int64(int64(value)), nil
			}
		}
		return nil, v2model.ValidationFailed("unknown view type",
			v2model.Issue{Path: path, Message: fmt.Sprintf("unknown view type %q", name),
				Hint: "one of: " + strings.Join(anyblockjson.ViewTypeNames(), ", ")})
	}
	if key == "defaultTemplateId" {
		var id string
		if err := json.Unmarshal(raw, &id); err != nil {
			return nil, v2model.ValidationFailed("invalid default_template",
				v2model.Issue{Path: path, Message: "expected a template object id"})
		}
		// stored as a LIST even though the member is a single id — the client
		// reads entry 0. Writing a bare string here makes the type unreadable
		// to every client that expects the list shape.
		if id == "" {
			return pbtypes.StringList(nil), nil
		}
		return pbtypes.StringList([]string{id}), nil
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
	entry, err := s.requireLiveType(spaceId, typeKey, "/key", errKeysFor(ctx))
	if err != nil {
		return nil, err
	}
	typeId := entry.Id
	result := &v2model.CreateResult{Id: typeId, Key: typeKey}
	// the objects of the type survive it, keeping it under the spelling
	// they were served (round-four eval R4-1): say so, on the real run and
	// the dry run alike, as delete_property does
	result.Warnings = append(result.Warnings, s.typeDeleteWarnings(spaceId, entry)...)
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

// typeDeleteWarnings names the objects a type delete leaves behind: they
// keep the type, served under its slug, and nothing new is created in it.
// A store error makes no warning.
func (s *Service) typeDeleteWarnings(spaceId string, entry typeEntry) []v2model.Issue {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{RelationKey: bundle.RelationKeyType, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.String(entry.Id)},
			{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
		},
		Limit: propertyHolderProbeLimit,
	})
	if err != nil || len(records) == 0 {
		return nil
	}
	count := fmt.Sprintf("%d objects are", len(records))
	if len(records) == 1 {
		count = "1 object is"
	} else if len(records) >= propertyHolderProbeLimit {
		count = fmt.Sprintf("at least %d objects are", propertyHolderProbeLimit)
	}
	// the spelling reads serve now, and the one they will serve after: the
	// slug, unless a removed type already answers to it — then both read
	// under their stored keys (the twin rule)
	served := s.apiKeys(spaceId, storeresolver.New(s.store.SpaceIndex(spaceId))).TypeSlug(entry.Key)
	after := fmt.Sprintf("reads still spell it %q", served)
	if entry.Slug != "" && served == entry.Slug {
		if removed, rerr := s.removedTypes(spaceId); rerr == nil {
			for _, e := range removed {
				if e.Slug == entry.Slug && e.Key != entry.Key {
					after = fmt.Sprintf("another removed type used %q before, so after this delete the objects of both read under their stored keys (%q here)", entry.Slug, entry.Key)
					break
				}
			}
		}
	}
	issue := v2model.Issue{
		Path:    "key",
		Message: fmt.Sprintf("%s of type %q; they keep it, and %s, but nothing new is created in it and it is not listed or filterable, until this same type is restored in the app", count, served, after),
	}.WithHint(v2model.Plain("a dry run reports this without deleting; to keep the type usable, keep it"))
	return []v2model.Issue{issue}
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
	// The caller's key never
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
			path, hint := "/key", v2model.Hintf("update it with %s, or pick a different key", v2model.RefUpdateProperty(spaceId, holder.Key))
			if req.Key == "" {
				path = "/name"
				hint = v2model.Plain(fmt.Sprintf("use the existing property %q, or pass an explicit different key", holder.Key))
			}
			if holder.Kind == "properties" {
				// several properties answer to this slug already: neither an
				// update by that slug nor "use the existing property" can be
				// followed — both would be refused as ambiguous — so the only
				// repair is a key of the caller's own, whichever slot they wrote
				hint = v2model.Plain(fmt.Sprintf("several properties answer to %q (%s) — pass an explicit different key", slug, holder.Name))
			}
			if _, bundled := bundle.PickRelation(domain.RelationKey(holder.Key)); bundled == nil || holder.Kind == "bundled property" {
				// a built-in property, installed or not: an update by its key
				// is refused (read-only) or 404s (not installed), so neither
				// repair above can be followed — the key is simply reserved
				hint = v2model.Plain(fmt.Sprintf("key %q is reserved by the built-in property %q — pick a different key", holder.Key, holder.Name))
			}
			return nil, v2model.ValidationFailed("property key already exists",
				v2model.Issue{Path: path,
					Message: fmt.Sprintf("key %q is taken by %s %q", slug, holder.Kind, holder.Name),
				}.WithHint(hint))
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
	entry, err := s.requireLiveProperty(spaceId, propertyKey, errKeysFor(ctx))
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
	entry, err := s.requireLiveProperty(spaceId, propertyKey, errKeysFor(ctx))
	if err != nil {
		return nil, err
	}
	result := &v2model.CreateResult{Id: entry.Id, Key: propertyKey}
	// the delete is destructive in two places the caller cannot see from
	// here: objects that hold a value of the property, and types that list
	// it. Say so, on the real run and the dry run alike (round-two eval F1:
	// a silent 200 here is how a caller destroyed five objects' data).
	result.Warnings = append(result.Warnings, s.propertyDeleteWarnings(spaceId, entry, s.servedKeySpeller(spaceId)(entry.Key))...)
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

// propertyHolderProbeLimit bounds the object count a delete reports: the
// count exists to make the loss visible, not to be exact past this.
const propertyHolderProbeLimit = 1000

// propertyDeleteWarnings names what a property delete leaves behind: the
// objects holding a value of it and the types listing it. servedKey is the
// spelling reads will serve for the property, not the one the caller
// deleted it by — a delete by display name or stored key still promises the
// slug the values will show under. Store errors make no warning — the
// delete still stands, and a warning the store could not substantiate is
// worse than none.
func (s *Service) propertyDeleteWarnings(spaceId string, entry propertyEntry, servedKey string) []v2model.Issue {
	var issues []v2model.Issue
	// presence, not emptiness: a stored 0 or false is a value the caller
	// set; and archived objects hold values too (a restore brings them
	// back), so the injected isArchived default is suppressed — deleted
	// objects stay out
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{RelationKey: domain.RelationKey(entry.Key), Condition: model.BlockContentDataviewFilter_Exists},
			{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
		},
		Limit: propertyHolderProbeLimit,
	})
	if err == nil && len(records) > 0 {
		count := fmt.Sprintf("%d objects hold", len(records))
		if len(records) == 1 {
			count = "1 object holds"
		} else if len(records) >= propertyHolderProbeLimit {
			count = fmt.Sprintf("at least %d objects hold", propertyHolderProbeLimit)
		}
		issues = append(issues, v2model.Issue{
			Path:    "key",
			Message: fmt.Sprintf("%s a value of %q; those values stay readable and editable in place under that key, but set_properties gives no other object one, until this same property is restored in the app (a new property with the same name is a different property)", count, servedKey),
		}.WithHint(v2model.Plain("a dry run reports this without deleting")))
	}
	types, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: liveTypeFilters()})
	if err == nil {
		var listing []string
		for _, record := range types {
			for _, listKey := range typeRecommendedListKeys {
				if slices.Contains(record.Details.GetStringList(listKey), entry.Id) {
					name := record.Details.GetString(bundle.RelationKeyName)
					if name == "" {
						name = record.Details.GetString(bundle.RelationKeyUniqueKey)
					}
					listing = append(listing, name)
					break
				}
			}
		}
		if len(listing) > 0 {
			sort.Strings(listing)
			noun := "types list"
			if len(listing) == 1 {
				noun = "type lists"
			}
			issues = append(issues, v2model.Issue{
				Path:    "key",
				Message: fmt.Sprintf("%d %s %q (%s); their property lists and views keep the entry, spelled %q, until it is taken off them", len(listing), noun, servedKey, strings.Join(listing, ", "), servedKey),
			}.Hintf("take it off a type with the remove_property op (%s)", v2model.RefGetOpSchema("remove_property")))
		}
	}
	return issues
}

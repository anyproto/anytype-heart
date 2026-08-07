package v2service

// schema_write.go implements the Phase-2 schema write surface (APIV2.md
// §2): POST/PATCH/DELETE for types (kind:"objectType" AnyBlock documents —
// typeProperties creates missing properties, SPEC §2a) and properties
// ({key?, name, format, options?}).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

// CreateType implements POST /v2/spaces/{spaceId}/types: a kind:"objectType"
// AnyBlock document; typeProperties creates missing properties atomically
// with the type (SPEC §2a create-missing).
func (s *V2Service) CreateType(ctx context.Context, spaceId string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
	}

	// the endpoint IS the kind: inject/enforce kind objectType and default
	// the version so a bare {key, typeProperties} document works
	if raw, ok := fields["kind"]; ok {
		var kind string
		if err := json.Unmarshal(raw, &kind); err != nil || kind != "objectType" {
			return nil, v2model.ValidationFailed("not a type document",
				v2model.Issue{Path: "/kind", Message: "POST types accepts kind \"objectType\" documents only"})
		}
	} else if fields["kind"], err = rawJSON("objectType"); err != nil {
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
	if envelope.Key != "" {
		if bundle.HasObjectTypeByKey(domain.TypeKey(envelope.Key)) {
			return nil, v2model.ValidationFailed("type key is reserved",
				v2model.Issue{Path: "/key", Message: fmt.Sprintf("%q is a bundled type — it already exists", envelope.Key)})
		}
		if _, exists := s.typeIdInSpace(spaceId, envelope.Key); exists {
			return nil, v2model.ValidationFailed("type key already exists",
				v2model.Issue{Path: "/key", Message: fmt.Sprintf("type %q already exists in space %q", envelope.Key, spaceId), Hint: fmt.Sprintf("update it with PATCH /v2/spaces/%s/types/%s", spaceId, envelope.Key)})
		}
	}

	// Unmarshal rebuilds the four recommended-relation lists from
	// typeProperties, creating missing properties through the resolver
	resolvers := newCreatingResolvers(ctx, s.mw, spaceId, s.store.SpaceIndex(spaceId), dryRun)
	_, snapshot, err := anyblockjson.Unmarshal(body, resolvers.Options())
	if err != nil {
		return nil, mapUnmarshalError(body, err)
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve type properties: %w", err)
	}

	result := &v2model.CreateResult{Key: envelope.Key, Created: resolvers.created()}
	if dryRun {
		result.DryRun = true
		return result, nil
	}

	details, err := typeDetailsFromSnapshot(snapshot, envelope.Key)
	if err != nil {
		return nil, err
	}
	if len(envelope.TypeProperties) > 0 {
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

// typeDetailsFromSnapshot converts the unmarshaled type snapshot into the
// ObjectCreateObjectType details: the minted id dropped, the key pinned as
// the unique key, and recommendedLayout accepting the layout NAME (the §2a
// worked example's form) as well as the stored number.
func typeDetailsFromSnapshot(snapshot *model.SmartBlockSnapshotBase, key string) (*types.Struct, error) {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	if snapshot.Details != nil {
		for k, v := range snapshot.Details.Fields {
			if k == v2DetailKeyId {
				continue
			}
			details.Fields[k] = v
		}
	}
	if key != "" {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key)
		if err != nil {
			return nil, v2model.ValidationFailed("invalid type key",
				v2model.Issue{Path: "/key", Message: err.Error()})
		}
		details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())
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

// updatableTypeDetailKeys is the explicit PATCH surface for a type's own
// properties; anything else is rejected (never silently dropped).
var updatableTypeDetailKeys = map[string]bool{
	"name": true, "description": true, "iconEmoji": true, "recommendedLayout": true,
}

// v2TypePatch is the PATCH types/{type} body: partial type-document
// semantics — properties updates the type's own details, typeProperties
// (when present) rebuilds the recommended lists, creating missing properties
// (SPEC §2a).
type v2TypePatch struct {
	Properties     map[string]json.RawMessage   `json:"properties"`
	TypeProperties *[]anyblockjson.TypeProperty `json:"typeProperties"`
}

// UpdateType implements PATCH /v2/spaces/{spaceId}/types/{type}.
func (s *V2Service) UpdateType(ctx context.Context, spaceId, typeKey string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	typeId, ok := s.typeIdInSpace(spaceId, typeKey)
	if !ok {
		return nil, s.typeNotFoundError(spaceId, typeKey)
	}

	var patch v2TypePatch
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		return nil, v2model.ValidationFailed("invalid type patch",
			v2model.Issue{Message: err.Error(), Hint: "the patch accepts properties and typeProperties"})
	}

	var detailUpdates []*model.Detail
	for _, key := range sortedKeys(patch.Properties) {
		if !updatableTypeDetailKeys[key] {
			return nil, v2model.ValidationFailed("property not updatable on a type",
				v2model.Issue{Path: "/properties/" + key, Message: fmt.Sprintf("cannot update %q", key), Hint: "updatable: name, description, iconEmoji, recommendedLayout"})
		}
		value, err := typeDetailValue(key, patch.Properties[key])
		if err != nil {
			return nil, err
		}
		detailUpdates = append(detailUpdates, &model.Detail{Key: key, Value: value})
	}

	resolvers := newCreatingResolvers(ctx, s.mw, spaceId, s.store.SpaceIndex(spaceId), dryRun)
	if patch.TypeProperties != nil {
		lists := anyblockjson.BuildRecommendedLists(*patch.TypeProperties, resolvers)
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

// typeDetailValue decodes one PATCH type property value.
func typeDetailValue(key string, raw json.RawMessage) (*types.Value, error) {
	if key == "recommendedLayout" {
		var name string
		if err := json.Unmarshal(raw, &name); err == nil {
			layout, ok := model.ObjectTypeLayout_value[name]
			if !ok {
				return nil, v2model.ValidationFailed("unknown layout name",
					v2model.Issue{Path: "/properties/recommendedLayout", Message: fmt.Sprintf("unknown layout %q", name), Hint: "common layouts: basic, todo, note, profile"})
			}
			return pbtypes.Int64(int64(layout)), nil
		}
		var number int64
		if err := json.Unmarshal(raw, &number); err == nil {
			return pbtypes.Int64(number), nil
		}
		return nil, v2model.ValidationFailed("invalid recommendedLayout",
			v2model.Issue{Path: "/properties/recommendedLayout", Message: "expected a layout name or number"})
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return nil, v2model.ValidationFailed("invalid property value",
			v2model.Issue{Path: "/properties/" + key, Message: "expected a string"})
	}
	return pbtypes.String(str), nil
}

// DeleteType implements DELETE /v2/spaces/{spaceId}/types/{type} (archive —
// v1 parity; hard delete is deferred with ?permanent).
func (s *V2Service) DeleteType(ctx context.Context, spaceId, typeKey string, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	typeId, ok := s.typeIdInSpace(spaceId, typeKey)
	if !ok {
		return nil, s.typeNotFoundError(spaceId, typeKey)
	}
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

// CreateProperty implements POST /v2/spaces/{spaceId}/properties.
func (s *V2Service) CreateProperty(ctx context.Context, spaceId string, req v2model.CreatePropertyRequest, dryRun bool) (*v2model.CreateResult, error) {
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
			v2model.Issue{Path: "/format", Message: fmt.Sprintf("unknown format %q", req.Format), Hint: "allowed: text, number, select, multiSelect, date, files, checkbox, url, email, phone, objects"})
	}
	isSelect := format == model.RelationFormat_status || format == model.RelationFormat_tag
	if len(req.Options) > 0 && !isSelect {
		return nil, v2model.ValidationFailed("options need a select format",
			v2model.Issue{Path: "/options", Message: fmt.Sprintf("options apply to select and multiSelect properties, not %q", req.Format)})
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
		if s.propertyKeyExists(spaceId, req.Key) {
			return nil, v2model.ValidationFailed("property key already exists",
				v2model.Issue{Path: "/key", Message: fmt.Sprintf("property %q already exists", req.Key), Hint: fmt.Sprintf("update it with PATCH /v2/spaces/%s/properties/%s", spaceId, req.Key)})
		}
	}

	result := &v2model.CreateResult{Key: req.Key}
	if dryRun {
		result.DryRun = true
		result.Created = &v2model.SideEffects{
			Properties: []v2model.PropertyRow{{Key: req.Key, Name: req.Name, Format: req.Format}},
		}
		for _, opt := range req.Options {
			result.Created.Options = append(result.Created.Options, v2model.CreatedOption{Property: req.Key, Name: opt.Name})
		}
		return result, nil
	}

	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(req.Name),
		bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(format)),
		bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}}
	if req.Key != "" {
		details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(req.Key)
	}
	resp := s.mw.ObjectCreateRelation(ctx, &pb.RpcObjectCreateRelationRequest{SpaceId: spaceId, Details: details})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		return nil, fmt.Errorf("create property in space %s: %s", spaceId, resp.Error.Description)
	}
	result.Id = resp.ObjectId
	result.Key = resp.Key

	for _, opt := range req.Options {
		optDetails := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String(): pbtypes.String(resp.Key),
			bundle.RelationKeyName.String():        pbtypes.String(opt.Name),
			bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
		}}
		if opt.Color != "" {
			optDetails.Fields[bundle.RelationKeyRelationOptionColor.String()] = pbtypes.String(opt.Color)
		}
		optResp := s.mw.ObjectCreateRelationOption(ctx, &pb.RpcObjectCreateRelationOptionRequest{SpaceId: spaceId, Details: optDetails})
		if optResp.Error != nil && optResp.Error.Code != pb.RpcObjectCreateRelationOptionResponseError_NULL {
			return nil, fmt.Errorf("create option %q of property %s: %s", opt.Name, resp.Key, optResp.Error.Description)
		}
		if result.Created == nil {
			result.Created = &v2model.SideEffects{}
		}
		result.Created.Options = append(result.Created.Options, v2model.CreatedOption{Property: resp.Key, Name: opt.Name})
	}
	return result, nil
}

// UpdateProperty implements PATCH /v2/spaces/{spaceId}/properties/{key}.
func (s *V2Service) UpdateProperty(ctx context.Context, spaceId, propertyKey string, req v2model.UpdatePropertyRequest, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	rel, err := s.store.SpaceIndex(spaceId).GetRelationByKey(propertyKey)
	if err != nil || rel == nil {
		return nil, s.propertyNotFoundError(spaceId, propertyKey)
	}
	if bundled, err := bundle.PickRelation(domain.RelationKey(propertyKey)); err == nil && bundled.ReadOnly {
		return nil, v2model.ValidationFailed("property is read-only",
			v2model.Issue{Path: "/name", Message: fmt.Sprintf("bundled property %q cannot be updated", propertyKey)})
	}

	result := &v2model.CreateResult{Id: rel.Id, Key: propertyKey}
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
		ContextId: rel.Id,
		Details:   []*model.Detail{{Key: bundle.RelationKeyName.String(), Value: pbtypes.String(*req.Name)}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return nil, fmt.Errorf("update property %s: %s", propertyKey, resp.Error.Description)
	}
	return result, nil
}

// DeleteProperty implements DELETE /v2/spaces/{spaceId}/properties/{key}
// (archive).
func (s *V2Service) DeleteProperty(ctx context.Context, spaceId, propertyKey string, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	rel, err := s.store.SpaceIndex(spaceId).GetRelationByKey(propertyKey)
	if err != nil || rel == nil {
		return nil, s.propertyNotFoundError(spaceId, propertyKey)
	}
	result := &v2model.CreateResult{Id: rel.Id, Key: propertyKey}
	if dryRun {
		result.DryRun = true
		return result, nil
	}
	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{ContextId: rel.Id, IsArchived: true})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return nil, fmt.Errorf("archive property %s: %s", propertyKey, resp.Error.Description)
	}
	return result, nil
}

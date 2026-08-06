package service

// v2_create.go implements the Phase-2 object create surface (APIV2.md §2):
// POST /v2/spaces/{spaceId}/objects (full AnyBlock document or the
// {type, name, properties, markdown} shortcut — discriminated per §8/R7 on
// the presence of version/blocks) and POST .../templates. The create path is
// snapshot-based: anyblockjson.Unmarshal → apicore.ObjectCreator (one change
// set), with create-missing resolvers (v2_resolver.go) and the referential
// validation layer (v2_refs.go) in front.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/types"
)

// docEnvelope is the light envelope decode used for referential validation
// before Unmarshal runs (no side effects yet at that point).
type docEnvelope struct {
	Kind           string                     `json:"kind"`
	Type           string                     `json:"type"`
	TemplateFor    string                     `json:"templateFor"`
	Key            string                     `json:"key"`
	Properties     map[string]json.RawMessage `json:"properties"`
	TypeProperties json.RawMessage            `json:"typeProperties"`
	Items          []string                   `json:"items"`
}

// v2ObjectShortcut is the R7 shortcut body: {type, name, properties,
// markdown}. Any other top-level key means the caller meant a full document
// and forgot version/blocks — rejected with steering.
type v2ObjectShortcut struct {
	Type       string                     `json:"type"`
	Name       string                     `json:"name"`
	Properties map[string]json.RawMessage `json:"properties"`
	Markdown   string                     `json:"markdown"`
}

var shortcutKeys = map[string]bool{"type": true, "name": true, "properties": true, "markdown": true}

// docCreateOptions parameterizes the shared document create path.
type docCreateOptions struct {
	dryRun          bool
	requireTemplate bool // POST /templates: templateFor is mandatory
}

// CreateObject implements POST /v2/spaces/{spaceId}/objects.
func (s *V2Service) CreateObject(ctx context.Context, spaceId string, body []byte, dryRun bool) (*apimodel.V2CreateResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, apimodel.V2ValidationFailed("request body is not a JSON object",
			apimodel.V2Issue{Message: err.Error()})
	}

	// §8/R7 discriminator: presence of version or blocks ⇒ full document
	_, hasVersion := fields["version"]
	_, hasBlocks := fields["blocks"]
	if hasVersion || hasBlocks {
		return s.createFromDocument(ctx, spaceId, body, docCreateOptions{dryRun: dryRun})
	}
	return s.createFromShortcut(ctx, spaceId, fields, dryRun)
}

// CreateTemplate implements POST /v2/spaces/{spaceId}/templates: an AnyBlock
// document with templateFor, routed through the generic object-create path
// (no create-from-body template RPC exists — APIV2.md Phase 2).
func (s *V2Service) CreateTemplate(ctx context.Context, spaceId string, body []byte, dryRun bool) (*apimodel.V2CreateResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	return s.createFromDocument(ctx, spaceId, body, docCreateOptions{dryRun: dryRun, requireTemplate: true})
}

// createFromShortcut synthesizes an AnyBlock document from the shortcut
// shape and reuses the full-document path. markdown is parsed into flat
// blocks server-side (anyblockjson.ParseMarkdownBlocks, the Phase-5 parser)
// and rides the same single-change-set create as an explicit blocks array —
// dry runs validate it, no half-built object on failure, and the C8 result
// cache replays it safely (the §7.2 two-change-set caveats are gone).
func (s *V2Service) createFromShortcut(ctx context.Context, spaceId string, fields map[string]json.RawMessage, dryRun bool) (*apimodel.V2CreateResult, error) {
	for key := range fields {
		if !shortcutKeys[key] {
			return nil, apimodel.V2ValidationFailed("unknown field in create shortcut",
				apimodel.V2Issue{
					Path:    "/" + key,
					Message: fmt.Sprintf("unknown key %q — the shortcut accepts type, name, properties, markdown", key),
					Hint:    "to send a full AnyBlock document, include \"version\": 1",
				})
		}
	}
	raw, err := encodeEnvelope(fields)
	if err != nil {
		return nil, err
	}
	var shortcut v2ObjectShortcut
	if err := json.Unmarshal(raw, &shortcut); err != nil {
		return nil, apimodel.V2ValidationFailed("decode create shortcut: " + err.Error())
	}
	if shortcut.Type == "" {
		return nil, apimodel.V2ValidationFailed("type is required",
			apimodel.V2Issue{Path: "/type", Message: "the shortcut needs a type key", Hint: "list keys with GET /v2/spaces/{spaceId}/types"})
	}

	doc := map[string]json.RawMessage{}
	if doc["version"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
		return nil, err
	}
	if doc["type"], err = rawJSON(shortcut.Type); err != nil {
		return nil, err
	}
	properties := shortcut.Properties
	if properties == nil {
		properties = map[string]json.RawMessage{}
	}
	if shortcut.Name != "" {
		if properties["name"], err = rawJSON(shortcut.Name); err != nil {
			return nil, err
		}
	}
	if len(properties) > 0 {
		if doc["properties"], err = rawJSON(properties); err != nil {
			return nil, err
		}
	}
	if shortcut.Markdown != "" {
		if run := anyblockjson.ParseMarkdownBlocks(shortcut.Markdown); len(run) > 0 {
			if doc["blocks"], err = rawJSON(run); err != nil {
				return nil, err
			}
		}
	}
	docJSON, err := encodeEnvelope(doc)
	if err != nil {
		return nil, err
	}

	return s.createFromDocument(ctx, spaceId, docJSON, docCreateOptions{dryRun: dryRun})
}

// createFromDocument is the shared full-document create path: structural
// validation → referential validation → Unmarshal with create-missing
// resolvers → snapshot create (one change set) → etag read-back.
func (s *V2Service) createFromDocument(ctx context.Context, spaceId string, body []byte, opts docCreateOptions) (*apimodel.V2CreateResult, error) {
	// 1. structural + format-semantic validation (no side effects)
	if err := s.rejectInvalidDocument(body); err != nil {
		return nil, err
	}

	var envelope docEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, apimodel.V2ValidationFailed("decode document envelope: " + err.Error())
	}
	if envelope.Type == "" {
		// absent type defaults to page on create (agent-friendly; SPEC §2
		// leaves it absent only for legacy/system objects); on the templates
		// endpoint the default is the template type itself
		envelope.Type = string(bundle.TypeKeyPage)
		if opts.requireTemplate {
			envelope.Type = string(bundle.TypeKeyTemplate)
		}
		var err error
		fields, err := parseEnvelope(body)
		if err != nil {
			return nil, err
		}
		if fields["type"], err = rawJSON(envelope.Type); err != nil {
			return nil, err
		}
		if body, err = encodeEnvelope(fields); err != nil {
			return nil, err
		}
	}

	// 2. referential validation (R9) — reject before anything is created
	if err := s.validateDocumentRefs(spaceId, &envelope, opts); err != nil {
		return nil, err
	}

	// 3. Unmarshal with create-missing resolvers (SPEC §3/§2a); on a dry run
	// the resolvers only record would-be creations
	resolvers := newCreatingResolvers(ctx, s.mw, spaceId, s.store.SpaceIndex(spaceId), opts.dryRun)
	_, snapshot, err := anyblockjson.Unmarshal(body, resolvers.Options())
	if err != nil {
		return nil, mapUnmarshalError(body, err)
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve document references: %w", err)
	}

	result := &apimodel.V2CreateResult{Type: envelope.Type, Created: resolvers.created()}
	if opts.dryRun {
		result.DryRun = true
		return result, nil
	}

	// 4. template target: templateFor (a type key) becomes the
	// targetObjectType detail (a type object id) the editor resolves
	// layout from
	if envelope.Type == string(bundle.TypeKeyTemplate) && envelope.TemplateFor != "" {
		targetId, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(envelope.TemplateFor))
		if err != nil {
			return nil, fmt.Errorf("resolve template target type %q: %w", envelope.TemplateFor, err)
		}
		if snapshot.Details == nil {
			snapshot.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}
		snapshot.Details.Fields[bundle.RelationKeyTargetObjectType.String()] = pbtypes.String(targetId)
	}

	// 5. create — the whole document as the object's initial state
	id, err := s.creator.CreateObjectFromSnapshot(ctx, spaceId, snapshot)
	if err != nil {
		return nil, fmt.Errorf("create object in space %s: %w", spaceId, err)
	}
	result.Id = id

	// 6. etag read-back (best effort — the create already succeeded)
	if read, err := s.reader.ReadObject(ctx, spaceId, id); err == nil {
		result.Etag = ComputeEtag(read.Heads)
	} else {
		result.Warnings = append(result.Warnings, apimodel.V2Issue{
			Message: "created, but the etag read-back failed — GET the object for its etag",
		})
	}
	return result, nil
}

// rejectInvalidDocument maps anyblockjson.Validate failures onto the C6
// contract: path-addressed validation_failed, or version_unsupported when
// the document was produced by a newer format version (§8: on create an
// unparseable version must fail the write).
func (s *V2Service) rejectInvalidDocument(body []byte) error {
	err := anyblockjson.Validate(body)
	if err == nil {
		return nil
	}
	return mapUnmarshalError(body, err)
}

// mapUnmarshalError converts anyblockjson validation errors into C6 errors.
func mapUnmarshalError(body []byte, err error) error {
	var validationErr *anyblockjson.ValidationError
	if !errors.As(err, &validationErr) {
		return apimodel.V2ValidationFailed("invalid AnyBlock document", apimodel.V2Issue{Message: err.Error()})
	}
	if validationErr.NewerFormat {
		docVersion, _, ok := anyblockjson.DetectFormat(body)
		if !ok {
			docVersion = anyblockjson.FormatVersion + 1
		}
		return apimodel.V2VersionUnsupported(docVersion, anyblockjson.FormatVersion)
	}
	issues := make([]apimodel.V2Issue, 0, len(validationErr.Issues))
	for _, issue := range validationErr.Issues {
		issues = append(issues, apimodel.V2Issue{Path: issue.Path, Message: issue.Message})
	}
	return apimodel.V2ValidationFailed("the document failed AnyBlock validation", issues...)
}

// validateDocumentRefs is the R9 layer for object creates: kind and type
// gating, template target, items-on-collections, and property-key existence
// in the properties map (reject with did-you-mean — creating properties from
// a possibly hallucinated key is reserved for typeProperties and
// POST /properties).
func (s *V2Service) validateDocumentRefs(spaceId string, envelope *docEnvelope, opts docCreateOptions) error {
	switch envelope.Kind {
	case "", "page", "template":
	case "objectType":
		return apimodel.V2ValidationFailed("type documents are created via their own endpoint",
			apimodel.V2Issue{Path: "/kind", Message: "kind \"objectType\" is not accepted here", Hint: fmt.Sprintf("POST /v2/spaces/%s/types", spaceId)})
	default:
		return apimodel.V2ValidationFailed("unsupported document kind",
			apimodel.V2Issue{Path: "/kind", Message: fmt.Sprintf("kind %q cannot be created through the API", envelope.Kind), Hint: "omit kind (page) or use type \"template\""})
	}

	if opts.requireTemplate {
		if envelope.Type == "" {
			envelope.Type = string(bundle.TypeKeyTemplate)
		}
		if envelope.Type != string(bundle.TypeKeyTemplate) {
			return apimodel.V2ValidationFailed("not a template document",
				apimodel.V2Issue{Path: "/type", Message: fmt.Sprintf("expected type \"template\", got %q", envelope.Type)})
		}
	}
	// a template must name its target type — the editor derives the layout
	// from it (SPEC §2 templateFor); enforced on both endpoints
	if envelope.Type == string(bundle.TypeKeyTemplate) && envelope.TemplateFor == "" {
		return apimodel.V2ValidationFailed("templateFor is required",
			apimodel.V2Issue{Path: "/templateFor", Message: "a template document names its target type key", Hint: fmt.Sprintf("list keys with GET /v2/spaces/%s/types", spaceId)})
	}

	if envelope.Type != "" && envelope.Type != string(bundle.TypeKeyTemplate) {
		if !s.typeKeyExists(spaceId, envelope.Type) {
			return s.unknownTypeKeyError(spaceId, envelope.Type, "/type")
		}
		if err := rejectRestrictedType(envelope.Type); err != nil {
			return err
		}
	}
	if envelope.TemplateFor != "" && !s.typeKeyExists(spaceId, envelope.TemplateFor) {
		return s.unknownTypeKeyError(spaceId, envelope.TemplateFor, "/templateFor")
	}

	// SPEC §2: items on a non-collection document is a wiring-enforced error
	if len(envelope.Items) > 0 && envelope.Type != string(bundle.TypeKeyCollection) {
		return apimodel.V2ValidationFailed("items on a non-collection document",
			apimodel.V2Issue{Path: "/items", Message: fmt.Sprintf("items requires type \"collection\", got %q", envelope.Type), Hint: fmt.Sprintf("POST /v2/spaces/%s/collections", spaceId)})
	}

	// property keys must exist — did-you-mean, never silent create (R9)
	var issues []apimodel.V2Issue
	var known []string
	for _, key := range sortedKeys(envelope.Properties) {
		if s.propertyKeyExists(spaceId, key) {
			continue
		}
		if known == nil {
			known = s.knownPropertyKeys(spaceId)
		}
		issues = append(issues, unknownPropertyIssue(key, "/properties/"+key, known,
			fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", spaceId, spaceId)))
	}
	if len(issues) > 0 {
		return apimodel.V2ValidationFailed("unknown property keys", issues...)
	}
	return nil
}

// rejectRestrictedType blocks creation of system-managed types through the
// generic object path (mirrors createObjectInSpace's guards).
func rejectRestrictedType(typeKey string) error {
	key := domain.TypeKey(typeKey)
	if t, err := bundle.GetType(key); err == nil && t.RestrictObjectCreation {
		return apimodel.V2ValidationFailed("this type cannot be created through the API",
			apimodel.V2Issue{Path: "/type", Message: fmt.Sprintf("creation of %q objects is restricted", typeKey)})
	}
	switch key {
	case bundle.TypeKeyFile, bundle.TypeKeyImage, bundle.TypeKeyAudio, bundle.TypeKeyVideo:
		return apimodel.V2ValidationFailed("file objects are created by upload",
			apimodel.V2Issue{Path: "/type", Message: fmt.Sprintf("%q objects come from file uploads", typeKey), Hint: "POST /v2/spaces/{spaceId}/files"})
	}
	return nil
}

// sortedKeys returns the map's keys in deterministic order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

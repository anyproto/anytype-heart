package v2service

// create.go implements the Phase-2 object create surface (APIV2.md §2):
// POST /v2/spaces/{spaceId}/objects (full AnyBlock document or the
// {type, name, properties, markdown} shortcut — discriminated per §8/R7 on
// the presence of version/blocks) and POST .../templates. The create path is
// snapshot-based: anyblockjson.Unmarshal → apicore.ObjectCreator (one change
// set), with create-missing resolvers (resolver.go) and the referential
// validation layer (refs.go) in front.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
func (s *V2Service) CreateObject(ctx context.Context, spaceId string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
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
func (s *V2Service) CreateTemplate(ctx context.Context, spaceId string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
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
func (s *V2Service) createFromShortcut(ctx context.Context, spaceId string, fields map[string]json.RawMessage, dryRun bool) (*v2model.CreateResult, error) {
	for key := range fields {
		if !shortcutKeys[key] {
			return nil, v2model.ValidationFailed("unknown field in create shortcut",
				v2model.Issue{
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
		return nil, v2model.ValidationFailed("decode create shortcut: " + err.Error())
	}
	if shortcut.Type == "" {
		return nil, v2model.ValidationFailed("type is required",
			v2model.Issue{Path: "/type", Message: "the shortcut needs a type key", Hint: "list keys with GET /v2/spaces/{spaceId}/types"})
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
	markdownBlocks := false
	if shortcut.Markdown != "" {
		run, exceeded := anyblockjson.ParseMarkdownBlocksLimit(shortcut.Markdown, v2MaxCreateMarkdownBlocks)
		if exceeded {
			return nil, v2model.ValidationFailed("markdown produced too many blocks",
				v2model.Issue{Path: "/markdown", Message: fmt.Sprintf(
					"the markdown parses to more than %d blocks — the create limit is %d; create with a shorter body and add the rest with PATCH insertBlocks",
					v2MaxCreateMarkdownBlocks, v2MaxCreateMarkdownBlocks)})
		}
		if len(run) == 0 {
			// same contract as the insertBlocks markdown channel — a silent
			// empty object teaches the caller nothing (C6)
			return nil, v2model.ValidationFailed("markdown produced no blocks",
				v2model.Issue{Path: "/markdown", Message: "the markdown body contains no content — give at least one non-blank line, or omit markdown"})
		}
		if doc["blocks"], err = rawJSON(run); err != nil {
			return nil, err
		}
		markdownBlocks = true
	}
	docJSON, err := encodeEnvelope(doc)
	if err != nil {
		return nil, err
	}

	result, err := s.createFromDocument(ctx, spaceId, docJSON, docCreateOptions{dryRun: dryRun})
	if err != nil && markdownBlocks {
		// the blocks array is synthetic here — readdress its issues to the
		// markdown channel the caller actually sent (C6)
		err = rebaseMarkdownCreateError(err)
	}
	return result, err
}

// v2MaxCreateMarkdownBlocks caps how many blocks a create shortcut's markdown
// body may parse to. Wider than the per-op insertBlocks cap (a whole document
// vs one insertion) but still a hard bound: the byte-bounded markdown channel
// would otherwise reach hundreds of thousands of blocks in one change set.
const v2MaxCreateMarkdownBlocks = 2048

// rebaseMarkdownCreateError rewrites /blocks/<j>… issue paths onto
// /markdown[<j>]… — the create-shortcut caller sent markdown, never a blocks
// array, so a path into the synthesized document is unactionable (C6). j is
// the parsed block position, the same convention the insertBlocks op's
// createdBlocks keys document.
func rebaseMarkdownCreateError(err error) error {
	var v2Err *v2model.Error
	if !errors.As(err, &v2Err) {
		return err
	}
	for i := range v2Err.Issues {
		rest, ok := strings.CutPrefix(v2Err.Issues[i].Path, "/blocks/")
		if !ok {
			continue
		}
		if idx, tail, found := strings.Cut(rest, "/"); found {
			v2Err.Issues[i].Path = fmt.Sprintf("/markdown[%s]/%s", idx, tail)
		} else {
			v2Err.Issues[i].Path = fmt.Sprintf("/markdown[%s]", rest)
		}
	}
	return v2Err
}

// normalizeCreateBody strips the v2 read-envelope additions (etag,
// warnings) so "read a document, create a copy" works from every GET shape,
// and refuses the partial ?block= subtree marker — a subtree is not a
// document.
func normalizeCreateBody(body []byte) ([]byte, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
	}
	if _, partial := fields["subtree"]; partial {
		return nil, v2model.ValidationFailed("this body is a partial ?block= subtree read, not a whole document",
			v2model.Issue{Path: "/subtree", Message: "an object cannot be created from a subtree read — it is a fragment of another document",
				Hint: "GET the source object with ?ids=full and without ?block= for a complete document"})
	}
	delete(fields, "etag") // C7: concurrency lives in headers, never in create bodies
	delete(fields, "warnings")
	return encodeEnvelope(fields)
}

// docLocalIds collects the doc-local ids a flat AnyBlock document carries
// explicitly, in document order and deduplicated — the create path's view of
// the id domain compact relabeling covers. The walk itself is
// v2EditDoc.localIds (ops.go), shared with the PATCH payload resolver so the
// two guards cannot drift into covering different slots. A body that is not
// decodable yields nil (later validation owns that failure).
func docLocalIds(doc []byte) []string {
	parsed, err := parseEditDoc(doc)
	if err != nil {
		return nil
	}
	return parsed.localIds()
}

// warnLabelShapedIds flags a create body whose local ids look like the
// default read's compact labels (5 lowercase-hex chars). Adopting them is
// legal — the new object has no other holders of those ids — but almost
// never intended: the clone's stored ids become the labels, and the
// source's real ids are one query parameter away. A warning, not a
// refusal: with no owned-id baseline to check against, a 5-hex authored id
// is indistinguishable from a label, and a clone of a document that truly
// owns such ids (they never relabel, so its export carries them verbatim)
// must keep working.
func warnLabelShapedIds(body []byte) []v2model.Issue {
	var labelLike []string
	for _, id := range docLocalIds(body) {
		if anyblockjson.IsCompactLabelShaped(id) {
			labelLike = append(labelLike, strconv.Quote(id))
		}
	}
	if len(labelLike) == 0 {
		return nil
	}
	return []v2model.Issue{{
		Path:    "/blocks",
		Message: fmt.Sprintf("ids %s look like compact labels from a default read and were adopted as this object's real ids", strings.Join(labelLike, ", ")),
		Hint:    "to clone with the source's real ids, GET it with ?ids=full; to mint fresh ids, omit them",
	}}
}

// createFromDocument is the shared full-document create path: structural
// validation → referential validation → Unmarshal with create-missing
// resolvers → snapshot create (one change set) → etag read-back.
func (s *V2Service) createFromDocument(ctx context.Context, spaceId string, body []byte, opts docCreateOptions) (*v2model.CreateResult, error) {
	// 0. envelope normalization: a pasted read body creates a copy instead of
	// 400ing on its own etag
	body, err := normalizeCreateBody(body)
	if err != nil {
		return nil, err
	}

	// 1. structural + format-semantic validation (no side effects)
	if err := s.rejectInvalidDocument(body); err != nil {
		return nil, err
	}

	// 1a. canonicalize addressing terms (§7.5a-5): api-key slugs in the
	// envelope's type/templateFor and in the properties map resolve to
	// their stored spellings before validation and import
	body, err = s.canonicalizeDocumentKeys(spaceId, body)
	if err != nil {
		return nil, err
	}

	var envelope docEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, v2model.ValidationFailed("decode document envelope: " + err.Error())
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
	resolvers := s.newCreatingResolvers(ctx, spaceId, opts.dryRun)
	_, snapshot, err := anyblockjson.Unmarshal(body, resolvers.Options())
	if err != nil {
		return nil, mapUnmarshalError(body, err)
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve document references: %w", err)
	}

	result := &v2model.CreateResult{Type: envelope.Type, Created: resolvers.created()}
	// the label-adoption tell rides real runs and dry runs alike (C9)
	result.Warnings = warnLabelShapedIds(body)
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
		result.Warnings = append(result.Warnings, v2model.Issue{
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
		return v2model.ValidationFailed("invalid AnyBlock document", v2model.Issue{Message: err.Error()})
	}
	if validationErr.NewerFormat {
		docVersion, _, ok := anyblockjson.DetectFormat(body)
		if !ok {
			docVersion = anyblockjson.FormatVersion + 1
		}
		return v2model.VersionUnsupported(docVersion, anyblockjson.FormatVersion)
	}
	issues := make([]v2model.Issue, 0, len(validationErr.Issues))
	for _, issue := range validationErr.Issues {
		issues = append(issues, v2model.Issue{Path: issue.Path, Message: issue.Message})
	}
	return v2model.ValidationFailed("the document failed AnyBlock validation", issues...)
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
		return v2model.ValidationFailed("type documents are created via their own endpoint",
			v2model.Issue{Path: "/kind", Message: "kind \"objectType\" is not accepted here", Hint: fmt.Sprintf("POST /v2/spaces/%s/types", spaceId)})
	default:
		return v2model.ValidationFailed("unsupported document kind",
			v2model.Issue{Path: "/kind", Message: fmt.Sprintf("kind %q cannot be created through the API", envelope.Kind), Hint: "omit kind (page) or use type \"template\""})
	}
	// the envelope key is the derived-identity slot of TYPE documents; on an
	// object document it would ride into snapshot.Key and DeriveTreeObject —
	// forged deterministic identity through a channel no guard inspects
	// (ADDRESSING §2.4). Reject, never strip: identity is nothing to drop
	// silently.
	if envelope.Key != "" {
		return v2model.ValidationFailed("key is not accepted on an object document",
			v2model.Issue{Path: "/key", Message: "key is the identity slot of type documents only", Hint: "remove key — objects are identified by their minted id"})
	}

	if opts.requireTemplate {
		if envelope.Type == "" {
			envelope.Type = string(bundle.TypeKeyTemplate)
		}
		if envelope.Type != string(bundle.TypeKeyTemplate) {
			return v2model.ValidationFailed("not a template document",
				v2model.Issue{Path: "/type", Message: fmt.Sprintf("expected type \"template\", got %q", envelope.Type)})
		}
	}
	// a template must name its target type — the editor derives the layout
	// from it (SPEC §2 templateFor); enforced on both endpoints
	if envelope.Type == string(bundle.TypeKeyTemplate) && envelope.TemplateFor == "" {
		return v2model.ValidationFailed("templateFor is required",
			v2model.Issue{Path: "/templateFor", Message: "a template document names its target type key", Hint: fmt.Sprintf("list keys with GET /v2/spaces/%s/types", spaceId)})
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
		return v2model.ValidationFailed("items on a non-collection document",
			v2model.Issue{Path: "/items", Message: fmt.Sprintf("items requires type \"collection\", got %q", envelope.Type), Hint: fmt.Sprintf("POST /v2/spaces/%s/collections", spaceId)})
	}

	// property keys must exist — did-you-mean, never silent create (R9)
	return s.validatePropertyKeys(spaceId, envelope.Properties)
}

// validatePropertyKeys is the R9 unknown-property loop over a document's
// properties map. One primed live set for the whole loop (§7.5a-2),
// failing closed on a load error. Only LIVE properties pass: the
// corpse-tolerant variant existed solely so a GET→PUT round trip of a
// document holding values of a UI-deleted relation would not 400, and it
// retired with PUT — a PATCH never resends a property it is not editing.
func (s *V2Service) validatePropertyKeys(spaceId string, props map[string]json.RawMessage) error {
	if len(props) == 0 {
		return nil
	}
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return err
	}
	var issues []v2model.Issue
	var known []string
	for _, key := range sortedKeys(props) {
		if propertyKeyExistsIn(entries, key) {
			continue
		}
		if known == nil {
			known = knownPropertyKeysIn(entries)
		}
		issues = append(issues, unknownPropertyIssue(key, "/properties/"+key, known,
			fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", spaceId, spaceId)))
	}
	if len(issues) > 0 {
		return v2model.ValidationFailed("unknown property keys", issues...)
	}
	return nil
}

// rejectRestrictedType blocks creation of system-managed types through the
// generic object path (mirrors createObjectInSpace's guards).
func rejectRestrictedType(typeKey string) error {
	key := domain.TypeKey(typeKey)
	if t, err := bundle.GetType(key); err == nil && t.RestrictObjectCreation {
		return v2model.ValidationFailed("this type cannot be created through the API",
			v2model.Issue{Path: "/type", Message: fmt.Sprintf("creation of %q objects is restricted", typeKey)})
	}
	switch key {
	case bundle.TypeKeyFile, bundle.TypeKeyImage, bundle.TypeKeyAudio, bundle.TypeKeyVideo:
		return v2model.ValidationFailed("file objects are created by upload",
			v2model.Issue{Path: "/type", Message: fmt.Sprintf("%q objects come from file uploads", typeKey), Hint: "POST /v2/spaces/{spaceId}/files"})
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

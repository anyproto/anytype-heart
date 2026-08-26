package v2service

// create.go implements the Phase-2 object create surface (APIV2.md §2):
// POST /v2/spaces/{space_id}/objects (full AnyBlock document or the
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
	Kind        string                     `json:"kind"`
	Type        string                     `json:"type"`
	TemplateFor string                     `json:"template_for"`
	Properties  map[string]json.RawMessage `json:"properties"`
	// TypeSettings is the §2a gated subtree of a TYPE document. The envelope
	// `key` and the top-level `type_properties` array both moved in here —
	// `key` as `api_key` (it was always the apiObjectKey slug) and the array
	// as `property_definitions`. Only the two members this package's own
	// logic reads are modelled: the rest reach the store through
	// anyblockjson.Unmarshal and the snapshot, never through this struct.
	TypeSettings *typeSettingsEnvelope `json:"type_settings"`
	Items        []string              `json:"items"`
}

// typeSettingsEnvelope is the slice of §2a's type_settings the API's own
// identity and validation layers read.
type typeSettingsEnvelope struct {
	ApiKey              string          `json:"api_key"`
	PropertyDefinitions json.RawMessage `json:"property_definitions"`
}

// apiKey is the caller's proposed api slug, nil-safe.
func (e docEnvelope) apiKey() string {
	if e.TypeSettings == nil {
		return ""
	}
	return e.TypeSettings.ApiKey
}

// propertyDefinitions is the §2a property-definition array, nil-safe.
func (e docEnvelope) propertyDefinitions() json.RawMessage {
	if e.TypeSettings == nil {
		return nil
	}
	return e.TypeSettings.PropertyDefinitions
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

// CreateObject implements POST /v2/spaces/{space_id}/objects.
func (s *Service) CreateObject(ctx context.Context, spaceId string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
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

// CreateTemplate implements POST /v2/spaces/{space_id}/templates: an AnyBlock
// document with template_for, routed through the generic object-create path
// (no create-from-body template RPC exists — APIV2.md Phase 2).
//
// The endpoint IS the kind, exactly as POST /types is: a template must now
// say `kind: "template"` (the type "template" stopped carrying that meaning
// on its own), and requiring a caller to restate what the URL already said
// is the trap C2 exists to avoid. Injected here rather than defaulted deeper
// so the whole create path below sees a document that is already complete.
func (s *Service) CreateTemplate(ctx context.Context, spaceId string, body []byte, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, v2model.ValidationFailed("request body is not a JSON object",
			v2model.Issue{Message: err.Error()})
	}
	if _, ok := fields["kind"]; !ok {
		if fields["kind"], err = rawJSON("template"); err != nil {
			return nil, err
		}
		if body, err = encodeEnvelope(fields); err != nil {
			return nil, err
		}
	}
	return s.createFromDocument(ctx, spaceId, body, docCreateOptions{dryRun: dryRun, requireTemplate: true})
}

// createFromShortcut synthesizes an AnyBlock document from the shortcut
// shape and reuses the full-document path. markdown is parsed into flat
// blocks server-side (anyblockjson.ParseMarkdownBlocks, the Phase-5 parser)
// and rides the same single-change-set create as an explicit blocks array —
// dry runs validate it, no half-built object on failure, and the C8 result
// cache replays it safely (the §7.2 two-change-set caveats are gone).
func (s *Service) createFromShortcut(ctx context.Context, spaceId string, fields map[string]json.RawMessage, dryRun bool) (*v2model.CreateResult, error) {
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
			v2model.Issue{Path: "/type", Message: "the shortcut needs a type key", Hint: "list keys with GET /v2/spaces/{space_id}/types"})
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
					"the markdown parses to more than %d blocks — the create limit is %d; create with a shorter body and add the rest with PATCH insert_blocks",
					v2MaxCreateMarkdownBlocks, v2MaxCreateMarkdownBlocks)})
		}
		if len(run) == 0 {
			// same contract as the insert_blocks markdown channel — a silent
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
// body may parse to. Wider than the per-op insert_blocks cap (a whole document
// vs one insertion) but still a hard bound: the byte-bounded markdown channel
// would otherwise reach hundreds of thousands of blocks in one change set.
const v2MaxCreateMarkdownBlocks = 2048

// rebaseMarkdownCreateError rewrites /blocks/<j>… issue paths onto
// /markdown[<j>]… — the create-shortcut caller sent markdown, never a blocks
// array, so a path into the synthesized document is unactionable (C6). j is
// the parsed block position, the same convention the insert_blocks op's
// created_blocks keys document.
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
func (s *Service) createFromDocument(ctx context.Context, spaceId string, body []byte, opts docCreateOptions) (*v2model.CreateResult, error) {
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
	// their stored spellings before validation and import; the spelling map
	// keeps refusal paths addressed to the request as sent
	body, spellings, err := s.canonicalizeDocumentKeys(spaceId, body)
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
	if err := s.validateDocumentRefs(ctx, spaceId, &envelope, opts, spellings); err != nil {
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
func (s *Service) rejectInvalidDocument(body []byte) error {
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
// POST /properties). spellings maps canonicalized keys back to the caller's
// own spelling (canonicalizeDocumentKeys), so refusals address the request
// that was actually sent.
func (s *Service) validateDocumentRefs(ctx context.Context, spaceId string, envelope *docEnvelope, opts docCreateOptions, spellings map[string]string) error {
	switch envelope.Kind {
	case "", "page", "template":
	case "object_type":
		return v2model.ValidationFailed("type documents are created via their own endpoint",
			v2model.Issue{Path: "/kind", Message: "kind \"object_type\" is not accepted here", Hint: fmt.Sprintf("POST /v2/spaces/%s/types", spaceId)})
	default:
		return v2model.ValidationFailed("unsupported document kind",
			v2model.Issue{Path: "/kind", Message: fmt.Sprintf("kind %q cannot be created through the API", envelope.Kind), Hint: "omit kind (page) or use type \"template\""})
	}
	// §2a's identity slots (`type_settings`, and the envelope `key` that
	// preceded it) are refused by the format itself, path-addressed and on
	// every kind — including the forged-identity case this layer used to
	// guard (ADDRESSING §2.4). One statement of the rule, in the validator.

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
	// from it (SPEC §2 template_for); enforced on both endpoints
	if envelope.Type == string(bundle.TypeKeyTemplate) && envelope.TemplateFor == "" {
		return v2model.ValidationFailed("template_for is required",
			v2model.Issue{Path: "/template_for", Message: "a template document names its target type key", Hint: fmt.Sprintf("list keys with GET /v2/spaces/%s/types", spaceId)})
	}

	if envelope.Type != "" && envelope.Type != string(bundle.TypeKeyTemplate) {
		if !s.typeKeyExists(spaceId, envelope.Type) {
			return s.unknownTypeKeyError(spaceId, envelope.Type, "/type")
		}
		if err := rejectRestrictedType(envelope.Type); err != nil {
			return err
		}
		// the bundled table answers for a type key forever, so existence alone
		// cannot see that this SPACE removed the type — without this check a
		// create landed a new object in a type whose route 404s, and a
		// reinstall lit it back up (§8.41; the type twin of the property
		// refusal below)
		if err := s.refuseRemovedType(ctx, spaceId, envelope.Type, "/type"); err != nil {
			return err
		}
	}
	if envelope.TemplateFor != "" {
		if !s.typeKeyExists(spaceId, envelope.TemplateFor) {
			return s.unknownTypeKeyError(spaceId, envelope.TemplateFor, "/template_for")
		}
		if err := s.refuseRemovedType(ctx, spaceId, envelope.TemplateFor, "/template_for"); err != nil {
			return err
		}
	}

	// SPEC §2: items on a non-collection document is a wiring-enforced error
	if len(envelope.Items) > 0 && envelope.Type != string(bundle.TypeKeyCollection) {
		return v2model.ValidationFailed("items on a non-collection document",
			v2model.Issue{Path: "/items", Message: fmt.Sprintf("items requires type \"collection\", got %q", envelope.Type), Hint: fmt.Sprintf("POST /v2/spaces/%s/collections", spaceId)})
	}

	// property keys must exist — did-you-mean, never silent create (R9)
	return s.validatePropertyKeys(ctx, spaceId, envelope.Properties, spellings)
}

// refuseRemovedType is the type-namespace removal gate for one canonicalized
// type slot (§8.41). Fails closed on any probe error: an unverifiable
// removal set must not read as "nothing was removed".
func (s *Service) refuseRemovedType(ctx context.Context, spaceId, typeKey, path string) error {
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return err
	}
	removed, err := s.bundledTypeRemovalSet(spaceId)
	if err != nil {
		return err
	}
	isRemoved, err := s.bundledTypeRemoved(ctx, spaceId, entries, removed, typeKey)
	if err != nil {
		return err
	}
	if isRemoved {
		return v2model.ValidationFailed("removed type key", removedTypeIssue(spaceId, typeKey, path))
	}
	return nil
}

// validatePropertyKeys is the R9 unknown-property loop over a document's
// properties map. One primed live set for the whole loop (§7.5a-2), failing
// closed on a load error.
//
// A LIVE property passes as an address. A key no live property claims but
// SOME relation object still holds — a UI-deleted or archived one, a
// "corpse" — passes as a round-trip tolerance
// (propertyKeyHeldByAnyRelation): an object holding values of such a
// relation exports that key, and create is the channel a read body is
// pasted into ("a pasted read body creates a copy", §3(b)). Refusing it
// would make the advertised clone loop fail on a document the API itself
// served, and would do so with a did-you-mean pointing at some unrelated
// live key that happens to be spelled nearby — moving a value onto the
// wrong property is worse than carrying a dormant one. The tolerance is a
// bare existence probe BY DESIGN — it cannot, and does not claim to,
// distinguish a pasted clone from a freshly authored value (see
// propertyKeyHeldByAnyRelation for why no provenance signal is worth its
// cost on a key that resolves nowhere).
//
// This tolerance is not create-specific special pleading: PATCH has the
// same escape by another route (stateops.go checkKey passes any key already
// present on the document). Both say the same thing — a document may keep a
// value it legitimately already carries; neither is an ADDRESS, because
// neither channel will resolve a corpse key to a property object.
//
// The ONE key class the tolerance does not cover is a BUNDLED relation this
// space removed (removedPropertyIssue; §8.41 widened "removed" from
// uninstalled to archived too, and to the tombstone window): bundle.
// HasRelation answers for it forever, so without the explicit check a
// create lands new data on a property the user deleted, and the reinstall
// lights it back up.
//
// spellings maps a canonicalized key back to the caller's spelling —
// refusal paths must address the request as sent, not the rewrite
// (§8.41-10).
func (s *Service) validatePropertyKeys(ctx context.Context, spaceId string, props map[string]json.RawMessage, spellings map[string]string) error {
	if len(props) == 0 {
		return nil
	}
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return err
	}
	spelledAs := func(key string) string {
		if original, ok := spellings[key]; ok {
			return original
		}
		return key
	}
	var issues []v2model.Issue
	var removedCount int
	var known []string
	// primed lazily and at most once (§7.5a-2), and only when a key reaches
	// the bundled arm at all
	var removedBundled map[string]bool
	for _, key := range sortedKeys(props) {
		if propertyKeyExistsIn(entries, key) {
			if !propertyKeyInstalledIn(entries, key) {
				if removedBundled == nil {
					if removedBundled, err = s.bundledRemovalSet(spaceId); err != nil {
						return err
					}
				}
				isRemoved, err := s.bundledPropertyRemoved(ctx, spaceId, entries, removedBundled, key)
				if err != nil {
					return err
				}
				if isRemoved {
					issues = append(issues, removedPropertyIssue(spaceId, key, spelledAs(key), "/properties/"+spelledAs(key)))
					removedCount++
				}
			}
			continue
		}
		if s.propertyKeyHeldByAnyRelation(ctx, spaceId, key) {
			continue
		}
		if known == nil {
			known = knownPropertyKeysIn(entries)
		}
		issues = append(issues, unknownPropertyIssue(key, "/properties/"+spelledAs(key), known,
			fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", spaceId, spaceId)))
	}
	if len(issues) > 0 {
		// the envelope names what actually happened: "unknown" on a key the
		// space knows and removed is a lie the issue text then contradicts
		switch {
		case removedCount == len(issues):
			return v2model.ValidationFailed("removed property keys", issues...)
		case removedCount > 0:
			return v2model.ValidationFailed("unknown and removed property keys", issues...)
		}
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
			v2model.Issue{Path: "/type", Message: fmt.Sprintf("%q objects come from file uploads", typeKey), Hint: "POST /v2/spaces/{space_id}/files"})
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

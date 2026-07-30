package service

// v2_edit.go implements the Phase-3 edit surface (APIV2.md §2 Phase 3):
// PATCH /v2/spaces/{spaceId}/objects/{objectId} (the batched, atomic op set —
// v2_ops.go) and PUT (full-document replace, the escape hatch). Both run one
// pipeline under the object lock: marshal the live state → mutate the
// document (ops) or take the client's (PUT) → full format validation (the R5
// normative post-op check, SPEC §12 V1–V5) → Unmarshal with the Phase-2
// create-missing resolvers → ONE diff-apply through apicore.ObjectMutator.
// diffStats come from the canonical before/after documents (v2_diff.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2PatchRequest is the PATCH body: the closed op list, nothing else.
type v2PatchRequest struct {
	Ops []json.RawMessage `json:"ops"`
}

// PatchObject implements PATCH /v2/spaces/{spaceId}/objects/{objectId}.
func (s *V2Service) PatchObject(ctx context.Context, spaceId, objectId string, body []byte, ifMatch string, dryRun bool) (*apimodel.V2EditResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	ops, err := parsePatchRequest(body)
	if err != nil {
		return nil, err
	}

	var result *apimodel.V2EditResult
	build := func(cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error) {
		snapshot, res, err := s.patchPipeline(ctx, spaceId, objectId, ops, ifMatch, dryRun, cur)
		if err != nil {
			return nil, err
		}
		result = res
		return snapshot, nil
	}
	return s.runEdit(ctx, spaceId, objectId, dryRun, build, &result)
}

// PutObject implements PUT /v2/spaces/{spaceId}/objects/{objectId}: the body
// is a full AnyBlock document that replaces the object's content in one
// change set. Minimal CRDT diff iff the block ids round-trip from the GET
// (they do on default reads, C4); diffStats make an accidental full rewrite
// visible.
func (s *V2Service) PutObject(ctx context.Context, spaceId, objectId string, body []byte, ifMatch string, dryRun bool) (*apimodel.V2EditResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	body, err := normalizePutBody(body, objectId)
	if err != nil {
		return nil, err
	}
	if err := s.rejectInvalidDocument(body); err != nil {
		return nil, err
	}
	var envelope docEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, apimodel.V2ValidationFailed("decode document envelope: " + err.Error())
	}
	// the R9 referential layer guards PUT like create: kind gating, type and
	// property keys with did-you-mean, items-on-collections
	if err := s.validateDocumentRefs(spaceId, &envelope, docCreateOptions{}); err != nil {
		return nil, err
	}

	var result *apimodel.V2EditResult
	build := func(cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error) {
		snapshot, res, err := s.putPipeline(ctx, spaceId, objectId, body, envelope, ifMatch, dryRun, cur)
		if err != nil {
			return nil, err
		}
		result = res
		return snapshot, nil
	}
	return s.runEdit(ctx, spaceId, objectId, dryRun, build, &result)
}

// runEdit executes an edit build: through the mutator (one locked
// diff-apply) on a real run, through a plain read on a dry run (C9 — nothing
// is committed, the would-be outcome rides the result).
func (s *V2Service) runEdit(ctx context.Context, spaceId, objectId string, dryRun bool, build func(apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error), result **apimodel.V2EditResult) (*apimodel.V2EditResult, error) {
	if dryRun {
		cur, err := s.reader.ReadObject(ctx, spaceId, objectId)
		if err != nil {
			return nil, mapReadError(spaceId, objectId, err)
		}
		if _, err := build(cur); err != nil {
			return nil, err
		}
		(*result).DryRun = true
		return *result, nil
	}
	heads, err := s.mutator.MutateObject(ctx, spaceId, objectId, build)
	if err != nil {
		var v2Err *apimodel.V2Error
		if errors.As(err, &v2Err) {
			return nil, v2Err
		}
		return nil, mapReadError(spaceId, objectId, err)
	}
	(*result).Etag = ComputeEtag(heads)
	return *result, nil
}

// parsePatchRequest decodes the PATCH body strictly.
func parsePatchRequest(body []byte) ([]json.RawMessage, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, apimodel.V2ValidationFailed("the PATCH body must be a JSON object",
			apimodel.V2Issue{Message: err.Error(), Hint: `send {"ops": [...]} — GET /v2/schemas/ops/{op} documents each op`})
	}
	for key := range fields {
		if key != "ops" {
			return nil, apimodel.V2ValidationFailed("unknown field in PATCH body",
				apimodel.V2Issue{Path: "/" + key, Message: fmt.Sprintf("unknown key %q — the PATCH body carries only ops", key), Hint: "the If-Match precondition is a header, not a body field (C7)"})
		}
	}
	var req v2PatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, apimodel.V2ValidationFailed("decode ops: " + err.Error())
	}
	if len(req.Ops) == 0 {
		return nil, apimodel.V2ValidationFailed("ops must not be empty",
			apimodel.V2Issue{Path: "/ops", Message: "give at least one op", Hint: "allowed ops: " + joinOpNames()})
	}
	return req.Ops, nil
}

// patchPipeline runs the whole PATCH against one consistent read. It returns
// the snapshot to diff-apply plus the response (sans etag, which the caller
// derives from the post-apply heads).
func (s *V2Service) patchPipeline(ctx context.Context, spaceId, objectId string, ops []json.RawMessage, ifMatch string, dryRun bool, cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, *apimodel.V2EditResult, error) {
	if err := checkEditPreconditions(cur, ifMatch); err != nil {
		return nil, nil, err
	}
	beforeDoc, err := s.marshalForEdit(spaceId, objectId, cur, true)
	if err != nil {
		return nil, nil, err
	}
	doc, err := parseEditDoc(beforeDoc)
	if err != nil {
		return nil, nil, fmt.Errorf("object %s: %w", objectId, err)
	}
	applier := newV2PatchApplier(s, spaceId, doc)
	for i, raw := range ops {
		if err := applier.apply(i, raw); err != nil {
			return nil, nil, err
		}
	}
	edited, err := doc.encode()
	if err != nil {
		return nil, nil, err
	}
	// R5, normative: the post-op document must satisfy the format's semantic
	// checks (SPEC §12 V1–V5); a violation rejects the whole PATCH with the
	// format's path-addressed errors (block paths; op-shaped cases were
	// already caught above with ops[i] paths)
	if err := anyblockjson.Validate(edited); err != nil {
		verr := mapUnmarshalError(edited, err)
		var v2Err *apimodel.V2Error
		if errors.As(verr, &v2Err) && v2Err.Code == apimodel.V2CodeValidationFailed {
			v2Err.Message = "the ops would produce an invalid document — no op was applied"
		}
		return nil, nil, verr
	}
	snapshot, result, err := s.finishEdit(ctx, spaceId, edited, beforeDoc, dryRun)
	if err != nil {
		return nil, nil, err
	}
	if len(applier.createdBlocks) > 0 {
		result.CreatedBlocks = applier.createdBlocks
	}
	return snapshot, result, nil
}

// putPipeline runs the PUT against one consistent read.
func (s *V2Service) putPipeline(ctx context.Context, spaceId, objectId string, body []byte, envelope docEnvelope, ifMatch string, dryRun bool, cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, *apimodel.V2EditResult, error) {
	if err := checkEditPreconditions(cur, ifMatch); err != nil {
		return nil, nil, err
	}
	// an absent type keeps the live object's (a full replace is about
	// content, not retyping by omission)
	if envelope.Type == "" {
		if liveType := objectTypeKey(cur); liveType != "" {
			fields, err := parseEnvelope(body)
			if err != nil {
				return nil, nil, err
			}
			if fields["type"], err = rawJSON(liveType); err != nil {
				return nil, nil, err
			}
			if body, err = encodeEnvelope(fields); err != nil {
				return nil, nil, err
			}
		}
	}
	beforeDoc, err := s.marshalForEdit(spaceId, objectId, cur, false)
	if err != nil {
		return nil, nil, err
	}
	snapshot, result, err := s.finishEdit(ctx, spaceId, body, beforeDoc, dryRun)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, result, nil
}

// finishEdit is the shared tail of both pipelines: Unmarshal the target
// document with the Phase-2 create-missing resolvers (select option names
// create, everything else was validated), marshal the resulting snapshot
// back to its canonical form, and diff it against the before-document.
func (s *V2Service) finishEdit(ctx context.Context, spaceId string, targetDoc, beforeDoc []byte, dryRun bool) (*model.SmartBlockSnapshotBase, *apimodel.V2EditResult, error) {
	resolvers := newCreatingResolvers(ctx, s.mw, spaceId, s.store.SpaceIndex(spaceId), dryRun)
	sbType, snapshot, err := anyblockjson.Unmarshal(targetDoc, resolvers.Options())
	if err != nil {
		return nil, nil, mapUnmarshalError(targetDoc, err)
	}
	if err := resolvers.err(); err != nil {
		return nil, nil, fmt.Errorf("resolve document references: %w", err)
	}
	afterDoc, err := anyblockjson.Marshal(sbType, snapshot, storeresolver.New(s.store.SpaceIndex(spaceId)).Options())
	if err != nil {
		return nil, nil, fmt.Errorf("marshal edited state: %w", err)
	}
	stats, err := diffEditDocs(beforeDoc, afterDoc)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, &apimodel.V2EditResult{Created: resolvers.created(), DiffStats: stats}, nil
}

// checkEditPreconditions applies the C7 If-Match check against the live
// heads (advisory: absent = last-write-wins) and the canUpdateObject system
// exclusions (system-managed smartblock types are not editable through the
// generic object surface).
func checkEditPreconditions(cur apicore.ObjectRead, ifMatch string) error {
	switch cur.SbType {
	case model.SmartBlockType_STRelation, model.SmartBlockType_STRelationOption,
		model.SmartBlockType_FileObject, model.SmartBlockType_Participant:
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("this object is system-managed (%s) and cannot be edited through the object surface", cur.SbType.String()),
			apimodel.V2Issue{Message: "properties, types and files have their own endpoints"})
	}
	if !EtagMatches(ifMatch, cur.Heads) {
		return apimodel.V2EtagMismatch(ComputeEtag(cur.Heads))
	}
	return nil
}

// marshalForEdit marshals the live state into the full-id document the edit
// pipeline works on. With guardWarnings (PATCH), any C11 marshal warning
// aborts the edit: content the format cannot represent would silently vanish
// in the write-back — the one thing C11 forbids. PUT skips the guard (a full
// replace is explicitly destructive) and the caller surfaces the warnings.
func (s *V2Service) marshalForEdit(spaceId, objectId string, cur apicore.ObjectRead, guardWarnings bool) ([]byte, error) {
	opts := storeresolver.New(s.store.SpaceIndex(spaceId)).Options()
	var warnings []apimodel.V2Issue
	opts.OnWarning = func(iss anyblockjson.Issue) {
		warnings = append(warnings, apimodel.V2Issue{Path: iss.Path, Message: iss.Message})
	}
	doc, err := anyblockjson.Marshal(cur.SbType, cur.Snapshot, opts)
	if err != nil {
		return nil, fmt.Errorf("marshal object %s: %w", objectId, err)
	}
	if guardWarnings && len(warnings) > 0 {
		return nil, apimodel.NewV2Error(http.StatusUnprocessableEntity, apimodel.V2CodeValidationFailed,
			"this object contains content the AnyBlock format cannot fully represent — a PATCH would drop it (C11); edit it in the app or replace it wholesale with PUT",
			warnings...)
	}
	return doc, nil
}

// normalizePutBody strips the v2 read-envelope additions (etag, warnings —
// so a GET body round-trips verbatim into PUT) and pins the envelope id to
// the addressed object.
func normalizePutBody(body []byte, objectId string) ([]byte, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, apimodel.V2ValidationFailed("the PUT body must be a full AnyBlock document",
			apimodel.V2Issue{Message: err.Error()})
	}
	delete(fields, "etag") // C7: preconditions ride the If-Match header only
	delete(fields, "warnings")
	if raw, ok := fields["id"]; ok {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil && id != "" && id != objectId {
			return nil, apimodel.V2ValidationFailed("the document id does not match the addressed object",
				apimodel.V2Issue{Path: "/id", Message: fmt.Sprintf("got %q, the URL addresses %q — omit id or repeat the addressed one", id, objectId)})
		}
	}
	if fields["id"], err = rawJSON(objectId); err != nil {
		return nil, err
	}
	return encodeEnvelope(fields)
}

func joinOpNames() string {
	out := ""
	for i, name := range v2OpNames {
		if i > 0 {
			out += ", "
		}
		out += name
	}
	return out
}

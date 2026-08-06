package v2service

// edit.go implements the Phase-3 edit surface (APIV2.md §2 Phase 3):
// PATCH /v2/spaces/{spaceId}/objects/{objectId} (the batched, atomic op set —
// ops.go) and PUT (full-document replace, the escape hatch).
//
// PATCH applies the ops to a child *state.State of the live object
// (stateops.go) and the adapter commits it with ONE ordinary sb.Apply —
// the Block* RPC handler model. The flat document is still rendered under
// the lock, but only as the read-only view the ops address (refs, indents,
// error texts) and as the diffStats input; nothing round-trips the whole
// document through Unmarshal anymore. Create-missing option resolution runs
// BEFORE the object lock (resolver.go prewarm), so no create-RPC ever
// holds the lock.
//
// PUT still runs the document-level pipeline (marshal live → validate the
// client body → Unmarshal → reset-to-version diff-apply via ResetObject);
// its stage-3 rework is pending. diffStats for both come from the canonical
// before/after documents (diff.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2MaxOpsPerPatch bounds one PATCH batch. Each op re-renders the document
// view while the object lock is held, so an unbounded batch would hold the
// lock for O(ops × document) work (review A′2).
const v2MaxOpsPerPatch = 512

// v2PatchRequest is the PATCH body: the closed op list, nothing else.
type v2PatchRequest struct {
	Ops []json.RawMessage `json:"ops"`
}

// PatchObject implements PATCH /v2/spaces/{spaceId}/objects/{objectId}: the
// ops apply to a child state of the live object, committed with one ordinary
// Apply (stateops.go).
func (s *V2Service) PatchObject(ctx context.Context, spaceId, objectId string, body []byte, ifMatch string, dryRun bool) (*v2model.EditResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	ops, err := parsePatchRequest(body)
	if err != nil {
		return nil, err
	}
	resolvers := newCreatingResolvers(ctx, s.mw, spaceId, s.store.SpaceIndex(spaceId), dryRun)

	// Create-missing option resolution runs before the object lock, so no
	// create-RPC ever holds it (review B6/A6) — but it must NOT run before the
	// request is known to be legitimate: prewarming first meant a PATCH to a
	// nonexistent object (404), with a stale If-Match (412) or on a restricted
	// object (403) still permanently created every option the batch named
	// (review A′1). So: read the object and check the preconditions first,
	// prewarm only once they pass, then take the lock.
	cur, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, mapReadError(spaceId, objectId, err)
	}
	if err := checkEditPreconditions(cur.SbType, cur.Heads, ifMatch); err != nil {
		return nil, err
	}
	// the object's own restrictions, from the same read — so a dry run reaches
	// the same verdict as the real edit rather than reporting a success the
	// adapter would refuse (review C′3)
	if cur.EditRefused != nil {
		return nil, cur.EditRefused
	}
	s.prewarmCreateMissing(ops, resolvers)

	var result *v2model.EditResult
	run := func(edit apicore.ObjectEdit) error {
		res, err := s.applyPatchOps(ctx, spaceId, objectId, ops, ifMatch, edit, resolvers)
		if err != nil {
			return err
		}
		result = res
		return nil
	}
	if dryRun {
		edit, err := editFromRead(objectId, cur)
		if err != nil {
			return nil, err
		}
		if err := run(edit); err != nil {
			return nil, err
		}
		result.DryRun = true
		return result, nil
	}
	heads, err := s.mutator.MutateObject(ctx, spaceId, objectId, run)
	if err != nil {
		var v2Err *v2model.Error
		if errors.As(err, &v2Err) {
			return nil, v2Err
		}
		return nil, mapReadError(spaceId, objectId, err)
	}
	result.Etag = ComputeEtag(heads)
	return result, nil
}

// editFromRead builds a dry-run editing session from a plain read: a private
// state reconstructed from the snapshot, never committed (C9).
func editFromRead(objectId string, cur apicore.ObjectRead) (apicore.ObjectEdit, error) {
	st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: cur.Snapshot})
	if err != nil {
		return apicore.ObjectEdit{}, fmt.Errorf("state from read snapshot: %w", err)
	}
	return apicore.ObjectEdit{SbType: cur.SbType, Heads: cur.Heads, State: st}, nil
}

// applyPatchOps runs the whole PATCH against one editing session: the C11
// write-safety guard, the ops (each validated with its ops[i] paths and
// applied to the state), the resolver error check, the flag-gated safety
// net, and the diffStats. The caller commits (or, on dry run, discards) the
// state.
func (s *V2Service) applyPatchOps(ctx context.Context, spaceId, objectId string, ops []json.RawMessage, ifMatch string, edit apicore.ObjectEdit, resolvers *creatingResolvers) (*v2model.EditResult, error) {
	if err := checkEditPreconditions(edit.SbType, edit.Heads, ifMatch); err != nil {
		return nil, err
	}
	applier := newV2StateApplier(s, spaceId, objectId, edit.SbType, edit.State, resolvers)
	beforeDoc, err := applier.begin()
	if err != nil {
		return nil, err
	}
	for i, raw := range ops {
		// the loop runs under the object lock: honour cancellation so an
		// abandoned request stops holding it (review A′2)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := applier.apply(i, raw); err != nil {
			return nil, err
		}
	}
	if err := resolvers.err(); err != nil {
		return nil, fmt.Errorf("resolve document references: %w", err)
	}
	// reuse the view's document when the last op left it valid (review A′2)
	afterDoc, err := applier.currentDoc()
	if err != nil {
		return nil, err
	}
	// R5: the whole-document net, on by default (review B′3) — catches what a
	// payload fragment cannot see (V3 containment, the document-wide id
	// domain, the absolute depth bound)
	if err := validateEditedDoc(objectId, afterDoc); err != nil {
		return nil, err
	}
	stats, err := diffEditDocs(beforeDoc, afterDoc)
	if err != nil {
		return nil, err
	}
	result := &v2model.EditResult{Created: resolvers.created(), DiffStats: stats}
	if len(applier.createdBlocks) > 0 {
		result.CreatedBlocks = applier.createdBlocks
	}
	return result, nil
}

// PutObject implements PUT /v2/spaces/{spaceId}/objects/{objectId}: the body
// is a full AnyBlock document that replaces the object's content in one
// change set. Minimal CRDT diff iff the block ids round-trip from the GET
// (they do on default reads, C4); diffStats make an accidental full rewrite
// visible.
func (s *V2Service) PutObject(ctx context.Context, spaceId, objectId string, body []byte, ifMatch string, dryRun bool) (*v2model.EditResult, error) {
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
		return nil, v2model.ValidationFailed("decode document envelope: " + err.Error())
	}
	// the R9 referential layer guards PUT like create: kind gating, type and
	// property keys with did-you-mean, items-on-collections
	if err := s.validateDocumentRefs(spaceId, &envelope, docCreateOptions{}); err != nil {
		return nil, err
	}

	var result *v2model.EditResult
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

// runEdit executes a PUT build: through the mutator's reset path (one locked
// diff-apply) on a real run, through a plain read on a dry run (C9 — nothing
// is committed, the would-be outcome rides the result).
func (s *V2Service) runEdit(ctx context.Context, spaceId, objectId string, dryRun bool, build func(apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error), result **v2model.EditResult) (*v2model.EditResult, error) {
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
	heads, err := s.mutator.ResetObject(ctx, spaceId, objectId, build)
	if err != nil {
		var v2Err *v2model.Error
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
		return nil, v2model.ValidationFailed("the PATCH body must be a JSON object",
			v2model.Issue{Message: err.Error(), Hint: `send {"ops": [...]} — GET /v2/schemas/ops/{op} documents each op`})
	}
	for key := range fields {
		if key != "ops" {
			return nil, v2model.ValidationFailed("unknown field in PATCH body",
				v2model.Issue{Path: "/" + key, Message: fmt.Sprintf("unknown key %q — the PATCH body carries only ops", key), Hint: "the If-Match precondition is a header, not a body field (C7)"})
		}
	}
	var req v2PatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, v2model.ValidationFailed("decode ops: " + err.Error())
	}
	if len(req.Ops) == 0 {
		return nil, v2model.ValidationFailed("ops must not be empty",
			v2model.Issue{Path: "/ops", Message: "give at least one op", Hint: "allowed ops: " + joinOpNames()})
	}
	// bound the batch: every op re-renders the document view under the object
	// lock, so an unbounded batch is a self-inflicted DoS (review A′2/B6).
	if len(req.Ops) > v2MaxOpsPerPatch {
		return nil, v2model.ValidationFailed("too many ops in one PATCH",
			v2model.Issue{
				Path:    "/ops",
				Message: fmt.Sprintf("%d ops exceeds the %d-op limit", len(req.Ops), v2MaxOpsPerPatch),
				Hint:    "split the edit across several PATCH requests",
			})
	}
	return req.Ops, nil
}

// putPipeline runs the PUT against one consistent read.
func (s *V2Service) putPipeline(ctx context.Context, spaceId, objectId string, body []byte, envelope docEnvelope, ifMatch string, dryRun bool, cur apicore.ObjectRead) (*model.SmartBlockSnapshotBase, *v2model.EditResult, error) {
	if err := checkEditPreconditions(cur.SbType, cur.Heads, ifMatch); err != nil {
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
func (s *V2Service) finishEdit(ctx context.Context, spaceId string, targetDoc, beforeDoc []byte, dryRun bool) (*model.SmartBlockSnapshotBase, *v2model.EditResult, error) {
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
	return snapshot, &v2model.EditResult{Created: resolvers.created(), DiffStats: stats}, nil
}

// checkEditPreconditions applies the C7 If-Match check against the live
// heads (advisory: absent = last-write-wins) and the canUpdateObject system
// exclusions (system-managed smartblock types are not editable through the
// generic object surface).
func checkEditPreconditions(sbType model.SmartBlockType, heads []string, ifMatch string) error {
	switch sbType {
	case model.SmartBlockType_STRelation, model.SmartBlockType_STRelationOption,
		model.SmartBlockType_FileObject, model.SmartBlockType_Participant:
		return v2model.ValidationFailed(
			fmt.Sprintf("this object is system-managed (%s) and cannot be edited through the object surface", sbType.String()),
			v2model.Issue{Message: "properties, types and files have their own endpoints"})
	}
	if !EtagMatches(ifMatch, heads) {
		return v2model.EtagMismatch(ComputeEtag(heads))
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
	var warnings []v2model.Issue
	opts.OnWarning = func(iss anyblockjson.Issue) {
		warnings = append(warnings, v2model.Issue{Path: iss.Path, Message: iss.Message})
	}
	doc, err := anyblockjson.Marshal(cur.SbType, cur.Snapshot, opts)
	if err != nil {
		return nil, fmt.Errorf("marshal object %s: %w", objectId, err)
	}
	if guardWarnings && len(warnings) > 0 {
		return nil, v2model.NewError(http.StatusUnprocessableEntity, v2model.CodeValidationFailed,
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
		return nil, v2model.ValidationFailed("the PUT body must be a full AnyBlock document",
			v2model.Issue{Message: err.Error()})
	}
	delete(fields, "etag") // C7: preconditions ride the If-Match header only
	delete(fields, "warnings")
	if raw, ok := fields["id"]; ok {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil && id != "" && id != objectId {
			return nil, v2model.ValidationFailed("the document id does not match the addressed object",
				v2model.Issue{Path: "/id", Message: fmt.Sprintf("got %q, the URL addresses %q — omit id or repeat the addressed one", id, objectId)})
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

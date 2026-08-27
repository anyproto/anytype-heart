package v2service

// edit.go implements the Phase-3 edit surface (APIV2.md §2 Phase 3):
// PATCH /v2/spaces/{space_id}/objects/{object_id} — the batched, atomic,
// id-addressed op set (ops.go). It is the ONLY edit surface: SNAPSHOTS ARE
// FOR CREATES, EDITS ARE OPS (APIV2.md §8.27). The full-document PUT that
// once sat beside it was removed with its whole pipeline — a surface that
// makes a caller materialize the whole document to change part of it pays
// whole-document tokens both ways and takes block ids literally, which is
// where every id-identity defect of the hardening came from.
//
// PATCH applies the ops to a child *state.State of the live object
// (stateops.go) and the adapter commits it with ONE ordinary sb.Apply —
// the Block* RPC handler model. The flat document is still rendered under
// the lock, but only as the read-only view the ops address (refs, indents,
// error texts) and as the diff_stats input; nothing round-trips the whole
// document through Unmarshal. Create-missing option resolution runs BEFORE
// the object lock (resolver.go prewarm), so no create-RPC ever holds the
// lock. diff_stats come from the canonical before/after documents (diff.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2MaxOpsPerPatch bounds one PATCH batch. Each op re-renders the document
// view while the object lock is held, so an unbounded batch would hold the
// lock for O(ops × document) work (review A′2). The op count alone does not
// bound that product — the document factor is bounded separately by
// v2MaxPatchRenderWork.
const v2MaxOpsPerPatch = 512

// v2MaxPatchRenderWork bounds the marshal work one PATCH may do under the
// object lock, in block-renders: every view-rebuilding op (v2OpRebuildsView)
// forces the next op to re-marshal the WHOLE document, so a batch's worst
// case is rebuildingOps × (document blocks + payload blocks) block-renders.
// The 512-op cap bounds only the first factor (surface review M7): 400
// trivial replace_text ops on a 24,000-block document measured 71 s inside
// PatchObject (~7 µs per block-render on a desktop machine) — all of it
// while ObjectOpen, sync and every other RPC on the object wait. 2^20
// block-renders ≈ 7 s worst case; an over-bound batch is refused before any
// op applies, with the numbers, and splitting the edit across several PATCH
// requests releases the object between batches — same total work, no
// minutes-long lock hold.
const v2MaxPatchRenderWork = 1 << 20

// checkPatchRenderWork computes the worst-case block-render product of a
// batch against the document and refuses an over-bound batch whole, before
// any op applies — with the numbers, so the caller can size its batches.
// Payload blocks count into the document factor (an insert inflates what
// every later op re-renders); a markdown payload counts as the parsed-run
// cap, since parsing happens later.
//
// A dataview is ONE block whose marshal cost is O(views × columns) — the
// §8.19-B correction: counting it as one render let a fully legal
// 512×insert_view batch on a wide set hold the object lock for tens of
// seconds while scoring 0.05% of the budget. The document factor therefore
// counts dataview weight (per view: 1 + columns + sorts + filters), and
// every insert_view adds the document's heaviest per-view weight to the
// payload factor — the copy_from worst case. Ops that fail to probe
// contribute nothing — they fail in the applier, on their own op path,
// before any rebuild.
func checkPatchRenderWork(ops []json.RawMessage, blocks []map[string]any) error {
	docWork := len(blocks) + dataviewRenderWork(blocks)
	perViewWork := heaviestViewRenderWork(blocks)
	rebuilds, payload := 0, 0
	for _, raw := range ops {
		var probe struct {
			Op       string            `json:"op"`
			Blocks   []json.RawMessage `json:"blocks"`
			Markdown string            `json:"markdown"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if v2OpRebuildsView[probe.Op] {
			rebuilds++
		}
		payload += len(probe.Blocks)
		if probe.Markdown != "" {
			payload += v2MaxBlocksPerOp
		}
		if probe.Op == "insert_view" {
			payload += perViewWork
		}
	}
	work := rebuilds * (docWork + payload)
	if work <= v2MaxPatchRenderWork {
		return nil
	}
	return v2model.ValidationFailed("this PATCH is too much re-rendering work for one atomic batch",
		v2model.Issue{
			Path: "/ops",
			Message: fmt.Sprintf(
				"%d view-rebuilding ops each re-render the whole document (%d block-render units incl. dataview views×columns, %d more from payloads): ~%d block-renders exceeds the %d limit",
				rebuilds, docWork, payload, work, v2MaxPatchRenderWork),
			Hint: "split the edit across several smaller PATCH requests — the object is released between batches",
		})
}

// dataviewRenderWork counts the render weight dataview blocks add beyond
// their single block: one unit per view plus its columns, sorts and filters.
func dataviewRenderWork(blocks []map[string]any) int {
	work := 0
	for _, b := range blocks {
		if blockType(b) != "dataview" {
			continue
		}
		views, _ := b["views"].([]any)
		for _, raw := range views {
			work += viewRenderWork(raw)
		}
	}
	return work
}

// heaviestViewRenderWork is the largest single-view weight in the document —
// what one insert_view may add (its copy_from worst case).
func heaviestViewRenderWork(blocks []map[string]any) int {
	heaviest := 1
	for _, b := range blocks {
		if blockType(b) != "dataview" {
			continue
		}
		views, _ := b["views"].([]any)
		for _, raw := range views {
			if w := viewRenderWork(raw); w > heaviest {
				heaviest = w
			}
		}
	}
	return heaviest
}

func viewRenderWork(raw any) int {
	view, ok := raw.(map[string]any)
	if !ok {
		return 1
	}
	columns, _ := view["columns"].([]any)
	sorts, _ := view["sorts"].([]any)
	filters, _ := view["filters"].([]any)
	return 1 + len(columns) + len(sorts) + len(filters)
}

// v2PatchRequest is the PATCH body: the closed op list, nothing else.
type v2PatchRequest struct {
	Ops []json.RawMessage `json:"ops"`
}

// PatchObject implements PATCH /v2/spaces/{space_id}/objects/{object_id}: the
// ops apply to a child state of the live object, committed with one ordinary
// Apply (stateops.go).
func (s *Service) PatchObject(ctx context.Context, spaceId, objectId string, body []byte, ifMatch string, dryRun, createMissingOptions bool) (*v2model.EditResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	ops, err := parsePatchRequest(body)
	if err != nil {
		return nil, err
	}
	resolvers := s.newCreatingResolvers(ctx, spaceId, dryRun, createMissingOptions)

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
	// adapter would refuse (review C′3). Per-op, not per-request: a set and a
	// collection restrict Blocks but not Details, so a blanket check refused
	// renames and every add_items (surface review M1).
	needs, err := editNeedsForOps(ops, cur)
	if err != nil {
		return nil, err
	}
	if err := s.guardCreateMissing(ctx, spaceId, objectId, ops, ifMatch, cur, dryRun, createMissingOptions); err != nil {
		return nil, err
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
	heads, err := s.mutator.MutateObject(ctx, spaceId, objectId, needs, run)
	if err != nil {
		var v2Err *v2model.Error
		if errors.As(err, &v2Err) {
			return nil, v2Err
		}
		return nil, mapWriteError(spaceId, objectId, err)
	}
	result.Etag = ComputeEtag(heads)
	return result, nil
}

// v2MaxCreatedOptionsPerPatch bounds how many select/multiSelect options one
// PATCH may bring into existence. Create-missing is deliberate (SPEC §3:
// option NAMES are the identity, so an unknown name is created, not
// rejected), but it is also irreversible: options are objects, v2 has no
// option-delete surface, and they sync to every device. A batch naming
// dozens of genuinely new options is already extreme for a single object
// edit; thousands is a hallucinated array, not intent.
const v2MaxCreatedOptionsPerPatch = 64

// guardCreateMissing is the M5 prevention pass. Before this, one FAILING
// PATCH permanently created every option it named — 5,000 real objects from
// a ~60 KB body, ~10^6 at the body cap — because prewarm ran before the
// batch was known to be applicable and nothing bounded it.
//
// There is no transaction to lean on: options are objects, each its own CRDT
// tree, so "create N options and mutate a document" cannot be one commit.
// The irreversible part therefore goes LAST and SMALL, in two halves that
// catch different requests:
//
//  1. THE BOUND. Only this stops a *well-formed* batch — one that would
//     succeed — from creating a million options. Enforced on a probe pass
//     that resolves without creating, so the rejection costs nothing.
//  2. THE ORDERING. Only this stops a *failing* batch from leaving debris:
//     the whole batch is validated against a private state first, so an op
//     that cannot apply is discovered before any create RPC fires. This
//     subsumes case-by-case skip lists (a key claimed by both `set` and
//     `unset`, a scalar where a list is required, …) — enumerating the ways
//     a batch can fail is open-ended; validating it is not.
//
// The second half runs only when the batch actually names new options, so
// the ordinary PATCH pays one extra JSON walk and nothing more. Both halves
// run for dry runs too, so C9's preview reaches the same verdict.
//
// What remains after this is a crash or cancellation between the creates and
// the apply. That cannot be eliminated without a cross-object transaction,
// but it is now bounded by the cap, convergent on retry (OptionId resolves an
// existing option by name before creating, so a retry adopts what the first
// attempt made instead of duplicating it), and detectable — created options
// carry ObjectOrigin_api.
func (s *Service) guardCreateMissing(ctx context.Context, spaceId, objectId string, ops []json.RawMessage, ifMatch string, cur apicore.ObjectRead, dryRun, createMissingOptions bool) error {
	// a resolver in dry mode records would-be creations instead of performing
	// them: no RPCs, no document work, just a walk of the op payloads
	// the probe must see the SAME consent as the real run: it exists to
	// preview what that run would create, and a probe with a different answer
	// previews a different request
	probe := s.newCreatingResolvers(ctx, spaceId, true, createMissingOptions)
	s.prewarmCreateMissing(ops, probe)
	pending := probe.sideEffects.Options
	if len(pending) == 0 {
		return nil
	}
	if !createMissingOptions {
		// A2: refuse BEFORE the lock and before any RPC, naming every option
		// the batch would have minted rather than only the first — a caller
		// fixing typos one round trip at a time is the failure mode a
		// pre-lock guard exists to avoid. A dry run gets the same refusal:
		// its job is to preview what the real run would do.
		return optionConsentError(spaceId, pending[0].Property, pending[0].Name)
	}
	if len(pending) > v2MaxCreatedOptionsPerPatch {
		props := map[string]int{}
		for _, o := range pending {
			props[o.Property]++
		}
		issue := v2model.Issue{
			Path: "/ops",
			Message: fmt.Sprintf("this batch would create %d new options (limit %d): %s",
				len(pending), v2MaxCreatedOptionsPerPatch, describeCreateCounts(props)),
			Hint: "creating an option is permanent and there is no delete surface — " +
				"check the names against GET /v2/spaces/{space_id}/properties/{property_key}/options, " +
				"or set values in smaller batches if they are all genuinely new",
		}
		return v2model.ValidationFailed("too many new options in one request", issue)
	}
	if dryRun {
		// the caller's own run is already create-free; it reports the same
		// pending list, so re-validating here would only duplicate the work
		return nil
	}
	// ORDERING: prove the batch applies before anything is created. The probe
	// resolvers create nothing, so a failure here leaves the space untouched;
	// the error is the same one the real pass would raise, in the same order.
	edit, err := editFromRead(objectId, cur)
	if err != nil {
		return err
	}
	if _, err := s.applyPatchOps(ctx, spaceId, objectId, ops, ifMatch, edit, probe); err != nil {
		return err
	}
	return nil
}

// describeCreateCounts renders "status: 3, tag: 4997" for the over-limit
// message, so the caller can see which property the runaway array belongs to.
func describeCreateCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
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
// net, and the diff_stats. The caller commits (or, on dry run, discards) the
// state.
func (s *Service) applyPatchOps(ctx context.Context, spaceId, objectId string, ops []json.RawMessage, ifMatch string, edit apicore.ObjectEdit, resolvers *creatingResolvers) (*v2model.EditResult, error) {
	if err := checkEditPreconditions(edit.SbType, edit.Heads, ifMatch); err != nil {
		return nil, err
	}
	applier := newV2StateApplier(s, spaceId, objectId, edit.SbType, edit.State, resolvers)
	beforeDoc, err := applier.begin()
	if err != nil {
		return nil, err
	}
	// the M7 render-work bound, checked against the authoritative view the
	// begin() marshal just produced: refusing here costs one marshal — the
	// same floor a GET pays — instead of the batch's whole product
	if err := checkPatchRenderWork(ops, applier.view.blocks); err != nil {
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
	if len(applier.createdViews) > 0 {
		result.CreatedViews = applier.createdViews
	}
	result.Warnings = applier.warnings
	return result, nil
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

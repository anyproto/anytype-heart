package v2service

// stateops.go applies the PATCH ops (ops.go) to a child
// *state.State of the live object — the ordinary editor mutation model
// object state: ops are operations, not a document rewrite. The
// applier keeps a read-only "view" — the state marshaled to its flat
// AnyBlock document — for everything agent-facing: reference resolution
// (full ids and unique suffixes), indent/subtree arithmetic, error texts.
// Mutations then go to the state directly (st.Set / st.Add / st.InsertTo /
// st.Unlink / st.SetDetail / st.UpdateStoreSlice), and the adapter commits
// the state with ONE ordinary sb.Apply — per-block restriction checks, undo
// recording, hooks/events and the minimal id-matched change diff ride the
// normal editor path. Payload blocks are interpreted by the format package
// at fragment granularity (anyblockjson.UnmarshalBlocks/UnmarshalBlock),
// with the same validation a whole document gets.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// v2SkipPostOpValidate disables the post-op document validation. The check is
// ON by default (review B′3): the whole-document Validate the state pipeline
// replaced was load-bearing for invariants no single fragment can see — V3
// row→column containment, the document-wide id domain (derived rowId-colId
// cells, other tables' row/column ids), and the absolute nesting bound. The
// after-document is already marshaled for diff_stats, so the check is nearly
// free. The escape hatch exists only for debugging a suspected false
// rejection.
var v2SkipPostOpValidate = os.Getenv("ANYTYPE_API_V2_SKIP_EDIT_VALIDATE") == "1"

// v2InvalidDocMessage is the R5-net rejection message (kept verbatim from
// the document-level pipeline — it is part of the agent-facing contract).
const v2InvalidDocMessage = "the ops would produce an invalid document — no op was applied"

// v2StateApplier applies the ops of one PATCH to one child state.
type v2StateApplier struct {
	s        *Service
	spaceId  string
	objectId string
	// v is the request's error vocabulary (?keys — §4.3), captured at
	// construction: the op appliers run without a ctx of their own.
	v errKeys
	sbType   model.SmartBlockType
	st       *state.State

	resolvers *creatingResolvers

	// createdBlocks is the response id map, keyed by payload position
	// ("ops[3].blocks[0]") — minted ids and echoed client-supplied ones.
	createdBlocks map[string]string
	// claimedIds are ids earlier ops of this PATCH placed in the state, so a
	// later payload cannot silently collide with them; mintedThisOp are the
	// current op's own mints (fresh by construction, cleared per op).
	claimedIds   map[string]bool
	mintedThisOp map[string]bool
	// payloadIdOrigin maps a resolved stored id back to the shorter spelling
	// the caller wrote for it (payloadids.go), so a collision refusal can name
	// both. Cleared per op alongside mintedThisOp.
	payloadIdOrigin map[string]string

	// view is the state's flat-document rendering, rebuilt lazily after any
	// mutating op. Everything agent-facing (refs, indents, error texts) reads
	// it; nothing writes it.
	view *v2EditDoc
	// viewBody is the marshaled form the current view was parsed from, so the
	// caller can reuse it as the after-document instead of marshaling again
	// (review A′2).
	viewBody []byte
	// marshalResolver is the ONE store-backed resolver reused across every
	// marshal of this PATCH (review A′2); marshalKeys is its slug vocabulary
	// (apikeyvocab.go), cached beside it for the same reason.
	marshalResolver *storeresolver.Resolvers
	marshalKeys     *apiKeyVocab
	// liveEntries is the ONE live-property snapshot shared by every key
	// check of this PATCH (§7.5a-2: one bounded query per request, never
	// one per reference), primed lazily by propEntries.
	liveEntries       []propertyEntry
	liveEntriesErr    error
	liveEntriesLoaded bool
	// removedBundled is the same shape for the bundled relations this space
	// uninstalled — primed lazily by removedBundledKeys, and only when a key
	// reaches the bundled arm at all.
	removedBundled       map[string]bool
	removedBundledErr    error
	removedBundledLoaded bool
	// marshalCount counts whole-document renders (marshalDoc calls) — the
	// unit the M7 render-work bound is denominated in. Tests pin it so an op
	// that claims to keep the view valid in place (v2OpRebuildsView false)
	// cannot silently regress to a re-marshal per op.
	marshalCount int

	// warnings collects C11 warning-grade findings the ops raise (today: the
	// §6.2 unguarded-date-comparison trap from an update_view filter edit);
	// applyPatchOps surfaces them on the EditResult.
	warnings []v2model.Issue

	// createdViews maps each insert_view op ("ops[i]") to the view id it
	// minted — the view-family twin of createdBlocks (ids are always
	// server-minted; a view payload has no id slot).
	createdViews map[string]string
}

func newV2StateApplier(s *Service, spaceId, objectId string, sbType model.SmartBlockType, st *state.State, resolvers *creatingResolvers, v errKeys) *v2StateApplier {
	return &v2StateApplier{
		s:               s,
		v:               v,
		spaceId:         spaceId,
		objectId:        objectId,
		sbType:          sbType,
		st:              st,
		resolvers:       resolvers,
		createdBlocks:   map[string]string{},
		claimedIds:      map[string]bool{},
		mintedThisOp:    map[string]bool{},
		payloadIdOrigin: map[string]string{},
		createdViews:    map[string]string{},
	}
}

// snapshotFromState renders a state as a marshal-ready snapshot. Used
// synchronously under the object lock (or on a dry-run's private state), so
// no defensive copies are needed.
func snapshotFromState(st *state.State) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks:      st.BlocksToSave(),
		Details:     st.CombinedDetails().ToProto(),
		ObjectTypes: domain.MarshalTypeKeys(st.ObjectTypeKeys()),
		Collections: st.Store(),
		Key:         st.UniqueKeyInternal(),
	}
}

// marshalOptions returns the applier's export options, reusing ONE
// storeresolver for the whole PATCH. A fresh resolver per marshal would start
// with empty caches and re-run ListAllRelations/ListRelationOptions against
// the space index on every op — under the object lock (review A′2).
func (a *v2StateApplier) marshalOptions() anyblockjson.Options {
	if a.marshalResolver == nil {
		a.marshalResolver = storeresolver.New(a.s.store.SpaceIndex(a.spaceId))
		// D1/§4.2: the applier's whole RMW cycle — after-documents, view
		// rebuilds, receipts — is pinned to the slug vocabulary, so what a
		// view op compares against is what the read served
		a.marshalKeys = a.s.apiKeys(a.spaceId, a.marshalResolver)
	}
	opts := a.marshalResolver.Options()
	opts.Keys = a.marshalKeys
	return opts
}

// marshalDoc renders the current state as its canonical flat document.
//
// With a nil sink (view rebuilds and the after-document) a marshal warning is
// an ERROR, not something to swallow (review B′2). The exporter degrades
// rather than failing when a sink is installed — it clamps an over-deep
// indent — so a silently-clamped view made later ops in the same batch read
// wrong depths: delete_block saw `descendants == 0`, skipped the recursive
// guard and dropped a whole subtree, and the clamped after-document then
// validated clean. Failing here rejects the whole PATCH instead.
func (a *v2StateApplier) marshalDoc(onWarning func(anyblockjson.Issue)) ([]byte, error) {
	a.marshalCount++
	opts := a.marshalOptions()
	var degraded []v2model.Issue
	if onWarning == nil {
		onWarning = func(iss anyblockjson.Issue) {
			if isPatchLossWarning(iss) {
				degraded = append(degraded, v2model.Issue{Path: iss.Path, Message: iss.Message})
			}
		}
	}
	opts.OnWarning = onWarning
	doc, err := anyblockjson.Marshal(a.sbType, snapshotFromState(a.st), opts)
	if err != nil {
		return nil, fmt.Errorf("marshal object %s: %w", a.objectId, err)
	}
	if len(degraded) > 0 {
		return nil, v2model.NewError(http.StatusBadRequest, v2model.CodeValidationFailed,
			v2InvalidDocMessage, degraded...)
	}
	return doc, nil
}

// begin renders the before-document with the C11 write-safety guard: any
// marshal loss warning refuses the PATCH — otherwise content the format
// cannot represent could be silently damaged by a later full-subtree op.
func (a *v2StateApplier) begin() ([]byte, error) {
	var warnings []v2model.Issue
	doc, err := a.marshalDoc(func(iss anyblockjson.Issue) {
		if isPatchLossWarning(iss) {
			warnings = append(warnings, v2model.Issue{Path: iss.Path, Message: iss.Message})
		}
	})
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		return nil, v2model.NewError(http.StatusUnprocessableEntity, v2model.CodeValidationFailed,
			"this object contains content the AnyBlock format cannot fully represent — a PATCH would drop it (C11); edit it in the app",
			warnings...)
	}
	// seed the view from the document we just rendered: without this the first
	// op re-marshals immediately, so even a 1-op PATCH marshaled three times
	// (review A′2).
	if err := a.seedView(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// isPatchLossWarning separates actual representation loss from exporter
// diagnostics that still round-trip the stored value. Legend-admission
// warnings say the term is written verbatim and remains its own address; an
// editor-state PATCH does not rebuild Details from that legend, so treating
// them as C11 loss falsely makes fresh collections uneditable.
func isPatchLossWarning(iss anyblockjson.Issue) bool {
	if iss.Path == "/property_internal_keys" || iss.Path == "/type_internal_keys" {
		return false
	}
	// The date exporter deliberately falls back to the original raw number
	// when no RFC 3339 spelling exists; the importer accepts that same number.
	// It is suspicious input worth surfacing on reads, but it is lossless and
	// therefore cannot justify a C11 write refusal.
	const rawDateSuffix = " has no RFC 3339 form (outside years 0000-9999), so it is written as a raw number; " +
		"a value this large is usually milliseconds where seconds belong"
	return !(strings.HasPrefix(iss.Path, "/properties/") &&
		strings.HasPrefix(iss.Message, "date ") && strings.HasSuffix(iss.Message, rawDateSuffix))
}

// seedView installs an already-marshaled document as the current view.
func (a *v2StateApplier) seedView(body []byte) error {
	view, err := parseEditDoc(body)
	if err != nil {
		return fmt.Errorf("object %s: %w", a.objectId, err)
	}
	a.view, a.viewBody = view, body
	return nil
}

// doc returns the current view, rebuilding it when a mutation invalidated it.
func (a *v2StateApplier) doc() (*v2EditDoc, error) {
	if a.view != nil {
		return a.view, nil
	}
	body, err := a.marshalDoc(nil)
	if err != nil {
		return nil, err
	}
	if err := a.seedView(body); err != nil {
		return nil, err
	}
	return a.view, nil
}

// currentDoc returns the marshaled form of the current state, reusing the
// view's body when it is still valid (review A′2: the after-document for
// diff_stats is the view we already rendered).
func (a *v2StateApplier) currentDoc() ([]byte, error) {
	if a.viewBody != nil {
		return a.viewBody, nil
	}
	body, err := a.marshalDoc(nil)
	if err != nil {
		return nil, err
	}
	if err := a.seedView(body); err != nil {
		return nil, err
	}
	return a.viewBody, nil
}

// mutated invalidates the view after a state mutation.
func (a *v2StateApplier) mutated() {
	a.view, a.viewBody = nil, nil
}

// textEdited invalidates only the marshaled body after an in-place view
// update (surface review M7): the caller has already written the CANONICAL
// exported form of everything it changed into the view, so later ops keep
// addressing a valid view without a whole-document re-marshal — the O(ops ×
// document) product this op class no longer pays. The body is dropped
// because diff_stats and the R5 net need the real after-document; currentDoc
// re-marshals once, at the end. An op may only call this instead of
// mutated() if its view update is byte-identical to a re-marshal — and must
// then be listed false in v2OpRebuildsView.
func (a *v2StateApplier) textEdited() {
	a.viewBody = nil
}

// importOptions are the fragment-import options: the create-missing
// resolvers plus this applier's id minting (editor-shaped, collision-free).
func (a *v2StateApplier) importOptions() anyblockjson.Options {
	opts := a.resolvers.Options()
	opts.GenerateId = a.mintBlockId
	return opts
}

// commitImportOptions are importOptions with NO-CREATE option resolution —
// the whole-dataview re-import at a view op's commit (§8.19-A): op-authored
// names were created by the pre-lock prewarm and resolve from its cache;
// anything else misses read-only and passes through verbatim instead of
// minting an option under the object lock.
func (a *v2StateApplier) commitImportOptions() anyblockjson.Options {
	opts := a.importOptions()
	opts.ResolveOptions = readOnlyOptionResolver{r: a.resolvers}
	return opts
}

// mintBlockId mints an editor-shaped 24-hex block id unused anywhere in the
// state or this PATCH.
func (a *v2StateApplier) mintBlockId() string {
	for {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(fmt.Errorf("random block id: %w", err))
		}
		id := hex.EncodeToString(b[:])
		if !a.claimedIds[id] && !a.mintedThisOp[id] && !a.st.Exists(id) {
			a.mintedThisOp[id] = true
			return id
		}
	}
}

// invalidDocError maps a fragment validation failure to the R5-net contract:
// the whole PATCH is rejected with the format's path-addressed issues under
// the unchanged agent-facing message.
func invalidDocError(err error) error {
	verr := mapUnmarshalError(nil, err)
	var v2Err *v2model.Error
	if errors.As(verr, &v2Err) && v2Err.Code == v2model.CodeValidationFailed {
		v2Err.Message = v2InvalidDocMessage
		return v2Err
	}
	return verr
}

// invalidFragmentError is invalidDocError for a payload fragment: it rebases
// the format's run-relative issue paths onto the op that carried them, so the
// repair loop can locate the failing op (review C′1). Without this a 4-op
// batch reported every payload problem as "/blocks/0/type" — indistinguishable
// between ops. base is the op's payload path ("ops[2].blocks", "ops[2].block",
// "ops[2].set"); a run-relative "/blocks/3/text" becomes "ops[2].blocks[3].text".
func invalidFragmentError(base string, err error) error {
	rebased := invalidDocError(err)
	var v2Err *v2model.Error
	if !errors.As(rebased, &v2Err) {
		return rebased
	}
	for i := range v2Err.Issues {
		v2Err.Issues[i].Path = rebaseFragmentPath(base, v2Err.Issues[i].Path)
	}
	return v2Err
}

// rebaseFragmentPath maps a run-relative issue path onto the op's payload.
func rebaseFragmentPath(base, path string) string {
	if path == "" {
		return base
	}
	// the fragment is validated inside a synthetic document, so paths arrive as
	// /blocks/<j>/<rest>; keep the index with the op's payload path
	if rest, ok := strings.CutPrefix(path, "/blocks/"); ok {
		idx, tail, found := strings.Cut(rest, "/")
		if found {
			return fmt.Sprintf("%s[%s].%s", base, idx, strings.ReplaceAll(tail, "/", "."))
		}
		return fmt.Sprintf("%s[%s]", base, idx)
	}
	return base + strings.ReplaceAll(path, "/", ".")
}

// validateEditedDoc is the R5 whole-document net, restored (review B′3):
// fragment validation only sees a payload run in isolation, so invariants
// that span the spliced result — V3 row→column containment, the
// document-wide id domain, the absolute nesting bound — are checked here on
// the would-be document. A failure rejects the whole PATCH under the
// unchanged agent-facing message; nothing is applied.
func validateEditedDoc(objectId string, doc []byte, createdBlocks map[string]string) error {
	if v2SkipPostOpValidate {
		return nil
	}
	if err := anyblockjson.Validate(doc); err != nil {
		log.Warnf("api v2 edit: object %s post-op document fails validation: %v", objectId, err)
		return rebaseEditedDocError(doc, createdBlocks, invalidDocError(err))
	}
	return nil
}

// rebaseEditedDocError translates a final-document /blocks/<n> path back to
// the PATCH payload position that introduced that block. Post-op validation
// necessarily runs over the complete would-be document, but its errors must
// still address the request the caller can repair.
func rebaseEditedDocError(doc []byte, createdBlocks map[string]string, err error) error {
	if len(createdBlocks) == 0 {
		return err
	}
	var envelope struct {
		Blocks []struct {
			Id string `json:"id"`
		} `json:"blocks"`
	}
	if json.Unmarshal(doc, &envelope) != nil {
		return err
	}
	originById := make(map[string]string, len(createdBlocks))
	for origin, id := range createdBlocks {
		originById[id] = origin
	}
	var v2Err *v2model.Error
	if !errors.As(err, &v2Err) {
		return err
	}
	for i := range v2Err.Issues {
		path := v2Err.Issues[i].Path
		rest, ok := strings.CutPrefix(path, "/blocks/")
		if !ok {
			continue
		}
		indexText, tail, _ := strings.Cut(rest, "/")
		index, convErr := strconv.Atoi(indexText)
		if convErr != nil || index < 0 || index >= len(envelope.Blocks) {
			continue
		}
		origin, ok := originById[envelope.Blocks[index].Id]
		if !ok {
			continue
		}
		v2Err.Issues[i].Path = rebaseSlashPath(origin, tail)
	}
	return v2Err
}

//
// ---- op dispatch ----
//

// apply dispatches one op. i is the op's position (error paths are
// "ops[i]…", R5).
func (a *v2StateApplier) apply(i int, raw json.RawMessage) error {
	opPath := fmt.Sprintf("ops[%d]", i)
	// per-op scratch: which payload ids this op resolved from a shorter
	// spelling. Reset here rather than in claimPayloadIds so an op that
	// never reaches the collision guard cannot leave an origin behind for
	// the next op's error message.
	a.payloadIdOrigin = map[string]string{}
	var probe struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return v2model.ValidationFailed("each op must be a JSON object",
			v2model.Issue{Path: opPath, Message: err.Error()})
	}
	switch probe.Op {
	case "set_properties":
		var op opSetProperties
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applySetProperties(op, opPath)
	case "update_block":
		var op opUpdateBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyUpdateBlock(op, opPath)
	case "replace_subtree":
		var op opReplaceSubtree
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyReplaceSubtree(op, opPath)
	case "insert_blocks":
		var op opInsertBlocks
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyInsertBlocks(op, opPath)
	case "move_block":
		var op opMoveBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyMoveBlock(op, opPath)
	case "delete_block":
		var op opDeleteBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyDeleteBlock(op, opPath)
	case "replace_text":
		var op opReplaceText
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyReplaceText(op, opPath)
	case "set_cell":
		var op opSetCell
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applySetCell(op, opPath)
	case "update_view":
		var op opUpdateView
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyUpdateView(op, opPath)
	case "insert_view":
		var op opInsertView
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyInsertView(op, opPath)
	case "move_view":
		var op opMoveView
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyMoveView(op, opPath)
	case "delete_view":
		var op opDeleteView
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyDeleteView(op, opPath)
	case "add_items", "remove_items":
		var op opItems
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyItems(op, opPath)
	case "":
		return v2model.ValidationFailed("op is required",
			v2model.Issue{Path: opPath + ".op", Message: "missing op", Hint: "allowed ops: " + strings.Join(v2OpNames, ", ")})
	default:
		return v2model.ValidationFailed(fmt.Sprintf("unknown op %q", probe.Op),
			v2model.Issue{Path: opPath + ".op", Message: fmt.Sprintf("unknown op %q", probe.Op), Hint: "allowed ops: " + strings.Join(v2OpNames, ", ")})
	}
}

// resolveSubject resolves the block an op acts on from its TWO alternative
// addressing channels: `id` (a full id or unique suffix,
// C4) or `match` (an exact substring of the block's text, which must
// identify exactly one block — locator.go). It returns the view index.
//
// Giving BOTH is refused, not ranked. `match` has exactly one job —
// addressing — so a precedence rule would make one of the two fields
// silently inert, which is the failure shape this surface has spent its
// review rounds removing; and reading the loser as a content precondition
// instead would be a second meaning for `match` invented at the point of
// conflict. (replace_text is not the counter-example: its `find` is the text
// to splice FIRST and the locator only when `id` is absent, so there is
// nothing to rank there either.) Giving neither is refused too — an
// unaddressed block op has no subject, and the empty-string id it used to
// send produced the misleading `block "" not found`.
//
// The both/neither pair reuses insertPayload's shipped vocabulary for
// alternative channels: ambiguous_input for both, validation_failed for
// neither.
func (a *v2StateApplier) resolveSubject(doc *v2EditDoc, op, id, match, opPath string, scope locatorScope) (int, error) {
	switch {
	case id != "" && match != "":
		return -1, v2model.AmbiguousInput(fmt.Sprintf("address the block with id or match, not both (%s)", op),
			v2model.Issue{
				Path:    opPath,
				Message: "id names the block directly; match locates it by its text — they are alternative addressing channels",
				Hint:    "drop match to address by id, or drop id to let the text locate the block",
			})
	case match != "":
		// the locator scans the applier's LIVE view, per op and under the
		// object lock, so op i addresses the document op i−1 left
		return resolveByText(doc, match, "match", opPath+".match", scope)
	case id != "":
		return a.resolveRef(doc, id, opPath+".id")
	default:
		return -1, v2model.ValidationFailed(fmt.Sprintf("%s needs a block to address", op),
			v2model.Issue{
				Path:    opPath,
				Message: "give id (a full block id or a unique suffix) or match (exact text from the block, which must appear in exactly one)",
			})
	}
}

// resolveRef resolves a block reference (full id or unique suffix, C4/§9a)
// against the view, with op-scoped errors.
func (a *v2StateApplier) resolveRef(doc *v2EditDoc, ref, path string) (int, error) {
	idx, matches := matchBlockRef(doc.blockIds(), ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("block reference %q matches more than one block — use the full block id", ref),
			v2model.Issue{Path: path, Message: "the reference is a suffix of several block ids"})
	default:
		return -1, v2model.NotFound(fmt.Sprintf("block %q not found", ref),
			v2model.Issue{Path: path, Message: v2AddressableBlocksMessage, Hint: v2AddressableBlocksHint})
	}
}

//
// ---- state splice helpers ----
//

// setBlocks lands fragment blocks in the state: reused ids are overwritten
// in place, fresh ids are added. The caller wires positions.
func (a *v2StateApplier) setBlocks(blocks []*model.Block) {
	for _, b := range blocks {
		if a.st.Exists(b.Id) {
			a.st.Set(simple.New(b))
		} else {
			a.st.Add(simple.New(b))
		}
	}
}

// claimPayloadIds rejects payload ids (table internals and dataview VIEW ids
// included) that already exist in the object or in this PATCH, except ids the
// op explicitly replaces (allowed) and ids the op itself minted (fresh by
// construction). On success every payload id is claimed for the rest of the
// PATCH.
//
// This is the collision half of the old checkFreshIds. Its other half — a
// scan for ids that merely TAIL a block the document keeps — is gone,
// subsumed by payload id resolution (payloadids.go): a tailing id no longer
// reaches the state as a literal, it resolves to the block it labels (or
// refuses as ambiguous/unresolvable) before the fragment import runs. What
// survives here is the case resolution cannot decide: an id that legitimately
// names an existing element, in an op that may not reuse it — insert_blocks
// naming a live block, replace_subtree naming one outside the subtree it
// replaces. Keeping only one guard is deliberate: the two overlapped, with
// different coverage, and the gap between them was F1.
//
// The two halves of the id rule must agree on the DOMAIN, and this is where
// they used to disagree (§8.31). Resolution's domain is the pre-op document's
// localIds() — blocks, table internals, cell descendants AND views — while
// this guard asked a.st.Exists, which walks the imported []*model.Block and
// in which a view is not an element at all. So a view id resolved but could
// never collide: two views could be stored under one id, and a block could
// adopt a live view's id. Both halves now ask payloadIdExists, and views are
// claimed alongside blocks.
//
// A view id is unique within its DATAVIEW BLOCK, not document-wide (SPEC
// §6.2, and matchViewRef resolves a view reference against one dataview's
// views) — so the intra-payload duplicate scan for views is per block, while
// block-shaped ids share one document-wide scan.
func (a *v2StateApplier) claimPayloadIds(blocks []*model.Block, allowed map[string]bool, pathFor func(id string) string) error {
	exists, err := a.payloadIdExists()
	if err != nil {
		return err
	}
	var claim []string
	// take checks one id slot. dup says the same payload already used this id
	// in the scope the id has to be unique in — a duplicate is refused even
	// when the id is one the op may reuse, because "may reuse it" licenses
	// ONE holder, not two.
	take := func(id, path string, dup bool) error {
		if id == "" {
			return nil
		}
		if dup || (!allowed[id] && !a.mintedThisOp[id] && (exists(id) || a.claimedIds[id])) {
			return a.duplicateIdError(path, id)
		}
		claim = append(claim, id)
		return nil
	}
	seen := map[string]bool{} // document-wide: block, row, column, cell ids
	for _, b := range blocks {
		base := pathFor(b.Id)
		if err := take(b.Id, base, seen[b.Id]); err != nil {
			return err
		}
		seen[b.Id] = true
		viewSeen := map[string]bool{} // per dataview block
		for j, v := range b.GetDataview().GetViews() {
			id := v.GetId()
			if err := take(id, viewIdPath(base, j), viewSeen[id]); err != nil {
				return err
			}
			viewSeen[id] = true
		}
	}
	for _, id := range claim {
		a.claimedIds[id] = true
	}
	a.mintedThisOp = map[string]bool{}
	return nil
}

// runPathFor reports a duplicate id under its payload position when the id
// is a run top block, or the op's payload list otherwise (table internals).
func runPathFor(opPath, field string, topIds []string) func(id string) string {
	topIndex := map[string]int{}
	for j, id := range topIds {
		topIndex[id] = j
	}
	return func(id string) string {
		return fmt.Sprintf("%s.%s[%d].id", opPath, field, topIndex[id])
	}
}

// collectSubtreeIds gathers a block, all its descendants, and the doc-local
// ids those blocks OWN without being blocks themselves: a dataview block's
// VIEW ids. It is the "ids this op may reuse" set, so it has to name every
// identity the op is replacing — and a dataview's views go with the block,
// so echoing them back has to keep them (§8.31).
func collectSubtreeIds(st *state.State, id string) map[string]bool {
	out := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		if visited[cur] {
			return
		}
		visited[cur] = true
		out[cur] = true
		b := st.Pick(cur)
		if b == nil {
			return
		}
		for _, v := range b.Model().GetDataview().GetViews() {
			if v.GetId() != "" {
				out[v.GetId()] = true
			}
		}
		for _, child := range b.Model().ChildrenIds {
			walk(child)
		}
	}
	walk(id)
	return out
}

// insertAt links ids as children of parentId at index (the public-API way:
// InsertTo relative to a sibling, or Inner into an empty parent).
func (a *v2StateApplier) insertAt(parentId string, index int, ids []string) error {
	parent := a.st.Pick(parentId)
	if parent == nil {
		return fmt.Errorf("insert parent %s not found", parentId)
	}
	children := parent.Model().ChildrenIds
	switch {
	case index > 0 && index <= len(children):
		return a.st.InsertTo(children[index-1], model.Block_Bottom, ids...)
	case len(children) > 0:
		return a.st.InsertTo(children[0], model.Block_Top, ids...)
	default:
		return a.st.InsertTo(parentId, model.Block_Inner, ids...)
	}
}

// targetPosition maps the R14 targeting vocabulary to the state's insert
// positions. Note the state's naming: Block_Inner appends (last child),
// Block_InnerFirst prepends (first child). Root mode never reaches here —
// rootTarget resolves it, because the document's two ends are not the state
// root's two ends (§8.32).
func targetPosition(mode, pos string) model.BlockPosition {
	switch mode {
	case "before":
		return model.Block_Top
	case "after":
		return model.Block_Bottom
	}
	if pos == "first" {
		return model.Block_InnerFirst
	}
	return model.Block_Inner
}

// rootTarget resolves root mode — no after/before/inside — into an InsertTo
// anchor. `last` (the default) is the empty anchor: InsertTo reads that as
// the document root and appends at its end.
//
// `first` is NOT the root's Block_InnerFirst. The state root's first child is
// the structural header — title, description, featuredProperties — which SPEC
// §7 keeps OUT of the served document, so prepending at the root would land
// the block above the title, which is not the start of the document. The
// start of the document is the slot `before: <first block>` names, and
// anchoring there needs no knowledge of the header at all.
//
// skipFrom/skipEnd exclude one subtree from the anchor search: move_block's
// own subtree, which InsertTo refuses as its own target. The next top-level
// block after it gives the same final order, and when there is none the two
// ends coincide and the append is correct.
func rootTarget(doc *v2EditDoc, pos string, skipFrom, skipEnd int) (anchorId string, position model.BlockPosition) {
	if pos != "first" {
		return "", model.Block_Inner
	}
	for i := 0; i < len(doc.blocks); i = doc.subtreeEnd(i) {
		if i >= skipFrom && i < skipEnd {
			continue
		}
		return blockId(doc.blocks[i]), model.Block_Top
	}
	// an empty document (or one that holds only the moved subtree): the start
	// and the end of the document are the same slot
	return "", model.Block_Inner
}

// fragmentBlocks converts a decoded payload run (relative indents, minted
// ids) into model blocks via the fragment import.
func (a *v2StateApplier) fragmentBlocks(base string, run []map[string]any) ([]*model.Block, []string, error) {
	raws := make([]json.RawMessage, len(run))
	for j, block := range run {
		raw, err := json.Marshal(block)
		if err != nil {
			return nil, nil, fmt.Errorf("encode payload block: %w", err)
		}
		raws[j] = raw
	}
	blocks, topIds, err := anyblockjson.UnmarshalBlocks(raws, a.importOptions())
	if err != nil {
		return nil, nil, invalidFragmentError(base, err)
	}
	return blocks, topIds, nil
}

// replaceLive swaps the addressed block's content for the fragment blocks,
// keeping its identity and position. Document children survive when both the
// old and the new shape carry them in the tree (everything except tables,
// whose children are format-internal, §6.1).
func (a *v2StateApplier) replaceLive(oldWasTable bool, blocks []*model.Block) {
	newRoot := blocks[0]
	_, newIsTable := newRoot.Content.(*model.BlockContentOfTable)
	if !oldWasTable && !newIsTable {
		if old := a.st.Pick(newRoot.Id); old != nil {
			newRoot.ChildrenIds = append([]string(nil), old.Model().ChildrenIds...)
		}
	}
	if oldWasTable && newIsTable {
		pinTableWrappers(a.st, blocks)
	}
	a.setBlocks(blocks)
	a.mutated()
}

// pinTableWrappers reuses the live table's column/row wrapper ids for the
// re-imported table. The format does not carry the wrappers (they are
// §6.1-internal), so the importer mints fresh ids for them — which turned
// every table op into "replace both wrappers, re-parent every row and
// column", and made concurrent cell edits on two devices merge into a table
// with duplicated rows and columns (review A′3).
func pinTableWrappers(st *state.State, blocks []*model.Block) {
	table := blocks[0]
	live := st.Pick(table.Id)
	if live == nil || len(live.Model().ChildrenIds) != len(table.ChildrenIds) {
		return
	}
	rename := map[string]string{}
	for i, newId := range table.ChildrenIds {
		liveId := live.Model().ChildrenIds[i]
		if newId != liveId && st.Pick(liveId) != nil {
			rename[newId] = liveId
		}
	}
	if len(rename) == 0 {
		return
	}
	for _, b := range blocks {
		if to, ok := rename[b.Id]; ok {
			b.Id = to
		}
		for i, child := range b.ChildrenIds {
			if to, ok := rename[child]; ok {
				b.ChildrenIds[i] = to
			}
		}
	}
}

//
// ---- set_properties ----
//

func (a *v2StateApplier) applySetProperties(op opSetProperties, opPath string) error {
	if len(op.Set) == 0 && len(op.Unset) == 0 && len(op.Add) == 0 && len(op.Remove) == 0 {
		return v2model.ValidationFailed("set_properties needs at least one of set, unset, add, remove",
			v2model.Issue{Path: opPath, Message: "set, unset, add and remove are all empty"})
	}
	// §7.5a-5: property terms may arrive as api-key slugs (a v2-created
	// property's stored key is BSON; its slug is its address) — canonicalize
	// to stored keys before validation and application. A term resolving to
	// nothing passes through verbatim (checkKey owns the did-you-mean);
	// ambiguity is a loud 400 here.
	spellings, err := a.canonicalizeSetPropertyKeys(&op, opPath)
	if err != nil {
		return err
	}
	spelledAs := func(key string) string {
		if original, ok := spellings[key]; ok {
			return original
		}
		return key
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	var issues []v2model.Issue
	var known []string

	// checkKey is the shared key validation (envelope keys, output-only,
	// unknown with did-you-mean); false = the key is unusable.
	checkKey := func(key, path string) bool {
		switch {
		case key == "id" || key == "type":
			issues = append(issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q is not a property — it is lifted to the document envelope and cannot be set here", key)})
			return false
		case v2OutputOnlyPropertyKeys(key):
			issues = append(issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only (SPEC §4a) — export writes it, writes must not", key)})
			return false
		default:
			entries, err := a.propEntries() // primed once per PATCH (§7.5a-2)
			if err != nil {
				issues = append(issues, v2model.Issue{Path: path, Message: err.Error()})
				return false
			}
			if _, inDoc := doc.properties[key]; !inDoc {
				if !propertyKeyExistsIn(entries, key) {
					if known == nil {
						known = knownPropertyKeysIn(entries, a.v)
					}
					issues = append(issues, unknownPropertyIssue(key, path, known,
						fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", a.spaceId, a.spaceId), a.v))
					return false
				}
				// the key exists only through the bundled table and this space
				// removed it — same refusal create makes, or PATCH would be the
				// hole create just closed
				refused, err := a.refusesRemovedBundled(entries, key)
				if err != nil {
					issues = append(issues, v2model.Issue{Path: path, Message: err.Error()})
					return false
				}
				if refused {
					issues = append(issues, removedPropertyIssue(a.spaceId, key, spelledAs(key), path, a.v))
					return false
				}
			}
		}
		return true
	}
	// claim enforces that a key appears in at most one of set/unset/add/
	// remove (v0.3.5); the fields are claimed in documentation order, so the
	// error names the earlier field.
	seenIn := map[string]string{}
	claim := func(key, field, path string) bool {
		if prev, ok := seenIn[key]; ok {
			issues = append(issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q appears in both %s and %s — pick one", key, prev, field)})
			return false
		}
		seenIn[key] = field
		return true
	}

	setKeys := sortedKeys(op.Set)
	for _, key := range setKeys {
		// issue paths spell the key as the CALLER sent it — canonicalization
		// already ran, and a path naming the rewrite is unactionable (§8.41-10)
		path := opPath + ".set." + spelledAs(key)
		claim(key, "set", path) // set goes first — it can never collide
		checkKey(key, path)
	}
	unset := map[string]bool{}
	for _, key := range op.Unset {
		path := opPath + ".unset." + spelledAs(key)
		if v2OutputOnlyPropertyKeys(key) {
			issues = append(issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only (SPEC §4a) and cannot be unset", key)})
			continue
		}
		if !claim(key, "unset", path) {
			continue
		}
		unset[key] = true
	}
	// add/remove get the same key validation as set, plus the list-shape
	// gate: they only apply to formats whose §3 encoding is a list.
	checkListField := func(field string, m map[string]json.RawMessage) map[string][]any {
		decoded := map[string][]any{}
		for _, key := range sortedKeys(m) {
			path := opPath + "." + field + "." + spelledAs(key)
			if !claim(key, field, path) || !checkKey(key, path) {
				continue
			}
			if format := a.propertyFormat(key); !v2ListShapedFormats[format] {
				issues = append(issues, v2model.Issue{Path: path,
					Message: fmt.Sprintf("%q has format %q — %s only applies to list-shaped formats (%s); use set", key, anyblockjson.FormatName(format), field, v2ListShapedFormatNames)})
				continue
			}
			var entries []any
			if err := decodeJSONUseNumber(m[key], &entries); err != nil {
				issues = append(issues, v2model.Issue{Path: path,
					Message: fmt.Sprintf("%s takes an array of entries (option names or object ids): %s", field, err.Error())})
				continue
			}
			decoded[key] = entries
		}
		return decoded
	}
	addEntries := checkListField("add", op.Add)
	removeEntries := checkListField("remove", op.Remove)
	if len(issues) > 0 {
		return v2model.ValidationFailed("set_properties rejected", issues...)
	}
	for _, key := range setKeys {
		var raw any
		if err := decodeJSONUseNumber(op.Set[key], &raw); err != nil {
			return v2model.ValidationFailed("invalid set value",
				v2model.Issue{Path: opPath + ".set." + key, Message: err.Error()})
		}
		value := anyblockjson.UnmarshalPropertyValue(key, raw, a.resolvers.Options())
		a.st.SetDetail(domain.RelationKey(key), domain.ValueFromProto(value))
		metadataKey, lexeme, exact := anyblockjson.ExactJSONIntegerMetadata(key, raw)
		a.st.RemoveDetail(domain.RelationKey(metadataKey))
		if exact {
			a.st.SetDetail(domain.RelationKey(metadataKey), domain.String(lexeme))
		}
		// a key new to the object needs its relation link, or the detail value
		// is wiped on replay (review A1 in miniature; AddRelationLinks dedupes)
		a.st.AddRelationLinks(&model.RelationLink{Key: key, Format: a.propertyFormat(key)})
	}
	for key := range unset {
		a.st.RemoveDetail(domain.RelationKey(key)) // unsetting an absent key is a no-op
		metadataKey, _, _ := anyblockjson.ExactJSONIntegerMetadata(key, nil)
		a.st.RemoveDetail(domain.RelationKey(metadataKey))
	}
	// add appends without duplicating an existing entry; entry resolution is
	// the same create-missing path as set (option names → ids, §3)
	for _, key := range sortedKeys(addEntries) {
		// select holds ONE value: appending to a non-empty one would leave a
		// two-valued single-select the UI renders arbitrarily. Steer to set
		// rather than silently breaking the invariant (v0.3.5 review).
		if a.propertyFormat(key) == model.RelationFormat_status &&
			len(a.st.CombinedDetails().Get(domain.RelationKey(key)).WrapToStringList()) > 0 {
			return v2model.ValidationFailed("add on a select property that already has a value",
				v2model.Issue{
					Path:    opPath + ".add." + key,
					Message: fmt.Sprintf("%q has format \"select\" and holds a single value", key),
					Hint:    "use set to replace it, or unset to clear it first",
				})
		}
		value := anyblockjson.UnmarshalPropertyValue(key, addEntries[key], a.resolvers.Options())
		toAdd := domain.ValueFromProto(value).WrapToStringList()
		current := a.st.CombinedDetails().Get(domain.RelationKey(key)).WrapToStringList()
		present := make(map[string]bool, len(current)+len(toAdd))
		merged := make([]string, 0, len(current)+len(toAdd))
		for _, lst := range [][]string{current, toAdd} {
			for _, id := range lst {
				if !present[id] {
					present[id] = true
					merged = append(merged, id)
				}
			}
		}
		a.st.SetDetail(domain.RelationKey(key), domain.StringList(merged))
		a.st.AddRelationLinks(&model.RelationLink{Key: key, Format: a.propertyFormat(key)})
	}
	// remove deletes matching entries and is a no-op when absent. Entry
	// resolution is READ-ONLY: a remove must never mint the very option it
	// names, and an unresolved name simply matches nothing. It is nevertheless
	// ambiguity-aware — duplicate labels cannot silently select the first
	// store entry here while set/add reject the same bare name.
	for _, key := range sortedKeys(removeEntries) {
		opts := a.marshalOptions()
		opts.ResolveOptions = ambiguityAwareReadOnlyOptionResolver{r: a.resolvers}
		value := anyblockjson.UnmarshalPropertyValue(key, removeEntries[key], opts)
		if err := a.resolvers.err(); err != nil {
			return err
		}
		if !a.st.CombinedDetails().Has(domain.RelationKey(key)) {
			continue // an absent key stays absent — remove never creates presence
		}
		drop := map[string]bool{}
		for _, id := range domain.ValueFromProto(value).WrapToStringList() {
			drop[id] = true
		}
		current := a.st.CombinedDetails().Get(domain.RelationKey(key)).WrapToStringList()
		kept := make([]string, 0, len(current))
		for _, id := range current {
			if !drop[id] {
				kept = append(kept, id)
			}
		}
		if len(current) > 0 && len(kept) == 0 {
			// Consuming the final entry has unset semantics. Preserve the
			// separate set:[] representation: removing from an already-empty
			// list consumes nothing and remains a no-op.
			a.st.RemoveDetail(domain.RelationKey(key))
		} else {
			a.st.SetDetail(domain.RelationKey(key), domain.StringList(kept))
		}
	}
	a.mutated()
	return nil
}

// propEntries primes the applier's live-property snapshot once. A load
// error fails the request (closed), never an empty-looking namespace.
func (a *v2StateApplier) propEntries() ([]propertyEntry, error) {
	if !a.liveEntriesLoaded {
		a.liveEntriesLoaded = true
		a.liveEntries, a.liveEntriesErr = a.s.liveProperties(a.spaceId)
	}
	return a.liveEntries, a.liveEntriesErr
}

// removedBundledKeys primes the applier's bundled-removal snapshot once
// (same one-query-per-request discipline as propEntries), and only when a
// key actually reaches the bundled arm. Fails closed on a load error: an
// unreadable removal set must not read as "nothing was removed".
func (a *v2StateApplier) removedBundledKeys() (map[string]bool, error) {
	if !a.removedBundledLoaded {
		a.removedBundledLoaded = true
		a.removedBundled, a.removedBundledErr = a.s.bundledRemovalSet(a.spaceId)
	}
	return a.removedBundled, a.removedBundledErr
}

// refusesRemovedBundled is the PATCH-side verdict: the key exists ONLY
// because the bundled table answers for it, and this space removed that
// bundled relation (uninstalled, archived, or sitting in the post-delete
// tombstone window — bundledPropertyRemoved covers all three shapes,
// §8.41). It is consulted AFTER the in-document escape — a removed
// property's existing values stay editable and removable, since unset is the
// one cleanup channel a caller has left; what this refuses is landing the
// key on a document that does not already carry it.
func (a *v2StateApplier) refusesRemovedBundled(entries []propertyEntry, key string) (bool, error) {
	if propertyKeyInstalledIn(entries, key) {
		return false, nil
	}
	removed, err := a.removedBundledKeys()
	if err != nil {
		return false, err
	}
	return a.s.bundledPropertyRemoved(a.resolvers.ctx, a.spaceId, entries, removed, key)
}

// canonicalizeSetPropertyKeys rewrites slug-spelled property terms in a
// set_properties op to their canonical stored keys (§7.5a-5). Rewrites apply
// only when the chain resolves to a DIFFERENT stored spelling; two spellings
// landing on one key is a loud 400, as is an ambiguous slug. The returned
// map remembers every rewrite (canonical → caller's spelling) so refusals
// raised after the rewrite can address the request as sent (§8.41-10).
func (a *v2StateApplier) canonicalizeSetPropertyKeys(op *opSetProperties, opPath string) (map[string]string, error) {
	spellings := map[string]string{}
	entries, err := a.propEntries()
	if err != nil {
		return nil, err
	}
	canon := func(key, path string) (string, error) {
		entry, ok, ambiguous := a.s.resolvePropertyInput(key, entries)
		if len(ambiguous) > 0 {
			return "", ambiguousKeyError("property key", key, path, ambiguous)
		}
		if ok && entry.Key != key {
			spellings[entry.Key] = key
			return entry.Key, nil
		}
		return key, nil
	}
	rewriteMap := func(m map[string]json.RawMessage, field string) (map[string]json.RawMessage, error) {
		if len(m) == 0 {
			return m, nil
		}
		out := make(map[string]json.RawMessage, len(m))
		for _, key := range sortedKeys(m) {
			path := opPath + "." + field + "." + key
			canonical, err := canon(key, path)
			if err != nil {
				return nil, err
			}
			if _, dup := out[canonical]; dup {
				return nil, v2model.ValidationFailed("duplicate property key",
					v2model.Issue{Path: path,
						Message: fmt.Sprintf("%q and another spelling both address property %q — keep one", key, canonical)})
			}
			out[canonical] = m[key]
		}
		return out, nil
	}
	if op.Set, err = rewriteMap(op.Set, "set"); err != nil {
		return nil, err
	}
	if op.Add, err = rewriteMap(op.Add, "add"); err != nil {
		return nil, err
	}
	if op.Remove, err = rewriteMap(op.Remove, "remove"); err != nil {
		return nil, err
	}
	for i, key := range op.Unset {
		canonical, err := canon(key, opPath+".unset."+key)
		if err != nil {
			return nil, err
		}
		op.Unset[i] = canonical
	}
	return spellings, nil
}

// propertyFormat resolves a property key's format for its relation link
// (bundle first, then the space, per §3).
func (a *v2StateApplier) propertyFormat(key string) model.RelationFormat {
	if format, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
		return format
	}
	if format, ok := a.resolvers.ResolveFormat(domain.RelationKey(key)); ok {
		return format
	}
	return model.RelationFormat_longtext
}

//
// ---- block ops ----
//

func (a *v2StateApplier) applyUpdateBlock(op opUpdateBlock, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	// `match` selects on the block's text as it stands when THIS op runs —
	// before its own set applies, which is the only coherent order (the block
	// has to be found before it can be changed) and is what makes
	// {"match":"Draft timeline","set":{"checked":true}} — §5.1's checkbox case
	// — and a match-then-rewrite rename expressible at all. Mid-batch the
	// same rule reads forward: op i matches the text op i−1 wrote, never the
	// text it overwrote (the view is rebuilt after every update_block).
	idx, err := a.resolveSubject(doc, "update_block", op.Id, op.Match, opPath, everyBlock)
	if err != nil {
		return err
	}
	if len(op.Set) == 0 {
		return v2model.ValidationFailed("update_block needs a non-empty set",
			v2model.Issue{Path: opPath + ".set", Message: "set is empty — name the fields to change (merge semantics: everything else stays)"})
	}
	block := doc.blocks[idx]
	oldWasTable := blockType(block) == "table"
	keys := make([]string, 0, len(op.Set))
	for key := range op.Set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := opPath + ".set." + key
		switch key {
		case "id":
			return v2model.ValidationFailed("block ids are immutable",
				v2model.Issue{Path: path, Message: "a block's id cannot change — insert a new block instead"})
		case "indent":
			return v2model.ValidationFailed("indent is structural",
				v2model.Issue{Path: path, Message: "indent cannot be set directly — use move_block to re-nest the block"})
		}
	}
	if raw, ok := op.Set["type"]; ok {
		var newType string
		if err := json.Unmarshal(raw, &newType); err != nil || newType == "" {
			return v2model.ValidationFailed("invalid type value",
				v2model.Issue{Path: opPath + ".set.type", Message: "type must be a non-empty string"})
		}
		oldType := blockType(block)
		if descendants := doc.subtreeEnd(idx) - idx - 1; descendants > 0 &&
			apiEditLeafBlockType(newType) &&
			!(apiFileFamilyBlockType(oldType) && apiFileFamilyBlockType(newType)) {
			return leafWithDescendantsError(blockId(block), newType, descendants, opPath+".set.type")
		}
	}
	// merge on the JSON shape: the view block IS the block's exported form
	merged := map[string]any{}
	for k, v := range block {
		merged[k] = v
	}
	delete(merged, "indent") // position is the tree's business
	// only the CALLER's arrays are resolved: the live fields merged in
	// already carry stored ids, and the vocabulary is loaded lazily so an
	// ordinary field edit pays nothing
	var vocab []string
	for _, key := range keys {
		var value any
		if err := decodeJSONUseNumber(op.Set[key], &value); err != nil {
			return v2model.ValidationFailed("invalid set value",
				v2model.Issue{Path: opPath + ".set." + key, Message: err.Error()})
		}
		if value == nil {
			delete(merged, key) // explicit null clears the field (merge semantics)
			continue
		}
		if entries, isArray := value.([]any); isArray && idBearingBlockField(key) {
			if vocab == nil {
				if vocab, err = a.payloadIdVocabulary(); err != nil {
					return err
				}
			}
			if err := a.resolveIdEntries(vocab, entries, key, opPath+".set"); err != nil {
				return err
			}
		}
		merged[key] = value
	}
	fullId := blockId(block)
	raw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode merged block: %w", err)
	}
	blocks, err := anyblockjson.UnmarshalBlock(raw, fullId, a.importOptions())
	if err != nil {
		return invalidFragmentError(opPath+".set", err)
	}
	if err := a.claimPayloadIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath + ".set" }); err != nil {
		return err
	}
	a.replaceLive(oldWasTable, blocks)
	return nil
}

func (a *v2StateApplier) applyReplaceSubtree(op opReplaceSubtree, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	run, err := a.decodePayloadRun(op.Blocks, opPath, "blocks", "replace_subtree")
	if err != nil {
		return err
	}
	oldId := blockId(doc.blocks[idx])
	oldSubtree := collectSubtreeIds(a.st, oldId)
	blocks, topIds, err := a.fragmentBlocks(opPath+".blocks", run)
	if err != nil {
		return err
	}
	if err := a.claimPayloadIds(blocks, oldSubtree, runPathFor(opPath, "blocks", topIds)); err != nil {
		return err
	}

	parent := a.st.PickParentOf(oldId)
	if parent == nil {
		return fmt.Errorf("block %s has no parent", oldId)
	}
	parentId := parent.Model().Id
	pos := slice.FindPos(parent.Model().ChildrenIds, oldId)
	a.st.Unlink(oldId)
	// reused ids from the old subtree change parents — detach them first so
	// the replaced structure never holds two links to one block
	for _, b := range blocks {
		if oldSubtree[b.Id] && b.Id != oldId {
			a.st.Unlink(b.Id)
		}
	}
	a.setBlocks(blocks)
	if err := a.insertAt(parentId, pos, topIds); err != nil {
		return fmt.Errorf("splice subtree at %s: %w", parentId, err)
	}
	a.mutated()
	return nil
}

// resolveTarget resolves the shared after/before/inside targeting vocabulary
// (insert_blocks and move_block, R14). It returns the anchor index, the mode
// ("after"|"before"|"inside"|"root") and the position ("first"|"last").
// Omitting all three targeting fields means the document root (§8.2 v0.3.5) —
// the only ops-path into an object that has no addressable blocks yet (SPEC
// §7 keeps title/description out of the document, so an empty object has
// none). The anchor index is -1 in root mode, and `position` picks which end
// of the document: `first` the start, `last` (or absent) the end — the same
// two words that pick an end inside a container (§8.32).
func (a *v2StateApplier) resolveTarget(doc *v2EditDoc, after, before, inside, position, opPath string) (anchor int, mode string, pos string, err error) {
	var refs []string
	if after != "" {
		refs, mode = append(refs, "after"), "after"
	}
	if before != "" {
		refs, mode = append(refs, "before"), "before"
	}
	if inside != "" {
		refs, mode = append(refs, "inside"), "inside"
	}
	if len(refs) > 1 {
		return 0, "", "", v2model.AmbiguousInput("at most one of after, before, inside is allowed",
			v2model.Issue{Path: opPath, Message: fmt.Sprintf("got %d targeting fields (%s)", len(refs), strings.Join(refs, ", "))})
	}
	if len(refs) == 0 {
		if position != "" {
			if err := checkPosition(position, opPath); err != nil {
				return 0, "", "", err
			}
			return -1, "root", position, nil
		}
		return -1, "root", "last", nil
	}
	pos = "last"
	if position != "" {
		if mode != "inside" {
			return 0, "", "", v2model.ValidationFailed("position only applies to inside",
				v2model.Issue{Path: opPath + ".position", Message: fmt.Sprintf("position with %q targeting is meaningless — after/before already name the slot", mode)})
		}
		if err := checkPosition(position, opPath); err != nil {
			return 0, "", "", err
		}
		pos = position
	}
	ref := after + before + inside // exactly one is non-empty
	anchor, err = a.resolveRef(doc, ref, opPath+"."+mode)
	if err != nil {
		return 0, "", "", err
	}
	if mode == "inside" {
		if typ := blockType(doc.blocks[anchor]); apiEditLeafBlockType(typ) {
			return 0, "", "", v2model.ValidationFailed(
				fmt.Sprintf("cannot target inside block %q — %q blocks cannot have children", ref, typ),
				v2model.Issue{Path: opPath + ".inside", Message: "the target is a leaf block type (SPEC §5)"})
		}
	}
	return anchor, mode, pos, nil
}

// apiEditLeafBlockType is the API's new-edit containment policy. The
// AnyBlock format deliberately accepts descendants under file blocks so it
// can losslessly read and write legacy documents, but the editor treats the
// whole file family as leaves. API edits must not create more of that legacy
// shape, whether the caller inserts, moves, or changes a parent's type.
func apiEditLeafBlockType(typ string) bool {
	if anyblockjson.LeafBlockType(typ) {
		return true
	}
	return apiFileFamilyBlockType(typ)
}

func apiFileFamilyBlockType(typ string) bool {
	switch typ {
	case "file", "image", "video", "audio", "pdf":
		return true
	default:
		return false
	}
}

type apiFileDescendantEdge struct {
	ancestorID   string
	descendantID string
}

type apiFileDescendantFinding struct {
	edge         apiFileDescendantEdge
	ancestorType string
	location     string
}

// validateNoNewFileDescendants is the compatibility boundary between the
// AnyBlock format and API editing. The format must preserve legacy file
// descendants, so it cannot reject them globally; the API compares the
// before/after ancestor relation and refuses only newly created legacy shape.
// Looking at every file ancestor (not just the direct parent) closes targeting
// through after/before and insertion below an existing legacy child.
func apiFileDescendantSet(body []byte) (map[apiFileDescendantEdge]bool, error) {
	doc, err := parseEditDoc(body)
	if err != nil {
		return nil, fmt.Errorf("decode document for file containment: %w", err)
	}
	set := map[apiFileDescendantEdge]bool{}
	for _, finding := range apiFileDescendantEdges(doc) {
		set[finding.edge] = true
	}
	return set, nil
}

func validateNoNewFileDescendants(legacy map[apiFileDescendantEdge]bool, afterBody []byte, createdBlocks map[string]string, opPath string) error {
	after, err := parseEditDoc(afterBody)
	if err != nil {
		return fmt.Errorf("decode after document for file containment: %w", err)
	}
	for _, finding := range apiFileDescendantEdges(after) {
		if legacy[finding.edge] {
			continue
		}
		path := opPath
		for origin, id := range createdBlocks {
			if id == finding.edge.descendantID && strings.HasPrefix(origin, opPath+".") {
				path = origin
				break
			}
		}
		return v2model.ValidationFailed(v2InvalidDocMessage,
			v2model.Issue{
				Path: path,
				Message: fmt.Sprintf("block %q would become a new descendant of %s block %q; editor-leaf file blocks cannot have children in new API edits",
					finding.edge.descendantID, finding.ancestorType, finding.edge.ancestorID),
			})
	}
	return nil
}

func opCanChangeFileContainment(raw json.RawMessage) bool {
	var probe struct {
		Op string `json:"op"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return false // apply already returned the path-addressed decode error
	}
	switch probe.Op {
	case "update_block", "replace_subtree", "insert_blocks", "move_block", "set_cell":
		return true
	default:
		return false
	}
}

func apiFileDescendantEdges(doc *v2EditDoc) []apiFileDescendantFinding {
	type ancestor struct {
		indent   int
		identity string
		typ      string
	}
	var findings []apiFileDescendantFinding
	var scanRun func([]map[string]any, string, string, []ancestor)
	var scanTableCells func(map[string]any, string, string, []ancestor)
	scanRun = func(blocks []map[string]any, basePath, rootIdentity string, inheritedFileAncestors []ancestor) {
		stack := make([]ancestor, 0, 8)
		for i, block := range blocks {
			location := fmt.Sprintf("%s/%d", basePath, i)
			indent := blockIndent(block)
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			identity := blockId(block)
			if identity == "" {
				if i == 0 && rootIdentity != "" {
					identity = rootIdentity
				} else {
					identity = "@" + location
				}
			}
			candidates := make([]ancestor, 0, len(inheritedFileAncestors)+len(stack))
			candidates = append(candidates, inheritedFileAncestors...)
			candidates = append(candidates, stack...)
			for _, candidate := range candidates {
				if apiFileFamilyBlockType(candidate.typ) {
					findings = append(findings, apiFileDescendantFinding{
						edge: apiFileDescendantEdge{
							ancestorID:   candidate.identity,
							descendantID: identity,
						},
						ancestorType: candidate.typ,
						location:     location,
					})
				}
			}
			stack = append(stack, ancestor{indent: indent, identity: identity, typ: blockType(block)})
			if blockType(block) == "table" {
				fileAncestors := append([]ancestor(nil), inheritedFileAncestors...)
				for _, candidate := range stack {
					if apiFileFamilyBlockType(candidate.typ) {
						fileAncestors = append(fileAncestors, candidate)
					}
				}
				scanTableCells(block, location, identity, fileAncestors)
			}
		}
	}
	scanTableCells = func(table map[string]any, tablePath, tableIdentity string, inheritedFileAncestors []ancestor) {
		if blockType(table) != "table" {
			return
		}
		columns, _ := table["columns"].([]any)
		rows, _ := table["rows"].([]any)
		for rowIndex, rowRaw := range rows {
			row, _ := rowRaw.(map[string]any)
			rowIdentity, _ := row["id"].(string)
			if rowIdentity == "" {
				rowIdentity = strconv.Itoa(rowIndex)
			}
			cells, _ := row["cells"].([]any)
			for cellIndex, cell := range cells {
				columnIdentity := strconv.Itoa(cellIndex)
				if cellIndex < len(columns) {
					if column, ok := columns[cellIndex].(map[string]any); ok {
						if id, _ := column["id"].(string); id != "" {
							columnIdentity = id
						}
					}
				}
				var run []map[string]any
				switch value := cell.(type) {
				case []any:
					for _, raw := range value {
						if block, ok := raw.(map[string]any); ok {
							run = append(run, block)
						}
					}
				case map[string]any:
					run = []map[string]any{value}
				}
				if len(run) > 0 {
					cellPath := fmt.Sprintf("%s/rows/%d/cells/%d", tablePath, rowIndex, cellIndex)
					cellIdentity := "@cell:" + tableIdentity + ":" + rowIdentity + ":" + columnIdentity
					scanRun(run, cellPath, cellIdentity, inheritedFileAncestors)
				}
			}
		}
	}
	scanRun(doc.blocks, "/blocks", "", nil)
	return findings
}

// checkPosition validates the position vocabulary. It is one check because
// the two words mean the same two ends wherever they appear (inside a
// container, or at the document root) — the schema publishes the enum, and
// this is the runtime backstop for the caller who ignored it.
func checkPosition(position, opPath string) error {
	if position != "first" && position != "last" {
		return v2model.ValidationFailed("invalid position",
			v2model.Issue{Path: opPath + ".position", Message: fmt.Sprintf("unknown position %q", position), Hint: "allowed: first, last"})
	}
	return nil
}

func (a *v2StateApplier) applyInsertBlocks(op opInsertBlocks, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	anchor, mode, pos, err := a.resolveTarget(doc, op.After, op.Before, op.Inside, op.Position, opPath)
	if err != nil {
		return err
	}
	payload, field, err := insertPayload(op, opPath)
	if err != nil {
		return err
	}
	// insert_blocks only ever creates, so its payload carries no id slots at
	// all (§8.30) — they are refused as not part of the op, never resolved.
	run, err := a.decodePayloadRun(payload, opPath, field, "insert_blocks")
	if err != nil {
		return err
	}
	blocks, topIds, err := a.fragmentBlocks(opPath+"."+field, run)
	if err != nil {
		return err
	}
	// the payload carried no ids, so every id here is server-generated; this
	// stays as the collision guard on those (table internals included)
	if err := a.claimPayloadIds(blocks, nil, runPathFor(opPath, field, topIds)); err != nil {
		return err
	}
	a.setBlocks(blocks)
	anchorId, insertPos := "", targetPosition(mode, pos)
	if mode == "root" {
		anchorId, insertPos = rootTarget(doc, pos, -1, -1)
	} else {
		anchorId = blockId(doc.blocks[anchor])
	}
	if err := a.st.InsertTo(anchorId, insertPos, topIds...); err != nil {
		return fmt.Errorf("insert blocks at %q: %w", anchorId, err)
	}
	a.mutated()
	return nil
}

// v2MaxBlocksPerOp caps one op's payload run — the maxItems the served op
// schemas already advertise for the blocks channel (insert_blocks,
// replace_subtree). Advertised but unenforced, one op could inflate the
// document by tens of thousands of blocks and every later op in the batch
// re-rendered them all under the object lock (surface review M7).
const v2MaxBlocksPerOp = 256

// v2MaxMarkdownBlocksPerOp caps how many blocks one op's markdown payload
// may parse to — the same cap as the blocks channel. The markdown channel is
// byte-bounded by its schema, but 3 bytes can encode one block, so without a
// parsed-run cap a maximum-size body reaches ~350k blocks per op — the two
// payload channels must share one cap.
const v2MaxMarkdownBlocksPerOp = v2MaxBlocksPerOp

// insertPayload picks the insert_blocks payload channel: the blocks array, or
// the markdown authoring alternative (§7.1) parsed into the same flat-run
// shape. Exactly one must be given.
func insertPayload(op opInsertBlocks, opPath string) ([]json.RawMessage, string, error) {
	hasBlocks := len(op.Blocks) > 0
	hasMarkdown := op.Markdown != ""
	switch {
	case hasBlocks && hasMarkdown:
		return nil, "", v2model.AmbiguousInput("provide blocks or markdown, not both",
			v2model.Issue{Path: opPath, Message: "blocks (flat AnyBlock payload) and markdown (parsed server-side) are alternative payload channels for insert_blocks"})
	case hasMarkdown:
		run, exceeded := anyblockjson.ParseMarkdownBlocksLimit(op.Markdown, v2MaxMarkdownBlocksPerOp)
		if exceeded {
			return nil, "", v2model.ValidationFailed("markdown produced too many blocks",
				v2model.Issue{Path: opPath + ".markdown", Message: fmt.Sprintf(
					"the markdown parses to more than %d blocks — the per-op limit is %d (the blocks channel's cap); split the content across several insert_blocks ops",
					v2MaxMarkdownBlocksPerOp, v2MaxMarkdownBlocksPerOp)})
		}
		if len(run) == 0 {
			return nil, "", v2model.ValidationFailed("markdown produced no blocks",
				v2model.Issue{Path: opPath + ".markdown", Message: "the markdown body contains no content — give at least one non-blank line"})
		}
		return run, "markdown", nil
	case hasBlocks:
		return op.Blocks, "blocks", nil
	default:
		return nil, "", v2model.ValidationFailed("insert_blocks needs a payload",
			v2model.Issue{Path: opPath, Message: "give blocks (a flat AnyBlock run) or markdown (parsed server-side)"})
	}
}

func (a *v2StateApplier) applyMoveBlock(op opMoveBlock, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	end := doc.subtreeEnd(idx)
	anchor, mode, pos, err := a.resolveTarget(doc, op.After, op.Before, op.Inside, op.Position, opPath)
	if err != nil {
		return err
	}
	if anchor >= idx && anchor < end {
		return v2model.ValidationFailed(
			fmt.Sprintf("cannot move block %q inside its own subtree", op.Id),
			v2model.Issue{Path: opPath, Message: "the target block is a descendant of (or is) the moved block — that would create a cycle; pick a target outside the moved subtree"})
	}
	fullId := blockId(doc.blocks[idx])
	// root mode (anchor -1, no targeting fields): move to one end of the
	// document — the moved subtree is excluded from the anchor search, so
	// "first" is reachable even for the block that is already first
	anchorId, movePos := "", targetPosition(mode, pos)
	if mode == "root" {
		anchorId, movePos = rootTarget(doc, pos, idx, end)
	} else {
		anchorId = blockId(doc.blocks[anchor])
	}
	a.st.Unlink(fullId)
	if err := a.st.InsertTo(anchorId, movePos, fullId); err != nil {
		return fmt.Errorf("move block %s to %q: %w", fullId, anchorId, err)
	}
	a.mutated()
	return nil
}

func (a *v2StateApplier) applyDeleteBlock(op opDeleteBlock, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveSubject(doc, "delete_block", op.Id, op.Match, opPath, everyBlock)
	if err != nil {
		return err
	}
	end := doc.subtreeEnd(idx)
	// the descendant guard names the block as the caller addressed it — the
	// reference it sent, or the id the locator resolved. A locator caller
	// never sent an id, and `block "" has 3 descendant blocks` would name
	// nothing it could retry with; the resolved full id always is one.
	ref := op.Id
	if ref == "" {
		ref = blockId(doc.blocks[idx])
	}
	if descendants := end - idx - 1; descendants > 0 && !op.Recursive {
		return v2model.ValidationFailed(
			fmt.Sprintf("block %q has %s — pass \"recursive\": true to delete the whole subtree", ref, countBlocks(descendants)),
			v2model.Issue{Path: opPath, Message: "delete_block without recursive only deletes childless blocks", Hint: "or move_block the descendants out first"})
	}
	a.st.Unlink(blockId(doc.blocks[idx])) // the unlinked subtree is dropped by apply-side normalization
	a.mutated()
	return nil
}

func (a *v2StateApplier) applyReplaceText(op opReplaceText, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	if op.Find == "" {
		return v2model.ValidationFailed("find must not be empty",
			v2model.Issue{Path: opPath + ".find", Message: "give the exact text to replace"})
	}
	var idx int
	if op.Id == "" {
		// id omitted: find IS the locator — resolved here,
		// per op, against the live view, one match or refuse. doc is fresh
		// under the object lock, so op i resolves against op i−1's edits.
		idx, err = resolveByText(doc, op.Find, "find", opPath+".find", textBlocksOnly)
	} else {
		idx, err = a.resolveRef(doc, op.Id, opPath+".id")
	}
	if err != nil {
		return err
	}
	block := doc.blocks[idx]
	// error texts name the block as the caller addressed it — the reference
	// it sent, or the id the locator resolved (always a valid retry value)
	ref := op.Id
	if ref == "" {
		ref = blockId(block)
	}
	typ := blockType(block)
	if !anyblockjson.TextBlockType(typ) {
		return v2model.ValidationFailed(
			fmt.Sprintf("block %q is a %q block and has no text", ref, typ),
			v2model.Issue{Path: opPath + ".id", Message: "replace_text only applies to text-bearing blocks"})
	}
	// Find runs on the block's document text — markup source for ordinary
	// text-bearing blocks, literal text for code/embed (§8.4) — exactly what
	// the agent read. Replacement is always literal prose: ordinary text
	// escapes it into inline source before the splice so caller-controlled
	// asterisks/tags cannot mint marks or tear the marks around the match.
	text, _ := block["text"].(string)
	count := strings.Count(text, op.Find)
	switch {
	case count == 0:
		return v2model.ValidationFailed(
			fmt.Sprintf("no match found for %q in block %q — read the block and copy the find text exactly, including inline markup", op.Find, ref),
			v2model.Issue{Path: opPath + ".find", Message: "0 matches in the block's text"})
	case count > 1 && !op.ReplaceAll:
		// the locator path lands here too: several occurrences WITHIN the one
		// matched block are replace_all's (later nth's) territory (§5.3), not
		// a resolution failure
		return v2model.ValidationFailed(
			fmt.Sprintf("found %d matches for %q in block %q — provide more context to make the match unique, or set \"replace_all\": true", count, op.Find, ref),
			v2model.Issue{Path: opPath + ".find", Message: fmt.Sprintf("%d matches in the block's text", count)})
	}
	resultLen, ok := replacedTextUTF16Length(text, op.Find, op.Replace, count)
	if !ok || resultLen > v2MaxBlockTextUTF16 {
		message := fmt.Sprintf("the replacement would exceed the %d UTF-16-unit block text limit", v2MaxBlockTextUTF16)
		if ok {
			message = fmt.Sprintf("the replacement would produce %d UTF-16 units; block text is limited to %d", resultLen, v2MaxBlockTextUTF16)
		}
		return v2model.ValidationFailed("replacement would make block text too long",
			v2model.Issue{Path: opPath + ".replace", Message: message, Hint: "use a more specific find, a shorter replacement, or split the edit"})
	}
	fullId := blockId(block)
	b := a.st.Pick(fullId)
	if b == nil {
		return fmt.Errorf("block %s not in state", fullId)
	}
	m := b.Copy().Model()
	// canonical is what a full re-marshal would emit as this block's "text":
	// the raw splice for the literal channels (§8.4 — code and embed render
	// t.Text verbatim). Markup text uses a semantic literal replacement:
	// source matching still happens against exactly what the caller read, but
	// a placeholder is parsed in the replacement's context and then replaced
	// in the plain-text model. That preserves enclosing marks even for an
	// empty replacement or a one-character delimiter such as "*".
	canonical := ""
	switch content := m.Content.(type) {
	case *model.BlockContentOfText:
		if content.Text == nil {
			content.Text = &model.BlockContentText{}
		}
		if typ == "code" {
			if op.ReplaceAll {
				text = strings.ReplaceAll(text, op.Find, op.Replace)
			} else {
				text = strings.Replace(text, op.Find, op.Replace, 1)
			}
			content.Text.Text = text // literal (§8.4); code carries no marks
			content.Text.Marks = nil
			canonical = text
		} else {
			plain, marks, err := replaceInlineSourceLiteral(text, op.Find, op.Replace, op.ReplaceAll, count)
			if err != nil {
				if errors.Is(err, errReplaceTextMarkupMetadata) {
					return v2model.ValidationFailed(
						"find selects inline-markup metadata rather than replaceable text",
						v2model.Issue{Path: opPath + ".find", Message: "replace_text can edit visible prose or whole markup spans, but not tag attributes or link destinations"})
				}
				return invalidDocError(err)
			}
			content.Text.Text = plain
			if len(marks) == 0 {
				content.Text.Marks = nil
			} else {
				content.Text.Marks = &model.BlockContentTextMarks{Marks: marks}
			}
			// the exporter renders renderInline(text, marks) with mark
			// compaction OFF on the edit path, so this IS the re-marshal's
			// output for the field
			canonical = anyblockjson.RenderInlineText(plain, marks)
		}
	case *model.BlockContentOfLatex:
		if content.Latex == nil {
			content.Latex = &model.BlockContentLatex{}
		}
		if op.ReplaceAll {
			text = strings.ReplaceAll(text, op.Find, op.Replace)
		} else {
			text = strings.Replace(text, op.Find, op.Replace, 1)
		}
		content.Latex.Text = text // literal (§8.4)
		canonical = text
	default:
		return v2model.ValidationFailed(
			fmt.Sprintf("block %q is a %q block and has no text", ref, typ),
			v2model.Issue{Path: opPath + ".id", Message: "replace_text only applies to text-bearing blocks"})
	}
	a.st.Set(simple.New(m))
	// incremental view maintenance (surface review M7): replace_text changes
	// exactly ONE exported field of one block — no ids, no structure, no
	// indents — so the view stays valid with that field updated in place and
	// the whole-document re-marshal the next op would otherwise pay is
	// skipped. This is what makes a batch of text edits O(document), not
	// O(ops × document), under the object lock.
	if canonical == "" {
		delete(block, "text") // the exporter omits empty text (setNonEmpty)
	} else {
		block["text"] = canonical
	}
	a.textEdited()
	return nil
}

var errReplaceTextMarkupMetadata = errors.New("replace_text match is not represented in parsed inline text")

// v2MaxBlockTextUTF16 mirrors the block.text maxLength served by the op
// schemas. Enforcing it before strings.ReplaceAll or a UTF-16 output buffer
// allocation prevents a dense replace_all from expanding a valid block into
// an unbounded transient result under the object lock.
const v2MaxBlockTextUTF16 int64 = 1_048_576

func replacedTextUTF16Length(source, find, replacement string, count int) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	sourceLen := int64(textutil.UTF16RuneCountString(source))
	findLen := int64(textutil.UTF16RuneCountString(find))
	replacementLen := int64(textutil.UTF16RuneCountString(replacement))
	occurrences := int64(count)
	if occurrences < 0 || (findLen != 0 && occurrences > maxInt64/findLen) ||
		(replacementLen != 0 && occurrences > maxInt64/replacementLen) {
		return 0, false
	}
	removed := occurrences * findLen
	added := occurrences * replacementLen
	if removed > sourceLen {
		return 0, false
	}
	base := sourceLen - removed
	if added > maxInt64-base {
		return 0, false
	}
	return base + added, true
}

// replaceInlineSourceLiteral keeps `find` source-addressed while making the
// replacement semantic plain text. Parsing a context-safe word placeholder
// first lets the parser determine which marks enclose the match; replacing
// that placeholder in the text model cannot create markup, join delimiter
// runs, or leave delimiter bytes behind when replacement is empty.
func replaceInlineSourceLiteral(source, find, replacement string, replaceAll bool, replacements int) (string, []*model.BlockContentTextMark, error) {
	originalPlain, _, err := anyblockjson.ParseInlineText(source)
	if err != nil {
		return "", nil, err
	}
	placeholder, err := inlineReplacementPlaceholder(source, originalPlain)
	if err != nil {
		return "", nil, err
	}
	withPlaceholder := strings.Replace(source, find, placeholder, 1)
	if replaceAll {
		withPlaceholder = strings.ReplaceAll(source, find, placeholder)
	}
	plain, marks, err := anyblockjson.ParseInlineText(withPlaceholder)
	if err != nil {
		return "", nil, err
	}
	if strings.Count(plain, placeholder) != replacements {
		// The source match was wholly or partly structural metadata (for
		// example a mention object_id or link destination), so it cannot be
		// replaced as literal prose without changing the markup vocabulary.
		return "", nil, errReplaceTextMarkupMetadata
	}

	ranges := inlinePlaceholderRanges(plain, placeholder)
	replacementLen := int32(textutil.UTF16RuneCountString(replacement))
	plain = replaceInlinePlaceholders(plain, replacement, ranges)
	marks = adjustInlineMarks(marks, ranges, replacementLen)
	return plain, marks, nil
}

// inlineReplacementPlaceholder returns a bounded, caller-unpredictable word
// absent from both source and parsed prose. The bound is a hard guarantee that
// hostile input cannot turn collision avoidance into an unbounded loop under
// the object lock; a collision remains astronomically unlikely per attempt.
func inlineReplacementPlaceholder(source, plain string) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		var entropy [4]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return "", fmt.Errorf("mint inline replacement placeholder: %w", err)
		}
		placeholder := inlinePlaceholderFromEntropy(entropy)
		if !strings.Contains(source, placeholder) && !strings.Contains(plain, placeholder) {
			return placeholder, nil
		}
	}
	return "", errors.New("could not mint a unique inline replacement placeholder")
}

func inlinePlaceholderFromEntropy(entropy [4]byte) string {
	// Two private-use code points are ordinary word characters to the inline
	// parser. They cap worst-case UTF-16 source expansion at 2x even when
	// replace_all matches every one-character prose token. Keeping them
	// distinct makes the token unbordered: a real adjacent rune cannot combine
	// with half the placeholder and shift the detected replacement range.
	const privateUseRunes = 0x1900 // U+E000..U+F8FF inclusive
	firstIndex := (int(entropy[0])<<8 | int(entropy[1])) % privateUseRunes
	secondIndex := (int(entropy[2])<<8 | int(entropy[3])) % privateUseRunes
	if secondIndex == firstIndex {
		secondIndex = (secondIndex + 1) % privateUseRunes
	}
	return string([]rune{rune(0xE000 + firstIndex), rune(0xE000 + secondIndex)})
}

func inlinePlaceholderRanges(plain, placeholder string) []model.Range {
	ranges := make([]model.Range, 0, strings.Count(plain, placeholder))
	cursor := 0
	utf16Cursor := int32(0)
	placeholderLen := int32(textutil.UTF16RuneCountString(placeholder))
	for {
		rel := strings.Index(plain[cursor:], placeholder)
		if rel < 0 {
			return ranges
		}
		start := cursor + rel
		utf16Cursor += int32(textutil.UTF16RuneCountString(plain[cursor:start]))
		ranges = append(ranges, model.Range{From: utf16Cursor, To: utf16Cursor + placeholderLen})
		cursor = start + len(placeholder)
		utf16Cursor += placeholderLen
	}
}

// replaceInlinePlaceholders performs every replacement in one UTF-16 pass.
// Mark ranges use UTF-16 offsets, so building in the same coordinate system
// avoids per-match whole-string conversions and keeps replace_all linear.
func replaceInlinePlaceholders(plain, replacement string, ranges []model.Range) string {
	source := textutil.StrToUTF16(plain)
	insert := textutil.StrToUTF16(replacement)
	placeholderLen := int(ranges[0].To - ranges[0].From)
	capacity := len(source) + len(ranges)*(len(insert)-placeholderLen)
	if capacity < 0 {
		capacity = 0
	}
	result := make([]uint16, 0, capacity)
	cursor := 0
	for _, r := range ranges {
		result = append(result, source[cursor:int(r.From)]...)
		result = append(result, insert...)
		cursor = int(r.To)
	}
	result = append(result, source[cursor:]...)
	return textutil.UTF16ToStr(result)
}

// adjustInlineMarks maps every parsed mark through the same simultaneous
// replacements in O(m log r). Start/end boundary bias preserves a mark that
// enclosed a placeholder, while keeping marks immediately before or after a
// match on their original side. Empty replacements drop zero-length marks.
func adjustInlineMarks(marks []*model.BlockContentTextMark, ranges []model.Range, replacementLen int32) []*model.BlockContentTextMark {
	placeholderLen := ranges[0].To - ranges[0].From
	prefixDelta := make([]int32, len(ranges)+1)
	for i := range ranges {
		prefixDelta[i+1] = prefixDelta[i] + replacementLen - placeholderLen
	}
	mapStart := func(pos int32) int32 {
		i := sort.Search(len(ranges), func(i int) bool { return ranges[i].To > pos })
		if i == len(ranges) || pos <= ranges[i].From {
			return pos + prefixDelta[i]
		}
		return ranges[i].From + prefixDelta[i]
	}
	mapEnd := func(pos int32) int32 {
		i := sort.Search(len(ranges), func(i int) bool { return ranges[i].To >= pos })
		if i == len(ranges) || pos <= ranges[i].From {
			return pos + prefixDelta[i]
		}
		return ranges[i].From + prefixDelta[i] + replacementLen
	}
	adjusted := marks[:0]
	for _, mark := range marks {
		if mark == nil || mark.Range == nil {
			continue
		}
		mark.Range.From = mapStart(mark.Range.From)
		mark.Range.To = mapEnd(mark.Range.To)
		if mark.Range.From < mark.Range.To {
			adjusted = append(adjusted, mark)
		}
	}
	return adjusted
}

func (a *v2StateApplier) applySetCell(op opSetCell, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveRef(doc, op.TableId, opPath+".table_id")
	if err != nil {
		return err
	}
	table := doc.blocks[idx]
	if typ := blockType(table); typ != "table" {
		return v2model.ValidationFailed(
			fmt.Sprintf("block %q is a %q block, not a table", op.TableId, typ),
			v2model.Issue{Path: opPath + ".table_id", Message: "set_cell addresses a table block (SPEC §6.1)"})
	}
	if op.Value == nil {
		return v2model.ValidationFailed("value is required",
			v2model.Issue{Path: opPath + ".value", Message: "give the new cell content — a string, null (clear), a block object, or an array of blocks (SPEC §6.1)"})
	}
	var value any
	if err := decodeJSONUseNumber(op.Value, &value); err != nil {
		return v2model.ValidationFailed("invalid cell value",
			v2model.Issue{Path: opPath + ".value", Message: err.Error()})
	}
	switch value.(type) {
	case nil, string, map[string]any, []any:
	default:
		return v2model.ValidationFailed("invalid cell value",
			v2model.Issue{Path: opPath + ".value", Message: "a cell is a string, null, a block object, or an array of blocks (SPEC §6.1)"})
	}
	// the array form's elements past the first are the cell's flat
	// DESCENDANTS, ids and all — a default read serves those relabeled, so
	// they resolve like every other id slot (payloadids.go). Element 0 is the
	// cell block itself, whose id is derived (rowId-colId) and forced by the
	// importer.
	vocab, err := a.payloadIdVocabulary()
	if err != nil {
		return err
	}
	if err := a.resolveCellValueIds(vocab, value, opPath+".value"); err != nil {
		return err
	}

	ci, err := resolveTablePart(table, "columns", op.Col, op.TableId, opPath+".col")
	if err != nil {
		return err
	}
	ri, err := resolveTablePart(table, "rows", op.Row, op.TableId, opPath+".row")
	if err != nil {
		return err
	}
	// edit the cell on the table's document form and re-import the one table
	// block: rows, columns and derived cell ids round-trip, so untouched
	// cells land unchanged and only the edited cell diffs
	edited := map[string]any{}
	for k, v := range table {
		edited[k] = v
	}
	delete(edited, "indent")
	rows, _ := edited["rows"].([]any)
	row, ok := rows[ri].(map[string]any)
	if !ok {
		return fmt.Errorf("table %s: row %d is not an object", op.TableId, ri)
	}
	cells, _ := row["cells"].([]any)
	for len(cells) <= ci {
		cells = append(cells, nil)
	}
	cells[ci] = value
	row["cells"] = cells
	rows[ri] = row
	edited["rows"] = rows

	fullId := blockId(table)
	raw, err := json.Marshal(edited)
	if err != nil {
		return fmt.Errorf("encode edited table: %w", err)
	}
	blocks, err := anyblockjson.UnmarshalBlock(raw, fullId, a.importOptions())
	if err != nil {
		return invalidDocError(err)
	}
	if err := a.claimPayloadIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath + ".value" }); err != nil {
		return err
	}
	a.replaceLive(true, blocks)
	return nil
}

func (a *v2StateApplier) applyItems(op opItems, opPath string) error {
	if len(op.Items) == 0 {
		return v2model.ValidationFailed(op.Op+" needs items",
			v2model.Issue{Path: opPath + ".items", Message: "items must list at least one member object id"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	items := a.st.GetStoreSlice(template.CollectionStoreKey)
	if len(items) == 0 && !a.s.isCollectionType(a.spaceId, doc.docType()) {
		return v2model.ValidationFailed(
			fmt.Sprintf("%s requires a collection — this object's type is %q", op.Op, doc.docType()),
			v2model.Issue{Path: opPath, Message: "only collection objects carry items", Hint: "POST /v2/spaces/{space_id}/collections creates one"})
	}
	if op.Op == "add_items" {
		present := map[string]bool{}
		for _, id := range items {
			present[id] = true
		}
		for _, id := range op.Items {
			if !present[id] {
				present[id] = true
				items = append(items, id)
			}
		}
		a.st.UpdateStoreSlice(template.CollectionStoreKey, items)
		a.mutated()
		return nil
	}
	remove := map[string]bool{}
	for _, id := range op.Items {
		remove[id] = true
	}
	kept := make([]string, 0, len(items))
	for _, id := range items {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	a.st.UpdateStoreSlice(template.CollectionStoreKey, kept)
	a.mutated()
	return nil
}

//
// ---- payload decoding ----
//

// decodePayloadRun decodes a flat payload run with R3 relative indents:
// payload indent 0 = the insertion level (the anchor's level for
// after/before and replace_subtree, the container's child level for inside).
// The run must obey the format's monotonicity (V1) internally; the indents
// stay run-relative — the state splice, not indent arithmetic, sets the
// insertion level.
//
// Ids: every id slot the run carries — the block's own, and the ones nested
// in its columns/rows/cells/views — RESOLVES against the pre-op document
// (payloadids.go), so a compact label echoed from a read names the element it
// labels instead of renaming it. Only MISSING ids are minted, and only those
// land in created_blocks (created_views for a view) keyed by payload position:
// a resolved id names something that already existed, and reporting it as
// created would be the same kind of lie diff_stats used to tell.
//
// op names the op the run belongs to. When it is a NEW-content op
// (v2NewContentOps — the one set the served schema reads too, §8.30) the run
// carries no id slots at all: they are refused as not part of the op rather
// than resolved.
func (a *v2StateApplier) decodePayloadRun(raws []json.RawMessage, opPath, field, op string) ([]map[string]any, error) {
	if len(raws) == 0 {
		return nil, v2model.ValidationFailed("blocks must not be empty",
			v2model.Issue{Path: opPath + "." + field, Message: "give at least one block"})
	}
	// the maxItems the op schemas advertise, enforced (surface review M7): an
	// unbounded run inflates the document every later op re-renders under the
	// object lock (the markdown channel enforces the same cap at parse time)
	if len(raws) > v2MaxBlocksPerOp {
		return nil, v2model.ValidationFailed("too many blocks in one op",
			v2model.Issue{
				Path:    opPath + "." + field,
				Message: fmt.Sprintf("%d blocks exceeds the %d-block per-op limit the op schema advertises (maxItems)", len(raws), v2MaxBlocksPerOp),
				Hint:    "split the run across several ops — the blocks and markdown channels share this cap",
			})
	}
	visit := a.rejectOrMintSlot(op)
	if !v2NewContentOps[op] {
		vocab, err := a.payloadIdVocabulary()
		if err != nil {
			return nil, err
		}
		visit = a.resolveOrMintSlot(vocab)
	}
	run := make([]map[string]any, 0, len(raws))
	prev := -1
	for j, raw := range raws {
		path := fmt.Sprintf("%s.%s[%d]", opPath, field, j)
		block, err := decodeOpBlock(raw, path)
		if err != nil {
			return nil, err
		}
		if err := walkPayloadIdSlots(block, path, visit); err != nil {
			return nil, err
		}
		rel := 0
		if v, hasIndent := block["indent"]; hasIndent {
			f, isNum := jsonFloat64(v)
			if !isNum || f != float64(int(f)) || f < 0 {
				return nil, v2model.ValidationFailed("invalid payload indent",
					v2model.Issue{Path: path + ".indent", Message: "indent must be a non-negative integer, relative to the insertion level (0 = the anchor's level; for inside, 0 = the container's child level)"})
			}
			rel = int(f)
		}
		if j == 0 && rel != 0 {
			return nil, v2model.ValidationFailed("the first payload block's indent must be 0",
				v2model.Issue{Path: path + ".indent", Message: fmt.Sprintf("indent %d on the first block — payload indents are relative: 0 is the insertion level", rel)})
		}
		if rel > prev+1 {
			return nil, v2model.ValidationFailed("payload indents must be monotonic",
				v2model.Issue{Path: path + ".indent", Message: fmt.Sprintf("indent %d follows indent %d — a block can be at most one level deeper than its predecessor", rel, prev)})
		}
		prev = rel
		if rel > 0 {
			block["indent"] = float64(rel)
		} else {
			delete(block, "indent")
		}
		run = append(run, block)
	}
	return run, nil
}

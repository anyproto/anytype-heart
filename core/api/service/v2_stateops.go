package service

// v2_stateops.go applies the Phase-3 PATCH ops (v2_ops.go) to a child
// *state.State of the live object — the ordinary editor mutation model
// (APIV2.md §2 Phase 3): ops are operations, not a document rewrite. The
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
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

// v2SkipPostOpValidate disables the post-op document validation. The check is
// ON by default (review B′3): the whole-document Validate the state pipeline
// replaced was load-bearing for invariants no single fragment can see — V3
// row→column containment, the document-wide id domain (derived rowId-colId
// cells, other tables' row/column ids), and the absolute nesting bound. The
// after-document is already marshaled for diffStats, so the check is nearly
// free. The escape hatch exists only for debugging a suspected false
// rejection.
var v2SkipPostOpValidate = os.Getenv("ANYTYPE_API_V2_SKIP_EDIT_VALIDATE") == "1"

// v2InvalidDocMessage is the R5-net rejection message (kept verbatim from
// the document-level pipeline — it is part of the agent-facing contract).
const v2InvalidDocMessage = "the ops would produce an invalid document — no op was applied"

// v2StateApplier applies the ops of one PATCH to one child state.
type v2StateApplier struct {
	s        *V2Service
	spaceId  string
	objectId string
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

	// view is the state's flat-document rendering, rebuilt lazily after any
	// mutating op. Everything agent-facing (refs, indents, error texts) reads
	// it; nothing writes it.
	view *v2EditDoc
	// viewBody is the marshaled form the current view was parsed from, so the
	// caller can reuse it as the after-document instead of marshaling again
	// (review A′2).
	viewBody []byte
	// marshalResolver is the ONE store-backed resolver reused across every
	// marshal of this PATCH (review A′2).
	marshalResolver *storeresolver.Resolvers
}

func newV2StateApplier(s *V2Service, spaceId, objectId string, sbType model.SmartBlockType, st *state.State, resolvers *creatingResolvers) *v2StateApplier {
	return &v2StateApplier{
		s:             s,
		spaceId:       spaceId,
		objectId:      objectId,
		sbType:        sbType,
		st:            st,
		resolvers:     resolvers,
		createdBlocks: map[string]string{},
		claimedIds:    map[string]bool{},
		mintedThisOp:  map[string]bool{},
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
	}
	return a.marshalResolver.Options()
}

// marshalDoc renders the current state as its canonical flat document.
//
// With a nil sink (view rebuilds and the after-document) a marshal warning is
// an ERROR, not something to swallow (review B′2). The exporter degrades
// rather than failing when a sink is installed — it clamps an over-deep
// indent — so a silently-clamped view made later ops in the same batch read
// wrong depths: deleteBlock saw `descendants == 0`, skipped the recursive
// guard and dropped a whole subtree, and the clamped after-document then
// validated clean. Failing here rejects the whole PATCH instead.
func (a *v2StateApplier) marshalDoc(onWarning func(anyblockjson.Issue)) ([]byte, error) {
	opts := a.marshalOptions()
	var degraded []apimodel.V2Issue
	if onWarning == nil {
		onWarning = func(iss anyblockjson.Issue) {
			degraded = append(degraded, apimodel.V2Issue{Path: iss.Path, Message: iss.Message})
		}
	}
	opts.OnWarning = onWarning
	doc, err := anyblockjson.Marshal(a.sbType, snapshotFromState(a.st), opts)
	if err != nil {
		return nil, fmt.Errorf("marshal object %s: %w", a.objectId, err)
	}
	if len(degraded) > 0 {
		return nil, apimodel.NewV2Error(http.StatusBadRequest, apimodel.V2CodeValidationFailed,
			v2InvalidDocMessage, degraded...)
	}
	return doc, nil
}

// begin renders the before-document with the C11 write-safety guard: any
// marshal loss warning refuses the PATCH — otherwise content the format
// cannot represent could be silently damaged by a later full-subtree op.
func (a *v2StateApplier) begin() ([]byte, error) {
	var warnings []apimodel.V2Issue
	doc, err := a.marshalDoc(func(iss anyblockjson.Issue) {
		warnings = append(warnings, apimodel.V2Issue{Path: iss.Path, Message: iss.Message})
	})
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		return nil, apimodel.NewV2Error(http.StatusUnprocessableEntity, apimodel.V2CodeValidationFailed,
			"this object contains content the AnyBlock format cannot fully represent — a PATCH would drop it (C11); edit it in the app or replace it wholesale with PUT",
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
// diffStats is the view we already rendered).
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

// importOptions are the fragment-import options: the Phase-2 create-missing
// resolvers plus this applier's id minting (editor-shaped, collision-free).
func (a *v2StateApplier) importOptions() anyblockjson.Options {
	opts := a.resolvers.Options()
	opts.GenerateId = a.mintBlockId
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
	var v2Err *apimodel.V2Error
	if errors.As(verr, &v2Err) && v2Err.Code == apimodel.V2CodeValidationFailed {
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
	var v2Err *apimodel.V2Error
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

// duplicateIdError is the R5-net rejection for a payload id that already
// exists in the document (the document-level pipeline caught this via the
// format's id-uniqueness check; the state pipeline checks it explicitly,
// with the op-shaped path).
func duplicateIdError(path string, id string) error {
	return apimodel.NewV2Error(http.StatusBadRequest, apimodel.V2CodeValidationFailed, v2InvalidDocMessage,
		apimodel.V2Issue{
			Path:    path,
			Message: fmt.Sprintf("duplicate block id %q — it already exists in the document", id),
		})
}

// validateEditedDoc is the R5 whole-document net, restored (review B′3):
// fragment validation only sees a payload run in isolation, so invariants
// that span the spliced result — V3 row→column containment, the
// document-wide id domain, the absolute nesting bound — are checked here on
// the would-be document. A failure rejects the whole PATCH under the
// unchanged agent-facing message; nothing is applied.
func validateEditedDoc(objectId string, doc []byte) error {
	if v2SkipPostOpValidate {
		return nil
	}
	if err := anyblockjson.Validate(doc); err != nil {
		log.Warnf("api v2 edit: object %s post-op document fails validation: %v", objectId, err)
		return invalidDocError(err)
	}
	return nil
}

//
// ---- op dispatch ----
//

// apply dispatches one op. i is the op's position (error paths are
// "ops[i]…", R5).
func (a *v2StateApplier) apply(i int, raw json.RawMessage) error {
	opPath := fmt.Sprintf("ops[%d]", i)
	var probe struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return apimodel.V2ValidationFailed("each op must be a JSON object",
			apimodel.V2Issue{Path: opPath, Message: err.Error()})
	}
	switch probe.Op {
	case "setProperties":
		var op opSetProperties
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applySetProperties(op, opPath)
	case "updateBlock":
		var op opUpdateBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyUpdateBlock(op, opPath)
	case "replaceBlock":
		var op opReplaceBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyReplaceBlock(op, opPath)
	case "replaceSubtree":
		var op opReplaceSubtree
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyReplaceSubtree(op, opPath)
	case "insertBlocks":
		var op opInsertBlocks
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyInsertBlocks(op, opPath)
	case "moveBlock":
		var op opMoveBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyMoveBlock(op, opPath)
	case "deleteBlock":
		var op opDeleteBlock
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyDeleteBlock(op, opPath)
	case "replaceText":
		var op opReplaceText
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyReplaceText(op, opPath)
	case "setCell":
		var op opSetCell
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applySetCell(op, opPath)
	case "addItems", "removeItems":
		var op opItems
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return err
		}
		return a.applyItems(op, opPath)
	case "":
		return apimodel.V2ValidationFailed("op is required",
			apimodel.V2Issue{Path: opPath + ".op", Message: "missing op", Hint: "allowed ops: " + strings.Join(v2OpNames, ", ")})
	default:
		return apimodel.V2ValidationFailed(fmt.Sprintf("unknown op %q", probe.Op),
			apimodel.V2Issue{Path: opPath + ".op", Message: fmt.Sprintf("unknown op %q", probe.Op), Hint: "allowed ops: " + strings.Join(v2OpNames, ", ")})
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
		return -1, apimodel.V2AmbiguousInput(
			fmt.Sprintf("block reference %q matches more than one block — use the full block id", ref),
			apimodel.V2Issue{Path: path, Message: "the reference is a suffix of several block ids"})
	default:
		return -1, apimodel.V2NotFound(
			fmt.Sprintf("block %q not found — GET the object with ?outline=true to list block ids", ref))
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

// checkFreshIds rejects payload block ids (including table internals) that
// already exist in the state or this PATCH, except ids the op explicitly
// replaces (allowed) and ids the op itself minted (fresh by construction).
// On success every payload id is claimed for the rest of the PATCH.
func (a *v2StateApplier) checkFreshIds(blocks []*model.Block, allowed map[string]bool, pathFor func(id string) string) error {
	for _, b := range blocks {
		if allowed[b.Id] || a.mintedThisOp[b.Id] {
			continue
		}
		if a.st.Exists(b.Id) || a.claimedIds[b.Id] {
			return duplicateIdError(pathFor(b.Id), b.Id)
		}
	}
	for _, b := range blocks {
		a.claimedIds[b.Id] = true
	}
	a.mintedThisOp = map[string]bool{}
	return nil
}

// runPathFor reports a duplicate id under its payload position when the id
// is a run top block, or the op's blocks list otherwise (table internals).
func runPathFor(opPath string, topIds []string) func(id string) string {
	topIndex := map[string]int{}
	for j, id := range topIds {
		topIndex[id] = j
	}
	return func(id string) string {
		return fmt.Sprintf("%s.blocks[%d].id", opPath, topIndex[id])
	}
}

// collectSubtreeIds gathers a block and all its descendants.
func collectSubtreeIds(st *state.State, id string) map[string]bool {
	out := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		if out[cur] {
			return
		}
		out[cur] = true
		if b := st.Pick(cur); b != nil {
			for _, child := range b.Model().ChildrenIds {
				walk(child)
			}
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
// Block_InnerFirst prepends (first child). Root mode falls through to
// Block_Inner — with an empty target id InsertTo appends at the document
// root's end.
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
// ---- setProperties ----
//

func (a *v2StateApplier) applySetProperties(op opSetProperties, opPath string) error {
	if len(op.Set) == 0 && len(op.Unset) == 0 && len(op.Add) == 0 && len(op.Remove) == 0 {
		return apimodel.V2ValidationFailed("setProperties needs at least one of set, unset, add, remove",
			apimodel.V2Issue{Path: opPath, Message: "set, unset, add and remove are all empty"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	var issues []apimodel.V2Issue
	var known []string

	// checkKey is the shared key validation (envelope keys, output-only,
	// unknown with did-you-mean); false = the key is unusable.
	checkKey := func(key, path string) bool {
		switch {
		case key == "id" || key == "type":
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q is not a property — it is lifted to the document envelope and cannot be set here", key)})
			return false
		case v2OutputOnlyPropertyKeys[key]:
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only (SPEC §4a) — export writes it, writes must not", key)})
			return false
		default:
			if _, inDoc := doc.properties[key]; !inDoc && !a.s.propertyKeyExists(a.spaceId, key) {
				if known == nil {
					known = a.s.knownPropertyKeys(a.spaceId)
				}
				issues = append(issues, unknownPropertyIssue(key, path, known,
					fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", a.spaceId, a.spaceId)))
				return false
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
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q appears in both %s and %s — pick one", key, prev, field)})
			return false
		}
		seenIn[key] = field
		return true
	}

	setKeys := sortedKeys(op.Set)
	for _, key := range setKeys {
		path := opPath + ".set." + key
		claim(key, "set", path) // set goes first — it can never collide
		checkKey(key, path)
	}
	unset := map[string]bool{}
	for _, key := range op.Unset {
		path := opPath + ".unset." + key
		if v2OutputOnlyPropertyKeys[key] {
			issues = append(issues, apimodel.V2Issue{Path: path,
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
			path := opPath + "." + field + "." + key
			if !claim(key, field, path) || !checkKey(key, path) {
				continue
			}
			if format := a.propertyFormat(key); !v2ListShapedFormats[format] {
				issues = append(issues, apimodel.V2Issue{Path: path,
					Message: fmt.Sprintf("%q has format %q — %s only applies to list-shaped formats (%s); use set", key, anyblockjson.FormatName(format), field, v2ListShapedFormatNames)})
				continue
			}
			var entries []any
			if err := json.Unmarshal(m[key], &entries); err != nil {
				issues = append(issues, apimodel.V2Issue{Path: path,
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
		return apimodel.V2ValidationFailed("setProperties rejected", issues...)
	}
	for _, key := range setKeys {
		var raw any
		if err := json.Unmarshal(op.Set[key], &raw); err != nil {
			return apimodel.V2ValidationFailed("invalid set value",
				apimodel.V2Issue{Path: opPath + ".set." + key, Message: err.Error()})
		}
		value := anyblockjson.UnmarshalPropertyValue(key, raw, a.resolvers.Options())
		a.st.SetDetail(domain.RelationKey(key), domain.ValueFromProto(value))
		// a key new to the object needs its relation link, or the detail value
		// is wiped on replay (review A1 in miniature; AddRelationLinks dedupes)
		a.st.AddRelationLinks(&model.RelationLink{Key: key, Format: a.propertyFormat(key)})
	}
	for key := range unset {
		a.st.RemoveDetail(domain.RelationKey(key)) // unsetting an absent key is a no-op
	}
	// add appends without duplicating an existing entry; entry resolution is
	// the same create-missing path as set (option names → ids, §3)
	for _, key := range sortedKeys(addEntries) {
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
	// resolution is READ-ONLY (a.marshalOptions() — store-backed, never
	// creating): a remove must never mint the very option it names; an
	// unresolved name simply matches nothing.
	for _, key := range sortedKeys(removeEntries) {
		if !a.st.CombinedDetails().Has(domain.RelationKey(key)) {
			continue // an absent key stays absent — remove never creates presence
		}
		value := anyblockjson.UnmarshalPropertyValue(key, removeEntries[key], a.marshalOptions())
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
		a.st.SetDetail(domain.RelationKey(key), domain.StringList(kept))
	}
	a.mutated()
	return nil
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
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	if len(op.Set) == 0 {
		return apimodel.V2ValidationFailed("updateBlock needs a non-empty set",
			apimodel.V2Issue{Path: opPath + ".set", Message: "set is empty — name the fields to change (merge semantics: everything else stays)"})
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
			return apimodel.V2ValidationFailed("block ids are immutable",
				apimodel.V2Issue{Path: path, Message: "a block's id cannot change — insert a new block instead"})
		case "indent":
			return apimodel.V2ValidationFailed("indent is structural",
				apimodel.V2Issue{Path: path, Message: "indent cannot be set directly — use moveBlock to re-nest the block"})
		}
	}
	if raw, ok := op.Set["type"]; ok {
		var newType string
		if err := json.Unmarshal(raw, &newType); err != nil || newType == "" {
			return apimodel.V2ValidationFailed("invalid type value",
				apimodel.V2Issue{Path: opPath + ".set.type", Message: "type must be a non-empty string"})
		}
		if descendants := doc.subtreeEnd(idx) - idx - 1; descendants > 0 && anyblockjson.LeafBlockType(newType) {
			return leafWithDescendantsError(blockId(block), newType, descendants, opPath+".set.type")
		}
	}
	// merge on the JSON shape: the view block IS the block's exported form
	merged := map[string]any{}
	for k, v := range block {
		merged[k] = v
	}
	delete(merged, "indent") // position is the tree's business
	for _, key := range keys {
		var value any
		if err := json.Unmarshal(op.Set[key], &value); err != nil {
			return apimodel.V2ValidationFailed("invalid set value",
				apimodel.V2Issue{Path: opPath + ".set." + key, Message: err.Error()})
		}
		if value == nil {
			delete(merged, key) // explicit null clears the field (merge semantics)
			continue
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
	if err := a.checkFreshIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath + ".set" }); err != nil {
		return err
	}
	a.replaceLive(oldWasTable, blocks)
	return nil
}

func (a *v2StateApplier) applyReplaceBlock(op opReplaceBlock, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	if len(op.Block) == 0 {
		return apimodel.V2ValidationFailed("replaceBlock needs a block",
			apimodel.V2Issue{Path: opPath + ".block", Message: "block is required — the whole replacement block object"})
	}
	payload, err := decodeOpBlock(op.Block, opPath+".block")
	if err != nil {
		return err
	}
	if _, hasIndent := payload["indent"]; hasIndent {
		return apimodel.V2ValidationFailed("replaceBlock keeps the block's position",
			apimodel.V2Issue{Path: opPath + ".block.indent", Message: "indent is not accepted here — the block stays at its level; use moveBlock or replaceSubtree to restructure"})
	}
	old := doc.blocks[idx]
	fullId := blockId(old)
	if pid := blockId(payload); pid != "" && pid != fullId {
		return apimodel.V2ValidationFailed("the payload id must match the addressed block",
			apimodel.V2Issue{Path: opPath + ".block.id", Message: fmt.Sprintf("got %q, the addressed block is %q — omit id or repeat the addressed one", pid, fullId)})
	}
	if descendants := doc.subtreeEnd(idx) - idx - 1; descendants > 0 && anyblockjson.LeafBlockType(blockType(payload)) {
		return leafWithDescendantsError(fullId, blockType(payload), descendants, opPath+".block.type")
	}
	payload["id"] = fullId
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode replacement block: %w", err)
	}
	blocks, err := anyblockjson.UnmarshalBlock(raw, fullId, a.importOptions())
	if err != nil {
		return invalidFragmentError(opPath+".block", err)
	}
	if err := a.checkFreshIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath + ".block" }); err != nil {
		return err
	}
	a.replaceLive(blockType(old) == "table", blocks)
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
	run, err := a.decodePayloadRun(op.Blocks, opPath)
	if err != nil {
		return err
	}
	oldId := blockId(doc.blocks[idx])
	oldSubtree := collectSubtreeIds(a.st, oldId)
	blocks, topIds, err := a.fragmentBlocks(opPath+".blocks", run)
	if err != nil {
		return err
	}
	if err := a.checkFreshIds(blocks, oldSubtree, runPathFor(opPath, topIds)); err != nil {
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
// (insertBlocks and moveBlock, R14). It returns the anchor index, the mode
// ("after"|"before"|"inside"|"root") and the inside position
// ("first"|"last"). Omitting all three targeting fields means the document
// root: append at the end of the document (§8.2 v0.3.5) — the only ops-path
// into an object that has no addressable blocks yet (SPEC §7 keeps
// title/description out of the document, so an empty object has none). The
// anchor index is -1 in root mode.
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
		return 0, "", "", apimodel.V2AmbiguousInput("at most one of after, before, inside is allowed",
			apimodel.V2Issue{Path: opPath, Message: fmt.Sprintf("got %d targeting fields (%s)", len(refs), strings.Join(refs, ", "))})
	}
	if len(refs) == 0 {
		if position != "" {
			return 0, "", "", apimodel.V2ValidationFailed("position only applies to inside",
				apimodel.V2Issue{Path: opPath + ".position", Message: "position without a targeting field is meaningless — omitting after/before/inside appends at the end of the document"})
		}
		return -1, "root", "last", nil
	}
	pos = "last"
	if position != "" {
		if mode != "inside" {
			return 0, "", "", apimodel.V2ValidationFailed("position only applies to inside",
				apimodel.V2Issue{Path: opPath + ".position", Message: fmt.Sprintf("position with %q targeting is meaningless — after/before already name the slot", mode)})
		}
		if position != "first" && position != "last" {
			return 0, "", "", apimodel.V2ValidationFailed("invalid position",
				apimodel.V2Issue{Path: opPath + ".position", Message: fmt.Sprintf("unknown position %q", position), Hint: "allowed: first, last"})
		}
		pos = position
	}
	ref := after + before + inside // exactly one is non-empty
	anchor, err = a.resolveRef(doc, ref, opPath+"."+mode)
	if err != nil {
		return 0, "", "", err
	}
	if mode == "inside" {
		if typ := blockType(doc.blocks[anchor]); anyblockjson.LeafBlockType(typ) {
			return 0, "", "", apimodel.V2ValidationFailed(
				fmt.Sprintf("cannot target inside block %q — %q blocks cannot have children", ref, typ),
				apimodel.V2Issue{Path: opPath + ".inside", Message: "the target is a leaf block type (SPEC §5)"})
		}
	}
	return anchor, mode, pos, nil
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
	run, err := a.decodePayloadRun(op.Blocks, opPath)
	if err != nil {
		return err
	}
	blocks, topIds, err := a.fragmentBlocks(opPath+".blocks", run)
	if err != nil {
		return err
	}
	if err := a.checkFreshIds(blocks, nil, runPathFor(opPath, topIds)); err != nil {
		return err
	}
	a.setBlocks(blocks)
	// root mode: an empty anchor id makes InsertTo target the document root
	// (append at the end for any non-InnerFirst position)
	anchorId := ""
	if mode != "root" {
		anchorId = blockId(doc.blocks[anchor])
	}
	if err := a.st.InsertTo(anchorId, targetPosition(mode, pos), topIds...); err != nil {
		return fmt.Errorf("insert blocks at %q: %w", anchorId, err)
	}
	a.mutated()
	return nil
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
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("cannot move block %q inside its own subtree", op.Id),
			apimodel.V2Issue{Path: opPath, Message: "the target block is a descendant of (or is) the moved block — that would create a cycle; pick a target outside the moved subtree"})
	}
	fullId := blockId(doc.blocks[idx])
	// root mode (anchor -1, no targeting fields): move to the end of the
	// document root — the root can never be inside the moved subtree
	anchorId := ""
	if mode != "root" {
		anchorId = blockId(doc.blocks[anchor])
	}
	a.st.Unlink(fullId)
	if err := a.st.InsertTo(anchorId, targetPosition(mode, pos), fullId); err != nil {
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
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	end := doc.subtreeEnd(idx)
	if descendants := end - idx - 1; descendants > 0 && !op.Recursive {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q has %s — pass \"recursive\": true to delete the whole subtree", op.Id, countBlocks(descendants)),
			apimodel.V2Issue{Path: opPath, Message: "deleteBlock without recursive only deletes childless blocks", Hint: "or moveBlock the descendants out first"})
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
	idx, err := a.resolveRef(doc, op.Id, opPath+".id")
	if err != nil {
		return err
	}
	if op.Find == "" {
		return apimodel.V2ValidationFailed("find must not be empty",
			apimodel.V2Issue{Path: opPath + ".find", Message: "give the exact text to replace"})
	}
	block := doc.blocks[idx]
	typ := blockType(block)
	if !anyblockjson.TextBlockType(typ) {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q is a %q block and has no text", op.Id, typ),
			apimodel.V2Issue{Path: opPath + ".id", Message: "replaceText only applies to text-bearing blocks"})
	}
	// the find/replace runs on the block's document text — markup source for
	// text-bearing blocks, the literal text for code/embed (§8.4) — exactly
	// what the agent read
	text, _ := block["text"].(string)
	count := strings.Count(text, op.Find)
	switch {
	case count == 0:
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("no match found for %q in block %q — read the block and copy the find text exactly, including inline markup", op.Find, op.Id),
			apimodel.V2Issue{Path: opPath + ".find", Message: "0 matches in the block's text"})
	case count > 1 && !op.ReplaceAll:
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("found %d matches for %q in block %q — provide more context to make the match unique, or set \"replace_all\": true", count, op.Find, op.Id),
			apimodel.V2Issue{Path: opPath + ".find", Message: fmt.Sprintf("%d matches in the block's text", count)})
	}
	if op.ReplaceAll {
		text = strings.ReplaceAll(text, op.Find, op.Replace)
	} else {
		text = strings.Replace(text, op.Find, op.Replace, 1)
	}

	fullId := blockId(block)
	b := a.st.Pick(fullId)
	if b == nil {
		return fmt.Errorf("block %s not in state", fullId)
	}
	m := b.Copy().Model()
	switch content := m.Content.(type) {
	case *model.BlockContentOfText:
		if content.Text == nil {
			content.Text = &model.BlockContentText{}
		}
		if typ == "code" {
			content.Text.Text = text // literal (§8.4); code carries no marks
			content.Text.Marks = nil
		} else {
			plain, marks, err := anyblockjson.ParseInlineText(text)
			if err != nil {
				return invalidDocError(err)
			}
			content.Text.Text = plain
			if len(marks) == 0 {
				content.Text.Marks = nil
			} else {
				content.Text.Marks = &model.BlockContentTextMarks{Marks: marks}
			}
		}
	case *model.BlockContentOfLatex:
		if content.Latex == nil {
			content.Latex = &model.BlockContentLatex{}
		}
		content.Latex.Text = text // literal (§8.4)
	default:
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q is a %q block and has no text", op.Id, typ),
			apimodel.V2Issue{Path: opPath + ".id", Message: "replaceText only applies to text-bearing blocks"})
	}
	a.st.Set(simple.New(m))
	a.mutated()
	return nil
}

func (a *v2StateApplier) applySetCell(op opSetCell, opPath string) error {
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveRef(doc, op.TableId, opPath+".tableId")
	if err != nil {
		return err
	}
	table := doc.blocks[idx]
	if typ := blockType(table); typ != "table" {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q is a %q block, not a table", op.TableId, typ),
			apimodel.V2Issue{Path: opPath + ".tableId", Message: "setCell addresses a table block (SPEC §6.1)"})
	}
	if op.Value == nil {
		return apimodel.V2ValidationFailed("value is required",
			apimodel.V2Issue{Path: opPath + ".value", Message: "give the new cell content — a string, null (clear), a block object, or an array of blocks (SPEC §6.1)"})
	}
	var value any
	if err := json.Unmarshal(op.Value, &value); err != nil {
		return apimodel.V2ValidationFailed("invalid cell value",
			apimodel.V2Issue{Path: opPath + ".value", Message: err.Error()})
	}
	switch value.(type) {
	case nil, string, map[string]any, []any:
	default:
		return apimodel.V2ValidationFailed("invalid cell value",
			apimodel.V2Issue{Path: opPath + ".value", Message: "a cell is a string, null, a block object, or an array of blocks (SPEC §6.1)"})
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
	if err := a.checkFreshIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath + ".value" }); err != nil {
		return err
	}
	a.replaceLive(true, blocks)
	return nil
}

func (a *v2StateApplier) applyItems(op opItems, opPath string) error {
	if len(op.Items) == 0 {
		return apimodel.V2ValidationFailed(op.Op+" needs items",
			apimodel.V2Issue{Path: opPath + ".items", Message: "items must list at least one member object id"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	items := a.st.GetStoreSlice(template.CollectionStoreKey)
	if len(items) == 0 && !a.s.isCollectionType(a.spaceId, doc.docType()) {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("%s requires a collection — this object's type is %q", op.Op, doc.docType()),
			apimodel.V2Issue{Path: opPath, Message: "only collection objects carry items", Hint: "POST /v2/spaces/{spaceId}/collections creates one"})
	}
	if op.Op == "addItems" {
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
// after/before and replaceSubtree, the container's child level for inside).
// The run must obey the format's monotonicity (V1) internally; the indents
// stay run-relative — the state splice, not indent arithmetic, sets the
// insertion level. Missing ids are minted; every payload block's id lands in
// createdBlocks keyed by payload position.
func (a *v2StateApplier) decodePayloadRun(raws []json.RawMessage, opPath string) ([]map[string]any, error) {
	if len(raws) == 0 {
		return nil, apimodel.V2ValidationFailed("blocks must not be empty",
			apimodel.V2Issue{Path: opPath + ".blocks", Message: "give at least one block"})
	}
	run := make([]map[string]any, 0, len(raws))
	prev := -1
	for j, raw := range raws {
		path := fmt.Sprintf("%s.blocks[%d]", opPath, j)
		block, err := decodeOpBlock(raw, path)
		if err != nil {
			return nil, err
		}
		rel := 0
		if v, hasIndent := block["indent"]; hasIndent {
			f, isNum := v.(float64)
			if !isNum || f != float64(int(f)) || f < 0 {
				return nil, apimodel.V2ValidationFailed("invalid payload indent",
					apimodel.V2Issue{Path: path + ".indent", Message: "indent must be a non-negative integer, relative to the insertion level (0 = the anchor's level; for inside, 0 = the container's child level)"})
			}
			rel = int(f)
		}
		if j == 0 && rel != 0 {
			return nil, apimodel.V2ValidationFailed("the first payload block's indent must be 0",
				apimodel.V2Issue{Path: path + ".indent", Message: fmt.Sprintf("indent %d on the first block — payload indents are relative: 0 is the insertion level", rel)})
		}
		if rel > prev+1 {
			return nil, apimodel.V2ValidationFailed("payload indents must be monotonic",
				apimodel.V2Issue{Path: path + ".indent", Message: fmt.Sprintf("indent %d follows indent %d — a block can be at most one level deeper than its predecessor", rel, prev)})
		}
		prev = rel
		if rel > 0 {
			block["indent"] = float64(rel)
		} else {
			delete(block, "indent")
		}
		id := blockId(block)
		if id == "" {
			id = a.mintBlockId()
			block["id"] = id
		}
		a.createdBlocks[fmt.Sprintf("%s.blocks[%d]", opPath, j)] = id
		run = append(run, block)
	}
	return run, nil
}

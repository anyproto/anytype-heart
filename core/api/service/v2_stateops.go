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

// v2DebugValidateEdits gates the read-only post-op safety net: when set, the
// would-be document is validated after the ops and any issue is logged (not
// failed) — off by default so it is never on the damage path.
var v2DebugValidateEdits = os.Getenv("ANYTYPE_API_V2_VALIDATE_EDITS") == "1"

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

// marshalDoc renders the current state as its canonical flat document.
func (a *v2StateApplier) marshalDoc(onWarning func(anyblockjson.Issue)) ([]byte, error) {
	opts := storeresolver.New(a.s.store.SpaceIndex(a.spaceId)).Options()
	if onWarning == nil {
		onWarning = func(anyblockjson.Issue) {} // degrade, never fail, on view rebuilds
	}
	opts.OnWarning = onWarning
	doc, err := anyblockjson.Marshal(a.sbType, snapshotFromState(a.st), opts)
	if err != nil {
		return nil, fmt.Errorf("marshal object %s: %w", a.objectId, err)
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
	return doc, nil
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
	view, err := parseEditDoc(body)
	if err != nil {
		return nil, fmt.Errorf("object %s: %w", a.objectId, err)
	}
	a.view = view
	return view, nil
}

// mutated invalidates the view after a state mutation.
func (a *v2StateApplier) mutated() {
	a.view = nil
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

// debugValidateEditedDoc is the flag-gated safety net: validate the would-be
// document read-only and log (never fail) any issue.
func debugValidateEditedDoc(objectId string, doc []byte) {
	if !v2DebugValidateEdits {
		return
	}
	if err := anyblockjson.Validate(doc); err != nil {
		log.Warnf("api v2 edit safety net: object %s post-op document fails validation: %v", objectId, err)
	}
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
// Block_InnerFirst prepends (first child).
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
func (a *v2StateApplier) fragmentBlocks(run []map[string]any) ([]*model.Block, []string, error) {
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
		return nil, nil, invalidDocError(err)
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
	a.setBlocks(blocks)
	a.mutated()
}

//
// ---- setProperties ----
//

func (a *v2StateApplier) applySetProperties(op opSetProperties, opPath string) error {
	if len(op.Set) == 0 && len(op.Unset) == 0 {
		return apimodel.V2ValidationFailed("setProperties needs set and/or unset",
			apimodel.V2Issue{Path: opPath, Message: "both set and unset are empty"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	var issues []apimodel.V2Issue
	var known []string
	setKeys := make([]string, 0, len(op.Set))
	for key := range op.Set {
		setKeys = append(setKeys, key)
	}
	sort.Strings(setKeys)
	for _, key := range setKeys {
		path := opPath + ".set." + key
		switch {
		case key == "id" || key == "type":
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q is not a property — it is lifted to the document envelope and cannot be set here", key)})
		case v2OutputOnlyPropertyKeys[key]:
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only (SPEC §4a) — export writes it, writes must not", key)})
		default:
			if _, inDoc := doc.properties[key]; !inDoc && !a.s.propertyKeyExists(a.spaceId, key) {
				if known == nil {
					known = a.s.knownPropertyKeys(a.spaceId)
				}
				issues = append(issues, unknownPropertyIssue(key, path, known,
					fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", a.spaceId, a.spaceId)))
			}
		}
	}
	unset := map[string]bool{}
	for _, key := range op.Unset {
		path := opPath + ".unset." + key
		if v2OutputOnlyPropertyKeys[key] {
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only (SPEC §4a) and cannot be unset", key)})
			continue
		}
		if _, also := op.Set[key]; also {
			issues = append(issues, apimodel.V2Issue{Path: path,
				Message: fmt.Sprintf("%q appears in both set and unset — pick one", key)})
			continue
		}
		unset[key] = true
	}
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
		return invalidDocError(err)
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
		return invalidDocError(err)
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
	blocks, topIds, err := a.fragmentBlocks(run)
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
// ("after"|"before"|"inside") and the inside position ("first"|"last").
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
	if len(refs) != 1 {
		return 0, "", "", apimodel.V2AmbiguousInput("exactly one of after, before, inside is required",
			apimodel.V2Issue{Path: opPath, Message: fmt.Sprintf("got %d targeting fields (%s)", len(refs), strings.Join(refs, ", "))})
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
	blocks, topIds, err := a.fragmentBlocks(run)
	if err != nil {
		return err
	}
	if err := a.checkFreshIds(blocks, nil, runPathFor(opPath, topIds)); err != nil {
		return err
	}
	a.setBlocks(blocks)
	anchorId := blockId(doc.blocks[anchor])
	if err := a.st.InsertTo(anchorId, targetPosition(mode, pos), topIds...); err != nil {
		return fmt.Errorf("insert blocks at %s: %w", anchorId, err)
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
	anchorId := blockId(doc.blocks[anchor])
	a.st.Unlink(fullId)
	if err := a.st.InsertTo(anchorId, targetPosition(mode, pos), fullId); err != nil {
		return fmt.Errorf("move block %s to %s: %w", fullId, anchorId, err)
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

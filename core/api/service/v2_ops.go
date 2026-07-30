package service

// v2_ops.go is the Phase-3 PATCH op machinery (APIV2.md §2 Phase 3): the
// closed, id-addressed op set applied to the object's flat AnyBlock document
// between the live-state marshal and the diff-apply (v2_edit.go). Ops work
// on the document representation — the same shape agents read and the only
// place payload blocks (inline markup, tables) can be interpreted — and the
// edited document then passes the format's full validation (SPEC §12
// V1–V5), which is the normative post-op check (R5).

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2OpNames is the closed op set, in documentation order.
var v2OpNames = []string{
	"setProperties", "updateBlock", "replaceBlock", "replaceSubtree",
	"insertBlocks", "moveBlock", "deleteBlock", "replaceText", "setCell",
	"addItems", "removeItems",
}

// v2OutputOnlyPropertyKeys are the SPEC §4a output-only property keys a
// setProperties must reject. isFavorite is deliberately absent — SPEC §3
// marks it authorable.
var v2OutputOnlyPropertyKeys = map[string]bool{
	"coverId": true, "coverType": true, "createdDate": true,
	"lastModifiedDate": true, "creator": true, "isArchived": true,
	"resolvedLayout": true,
}

//
// ---- the editable document ----
//

// v2EditDoc is the decoded flat AnyBlock document the ops mutate.
type v2EditDoc struct {
	fields     map[string]json.RawMessage
	properties map[string]json.RawMessage
	items      []string
	blocks     []map[string]any
}

func parseEditDoc(body []byte) (*v2EditDoc, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, err
	}
	doc := &v2EditDoc{fields: fields, properties: map[string]json.RawMessage{}}
	if raw, ok := fields["properties"]; ok {
		if err := json.Unmarshal(raw, &doc.properties); err != nil {
			return nil, fmt.Errorf("decode properties: %w", err)
		}
	}
	if raw, ok := fields["items"]; ok {
		if err := json.Unmarshal(raw, &doc.items); err != nil {
			return nil, fmt.Errorf("decode items: %w", err)
		}
	}
	if raw, ok := fields["blocks"]; ok {
		if err := json.Unmarshal(raw, &doc.blocks); err != nil {
			return nil, fmt.Errorf("decode blocks: %w", err)
		}
	}
	return doc, nil
}

// encode re-emits the edited document, dropping empty containers.
func (d *v2EditDoc) encode() ([]byte, error) {
	var err error
	setOrDelete := func(key string, empty bool, v any) {
		if err != nil {
			return
		}
		if empty {
			delete(d.fields, key)
			return
		}
		d.fields[key], err = rawJSON(v)
	}
	setOrDelete("properties", len(d.properties) == 0, d.properties)
	setOrDelete("items", len(d.items) == 0, d.items)
	setOrDelete("blocks", len(d.blocks) == 0, d.blocks)
	if err != nil {
		return nil, err
	}
	return encodeEnvelope(d.fields)
}

func blockIndent(b map[string]any) int {
	if v, ok := b["indent"].(float64); ok {
		return int(v)
	}
	return 0
}

func blockId(b map[string]any) string {
	s, _ := b["id"].(string)
	return s
}

func blockType(b map[string]any) string {
	s, _ := b["type"].(string)
	return s
}

func (d *v2EditDoc) blockIds() []string {
	ids := make([]string, len(d.blocks))
	for i, b := range d.blocks {
		ids[i] = blockId(b)
	}
	return ids
}

// subtreeEnd returns the index just past block i's contiguous descendant run.
func (d *v2EditDoc) subtreeEnd(i int) int {
	base := blockIndent(d.blocks[i])
	j := i + 1
	for j < len(d.blocks) && blockIndent(d.blocks[j]) > base {
		j++
	}
	return j
}

// docType returns the envelope type key.
func (d *v2EditDoc) docType() string {
	var t string
	if raw, ok := d.fields["type"]; ok {
		_ = json.Unmarshal(raw, &t)
	}
	return t
}

//
// ---- op decoding ----
//

type opSetProperties struct {
	Op    string                     `json:"op"`
	Set   map[string]json.RawMessage `json:"set"`
	Unset []string                   `json:"unset"`
}

type opUpdateBlock struct {
	Op  string                     `json:"op"`
	Id  string                     `json:"id"`
	Set map[string]json.RawMessage `json:"set"`
}

type opReplaceBlock struct {
	Op    string          `json:"op"`
	Id    string          `json:"id"`
	Block json.RawMessage `json:"block"`
}

type opReplaceSubtree struct {
	Op     string            `json:"op"`
	Id     string            `json:"id"`
	Blocks []json.RawMessage `json:"blocks"`
}

type opInsertBlocks struct {
	Op       string            `json:"op"`
	After    string            `json:"after"`
	Before   string            `json:"before"`
	Inside   string            `json:"inside"`
	Position string            `json:"position"`
	Blocks   []json.RawMessage `json:"blocks"`
}

type opMoveBlock struct {
	Op       string `json:"op"`
	Id       string `json:"id"`
	After    string `json:"after"`
	Before   string `json:"before"`
	Inside   string `json:"inside"`
	Position string `json:"position"`
}

type opDeleteBlock struct {
	Op        string `json:"op"`
	Id        string `json:"id"`
	Recursive bool   `json:"recursive"`
}

type opReplaceText struct {
	Op         string `json:"op"`
	Id         string `json:"id"`
	Find       string `json:"find"`
	Replace    string `json:"replace"`
	ReplaceAll bool   `json:"replace_all"`
}

type opSetCell struct {
	Op      string          `json:"op"`
	TableId string          `json:"tableId"`
	Row     string          `json:"row"`
	Col     string          `json:"col"`
	Value   json.RawMessage `json:"value"`
}

type opItems struct {
	Op    string   `json:"op"`
	Items []string `json:"items"`
}

// decodeStrictOp decodes one op body into its typed struct, rejecting
// unknown fields with a schema pointer.
func decodeStrictOp(raw json.RawMessage, opName, opPath string, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apimodel.V2ValidationFailed(fmt.Sprintf("invalid %s op", opName),
			apimodel.V2Issue{
				Path:    opPath,
				Message: err.Error(),
				Hint:    fmt.Sprintf("GET /v2/schemas/ops/%s for the op's schema and example", opName),
			})
	}
	return nil
}

//
// ---- the applier ----
//

// v2PatchApplier applies the ops of one PATCH to one edit doc.
type v2PatchApplier struct {
	s       *V2Service
	spaceId string
	doc     *v2EditDoc
	// createdBlocks is the response id map, keyed by payload position
	// ("ops[3].blocks[0]") — minted ids and echoed client-supplied ones.
	createdBlocks map[string]string
	usedIds       map[string]bool
}

func newV2PatchApplier(s *V2Service, spaceId string, doc *v2EditDoc) *v2PatchApplier {
	used := map[string]bool{}
	for _, id := range doc.blockIds() {
		used[id] = true
	}
	return &v2PatchApplier{s: s, spaceId: spaceId, doc: doc, createdBlocks: map[string]string{}, usedIds: used}
}

// mintBlockId mints an editor-shaped 24-hex block id unused in this document.
func (a *v2PatchApplier) mintBlockId() string {
	for {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(fmt.Errorf("random block id: %w", err))
		}
		id := hex.EncodeToString(b[:])
		if !a.usedIds[id] {
			a.usedIds[id] = true
			return id
		}
	}
}

// apply dispatches one op. i is the op's position (error paths are
// "ops[i]…", R5).
func (a *v2PatchApplier) apply(i int, raw json.RawMessage) error {
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
// with op-scoped errors.
func (a *v2PatchApplier) resolveRef(ref, path string) (int, error) {
	idx, matches := matchBlockRef(a.doc.blockIds(), ref)
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
// ---- setProperties ----
//

func (a *v2PatchApplier) applySetProperties(op opSetProperties, opPath string) error {
	if len(op.Set) == 0 && len(op.Unset) == 0 {
		return apimodel.V2ValidationFailed("setProperties needs set and/or unset",
			apimodel.V2Issue{Path: opPath, Message: "both set and unset are empty"})
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
			if _, inDoc := a.doc.properties[key]; !inDoc && !a.s.propertyKeyExists(a.spaceId, key) {
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
		a.doc.properties[key] = op.Set[key]
	}
	for key := range unset {
		delete(a.doc.properties, key) // unsetting an absent key is a no-op
	}
	return nil
}

//
// ---- block ops ----
//

// countBlocks renders "1 descendant block" / "3 descendant blocks".
func countBlocks(n int) string {
	if n == 1 {
		return "1 descendant block"
	}
	return fmt.Sprintf("%d descendant blocks", n)
}

// leafWithDescendantsError names the descendant count when a type change
// would turn a parent into a leaf (R5).
func leafWithDescendantsError(id, newType string, descendants int, path string) error {
	return apimodel.V2ValidationFailed(
		fmt.Sprintf("cannot change block %q to leaf type %q — it has %s; %q blocks cannot have children", id, newType, countBlocks(descendants), newType),
		apimodel.V2Issue{Path: path, Message: "move or delete the descendants first, or use replaceSubtree"})
}

func (a *v2PatchApplier) applyUpdateBlock(op opUpdateBlock, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
	if err != nil {
		return err
	}
	if len(op.Set) == 0 {
		return apimodel.V2ValidationFailed("updateBlock needs a non-empty set",
			apimodel.V2Issue{Path: opPath + ".set", Message: "set is empty — name the fields to change (merge semantics: everything else stays)"})
	}
	block := a.doc.blocks[idx]
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
		if descendants := a.doc.subtreeEnd(idx) - idx - 1; descendants > 0 && anyblockjson.LeafBlockType(newType) {
			return leafWithDescendantsError(blockId(block), newType, descendants, opPath+".set.type")
		}
	}
	for _, key := range keys {
		var value any
		if err := json.Unmarshal(op.Set[key], &value); err != nil {
			return apimodel.V2ValidationFailed("invalid set value",
				apimodel.V2Issue{Path: opPath + ".set." + key, Message: err.Error()})
		}
		if value == nil {
			delete(block, key) // explicit null clears the field (merge semantics)
			continue
		}
		block[key] = value
	}
	return nil
}

func (a *v2PatchApplier) applyReplaceBlock(op opReplaceBlock, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
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
	old := a.doc.blocks[idx]
	fullId := blockId(old)
	if pid := blockId(payload); pid != "" && pid != fullId {
		return apimodel.V2ValidationFailed("the payload id must match the addressed block",
			apimodel.V2Issue{Path: opPath + ".block.id", Message: fmt.Sprintf("got %q, the addressed block is %q — omit id or repeat the addressed one", pid, fullId)})
	}
	if descendants := a.doc.subtreeEnd(idx) - idx - 1; descendants > 0 && anyblockjson.LeafBlockType(blockType(payload)) {
		return leafWithDescendantsError(fullId, blockType(payload), descendants, opPath+".block.type")
	}
	payload["id"] = fullId
	if indent := blockIndent(old); indent > 0 {
		payload["indent"] = float64(indent)
	}
	a.doc.blocks[idx] = payload
	return nil
}

func (a *v2PatchApplier) applyReplaceSubtree(op opReplaceSubtree, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
	if err != nil {
		return err
	}
	end := a.doc.subtreeEnd(idx)
	base := blockIndent(a.doc.blocks[idx])
	run, err := a.decodePayloadRun(op.Blocks, base, opPath)
	if err != nil {
		return err
	}
	a.doc.blocks = append(a.doc.blocks[:idx], append(run, a.doc.blocks[end:]...)...)
	return nil
}

// resolveTarget resolves the shared after/before/inside targeting vocabulary
// (insertBlocks and moveBlock, R14). It returns the anchor index, the mode
// ("after"|"before"|"inside") and the inside position ("first"|"last").
func (a *v2PatchApplier) resolveTarget(after, before, inside, position, opPath string) (anchor int, mode string, pos string, err error) {
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
	anchor, err = a.resolveRef(ref, opPath+"."+mode)
	if err != nil {
		return 0, "", "", err
	}
	if mode == "inside" {
		if typ := blockType(a.doc.blocks[anchor]); anyblockjson.LeafBlockType(typ) {
			return 0, "", "", apimodel.V2ValidationFailed(
				fmt.Sprintf("cannot target inside block %q — %q blocks cannot have children", ref, typ),
				apimodel.V2Issue{Path: opPath + ".inside", Message: "the target is a leaf block type (SPEC §5)"})
		}
	}
	return anchor, mode, pos, nil
}

// insertionPoint maps a resolved target to (splice index, base indent). The
// R3 relative-indent rule: payload indent 0 = the anchor's level for
// after/before, the anchor's child level for inside.
func (a *v2PatchApplier) insertionPoint(anchor int, mode, pos string) (at, base int) {
	base = blockIndent(a.doc.blocks[anchor])
	switch mode {
	case "before":
		return anchor, base
	case "after":
		return a.doc.subtreeEnd(anchor), base
	default: // inside
		if pos == "first" {
			return anchor + 1, base + 1
		}
		return a.doc.subtreeEnd(anchor), base + 1
	}
}

func (a *v2PatchApplier) applyInsertBlocks(op opInsertBlocks, opPath string) error {
	anchor, mode, pos, err := a.resolveTarget(op.After, op.Before, op.Inside, op.Position, opPath)
	if err != nil {
		return err
	}
	at, base := a.insertionPoint(anchor, mode, pos)
	run, err := a.decodePayloadRun(op.Blocks, base, opPath)
	if err != nil {
		return err
	}
	rest := append([]map[string]any(nil), a.doc.blocks[at:]...)
	a.doc.blocks = append(append(a.doc.blocks[:at], run...), rest...)
	return nil
}

func (a *v2PatchApplier) applyMoveBlock(op opMoveBlock, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
	if err != nil {
		return err
	}
	end := a.doc.subtreeEnd(idx)
	anchor, mode, pos, err := a.resolveTarget(op.After, op.Before, op.Inside, op.Position, opPath)
	if err != nil {
		return err
	}
	if anchor >= idx && anchor < end {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("cannot move block %q inside its own subtree", op.Id),
			apimodel.V2Issue{Path: opPath, Message: "the target block is a descendant of (or is) the moved block — that would create a cycle; pick a target outside the moved subtree"})
	}
	run := append([]map[string]any(nil), a.doc.blocks[idx:end]...)
	oldIndent := blockIndent(run[0])
	a.doc.blocks = append(a.doc.blocks[:idx], a.doc.blocks[end:]...)
	if anchor > idx {
		anchor -= end - idx
	}
	at, base := a.insertionPoint(anchor, mode, pos)
	delta := base - oldIndent
	for _, b := range run {
		if newIndent := blockIndent(b) + delta; newIndent > 0 {
			b["indent"] = float64(newIndent)
		} else {
			delete(b, "indent")
		}
	}
	rest := append([]map[string]any(nil), a.doc.blocks[at:]...)
	a.doc.blocks = append(append(a.doc.blocks[:at], run...), rest...)
	return nil
}

func (a *v2PatchApplier) applyDeleteBlock(op opDeleteBlock, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
	if err != nil {
		return err
	}
	end := a.doc.subtreeEnd(idx)
	if descendants := end - idx - 1; descendants > 0 && !op.Recursive {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q has %s — pass \"recursive\": true to delete the whole subtree", op.Id, countBlocks(descendants)),
			apimodel.V2Issue{Path: opPath, Message: "deleteBlock without recursive only deletes childless blocks", Hint: "or moveBlock the descendants out first"})
	}
	if !op.Recursive {
		end = idx + 1
	}
	a.doc.blocks = append(a.doc.blocks[:idx], a.doc.blocks[end:]...)
	return nil
}

func (a *v2PatchApplier) applyReplaceText(op opReplaceText, opPath string) error {
	idx, err := a.resolveRef(op.Id, opPath+".id")
	if err != nil {
		return err
	}
	if op.Find == "" {
		return apimodel.V2ValidationFailed("find must not be empty",
			apimodel.V2Issue{Path: opPath + ".find", Message: "give the exact text to replace"})
	}
	block := a.doc.blocks[idx]
	typ := blockType(block)
	if !anyblockjson.TextBlockType(typ) {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("block %q is a %q block and has no text", op.Id, typ),
			apimodel.V2Issue{Path: opPath + ".id", Message: "replaceText only applies to text-bearing blocks"})
	}
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
	if text == "" {
		delete(block, "text")
	} else {
		block["text"] = text
	}
	return nil
}

func (a *v2PatchApplier) applySetCell(op opSetCell, opPath string) error {
	idx, err := a.resolveRef(op.TableId, opPath+".tableId")
	if err != nil {
		return err
	}
	table := a.doc.blocks[idx]
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
	rows, _ := table["rows"].([]any)
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
	table["rows"] = rows
	return nil
}

// resolveTablePart resolves a row/column reference (exact id or unique
// suffix — the same C4 leniency as block refs) within one table.
func resolveTablePart(table map[string]any, kind, ref, tableRef, path string) (int, error) {
	entries, _ := table[kind].([]any)
	ids := make([]string, len(entries))
	for i, e := range entries {
		if m, ok := e.(map[string]any); ok {
			ids[i], _ = m["id"].(string)
		}
	}
	idx, matches := matchBlockRef(ids, ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		return -1, apimodel.V2AmbiguousInput(
			fmt.Sprintf("%s reference %q matches more than one %s in table %q — use the full id", strings.TrimSuffix(kind, "s"), ref, strings.TrimSuffix(kind, "s"), tableRef),
			apimodel.V2Issue{Path: path, Message: "the reference is a suffix of several ids"})
	default:
		listed := ids
		if len(listed) > maxListedKeys {
			listed = listed[:maxListedKeys]
		}
		return -1, apimodel.V2NotFound(
			fmt.Sprintf("%s %q not found in table %q — %s: %s", strings.TrimSuffix(kind, "s"), ref, tableRef, kind, strings.Join(listed, ", ")))
	}
}

func (a *v2PatchApplier) applyItems(op opItems, opPath string) error {
	if len(op.Items) == 0 {
		return apimodel.V2ValidationFailed(op.Op+" needs items",
			apimodel.V2Issue{Path: opPath + ".items", Message: "items must list at least one member object id"})
	}
	if len(a.doc.items) == 0 && !a.s.isCollectionType(a.spaceId, a.doc.docType()) {
		return apimodel.V2ValidationFailed(
			fmt.Sprintf("%s requires a collection — this object's type is %q", op.Op, a.doc.docType()),
			apimodel.V2Issue{Path: opPath, Message: "only collection objects carry items", Hint: "POST /v2/spaces/{spaceId}/collections creates one"})
	}
	if op.Op == "addItems" {
		present := map[string]bool{}
		for _, id := range a.doc.items {
			present[id] = true
		}
		for _, id := range op.Items {
			if !present[id] {
				present[id] = true
				a.doc.items = append(a.doc.items, id)
			}
		}
		return nil
	}
	remove := map[string]bool{}
	for _, id := range op.Items {
		remove[id] = true
	}
	kept := a.doc.items[:0]
	for _, id := range a.doc.items {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	a.doc.items = kept
	return nil
}

//
// ---- payload decoding ----
//

// decodeOpBlock decodes one payload block object.
func decodeOpBlock(raw json.RawMessage, path string) (map[string]any, error) {
	var block map[string]any
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, apimodel.V2ValidationFailed("a payload block must be a JSON object",
			apimodel.V2Issue{Path: path, Message: err.Error()})
	}
	if typ := blockType(block); typ == "" {
		return nil, apimodel.V2ValidationFailed("a payload block needs a type",
			apimodel.V2Issue{Path: path + ".type", Message: "type is required (SPEC §5 lists the inventory)"})
	}
	return block, nil
}

// decodePayloadRun decodes a flat payload run with R3 relative indents:
// payload indent 0 = base (the anchor's level for after/before and
// replaceSubtree, the container's child level for inside). The run must obey
// the format's monotonicity (V1) internally. Missing ids are minted;
// every payload block's id lands in createdBlocks keyed by payload position.
func (a *v2PatchApplier) decodePayloadRun(raws []json.RawMessage, base int, opPath string) ([]map[string]any, error) {
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
		if abs := base + rel; abs > 0 {
			block["indent"] = float64(abs)
		} else {
			delete(block, "indent")
		}
		id := blockId(block)
		if id == "" {
			id = a.mintBlockId()
			block["id"] = id
		} else {
			a.usedIds[id] = true
		}
		a.createdBlocks[fmt.Sprintf("%s.blocks[%d]", opPath, j)] = id
		run = append(run, block)
	}
	return run, nil
}

//
// ---- space lookups ----
//

// isCollectionType reports whether a type key is the collection type or a
// custom type with the collection layout.
func (s *V2Service) isCollectionType(spaceId, typeKey string) bool {
	if typeKey == string(bundle.TypeKeyCollection) {
		return true
	}
	typeId, ok := s.typeIdInSpace(spaceId, typeKey)
	if !ok {
		return false
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return false
	}
	return details.GetInt64(bundle.RelationKeyRecommendedLayout) == int64(model.ObjectType_collection)
}

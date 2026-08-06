package v2service

// ops.go holds the Phase-3 PATCH op vocabulary (APIV2.md §2 Phase 3):
// the closed, id-addressed op set, its strict decoding, and the read-only
// document view the ops address blocks through. The view is the live
// state's flat AnyBlock rendering — the same shape agents read, so
// references (full ids, unique suffixes), indents and error texts all speak
// the document language. The actual mutations happen on the object's child
// state (stateops.go); nothing here writes the view.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2OpNames is the closed op set, in documentation order. replaceBlock was
// deliberately folded into updateBlock before release (v0.3.5): four routes
// to changing a block's text was the surface's largest disambiguation load,
// and replaceBlock's silent text-wipe (a checkbox toggle losing the text)
// was the documented small-model trap. updateBlock's merge-with-null-clears
// expresses everything replaceBlock did except the wipe.
var v2OpNames = []string{
	"setProperties", "updateBlock", "replaceSubtree",
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

// v2ListShapedFormats are the property formats whose SPEC §3 value encoding
// is a list — the only formats setProperties add/remove apply to.
var v2ListShapedFormats = map[model.RelationFormat]bool{
	model.RelationFormat_status: true, // select
	model.RelationFormat_tag:    true, // multiSelect
	model.RelationFormat_object: true, // objects
	model.RelationFormat_file:   true, // files
}

// v2ListShapedFormatNames is the agent-facing list for the rejection text.
const v2ListShapedFormatNames = "select, multiSelect, objects, files"

//
// ---- the document view ----
//

// v2EditDoc is the decoded flat AnyBlock document the ops read: the
// addressing view (ids, types, indents) and the reference sets the
// validations consult.
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
	// Add/Remove are the per-key list edits (v0.3.5): append entries to /
	// delete entries from a list-shaped property without rewriting the whole
	// array. Values are arrays of entries (option names or object/file ids).
	Add    map[string]json.RawMessage `json:"add"`
	Remove map[string]json.RawMessage `json:"remove"`
}

type opUpdateBlock struct {
	Op  string                     `json:"op"`
	Id  string                     `json:"id"`
	Set map[string]json.RawMessage `json:"set"`
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
	// Markdown is the authoring-channel alternative to Blocks (§7.1, v0.4):
	// the server parses it into a flat run (anyblockjson.ParseMarkdownBlocks)
	// and the op proceeds exactly as if that run had been supplied as Blocks —
	// same targeting (incl. root-append), validation, createdBlocks and
	// diffStats. Mutually exclusive with Blocks.
	Markdown string `json:"markdown"`
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
		return v2model.V2ValidationFailed(fmt.Sprintf("invalid %s op", opName),
			v2model.V2Issue{
				Path:    opPath,
				Message: err.Error(),
				Hint:    fmt.Sprintf("GET /v2/schemas/ops/%s for the op's schema and example", opName),
			})
	}
	return nil
}

//
// ---- shared op helpers ----
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
	return v2model.V2ValidationFailed(
		fmt.Sprintf("cannot change block %q to leaf type %q — it has %s; %q blocks cannot have children", id, newType, countBlocks(descendants), newType),
		v2model.V2Issue{Path: path, Message: "move or delete the descendants first, or use replaceSubtree"})
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
		return -1, v2model.V2AmbiguousInput(
			fmt.Sprintf("%s reference %q matches more than one %s in table %q — use the full id", strings.TrimSuffix(kind, "s"), ref, strings.TrimSuffix(kind, "s"), tableRef),
			v2model.V2Issue{Path: path, Message: "the reference is a suffix of several ids"})
	default:
		listed := ids
		if len(listed) > maxListedKeys {
			listed = listed[:maxListedKeys]
		}
		return -1, v2model.V2NotFound(
			fmt.Sprintf("%s %q not found in table %q — %s: %s", strings.TrimSuffix(kind, "s"), ref, tableRef, kind, strings.Join(listed, ", ")))
	}
}

// decodeOpBlock decodes one payload block object.
func decodeOpBlock(raw json.RawMessage, path string) (map[string]any, error) {
	var block map[string]any
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, v2model.V2ValidationFailed("a payload block must be a JSON object",
			v2model.V2Issue{Path: path, Message: err.Error()})
	}
	if typ := blockType(block); typ == "" {
		return nil, v2model.V2ValidationFailed("a payload block needs a type",
			v2model.V2Issue{Path: path + ".type", Message: "type is required (SPEC §5 lists the inventory)"})
	}
	return block, nil
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

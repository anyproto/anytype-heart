package anyblockjson

// fragment.go exposes the conversion machinery at fragment granularity —
// single blocks, flat runs, one property value, the inline codec — for
// wiring that edits a live document op-by-op instead of round-tripping the
// whole document (API v2 PATCH, APIV2.md §2 Phase 3).
//
// Fragment validation reuses the document validation wholesale: a run is
// wrapped into a minimal synthetic document and validated there, so V1
// monotonicity and the §5 per-type shape checks apply exactly as on a whole
// document. Two fragment-specific rules on top:
//   - structural block types (title/description/featuredProperties, §7) are
//     rejected explicitly — a fragment has no document to absorb them into,
//     and the import path's silent top-level absorption must not fire;
//   - no primary-dataview pinning (§7): a fragment never names the
//     document's own dataview, so no block is renamed to the "dataview" id.

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fragmentStructuralTypes are the §7 structural block types a fragment must
// not carry (the whole-document import absorbs or drops them silently; a
// fragment rejects them loudly instead).
var fragmentStructuralTypes = map[string]bool{
	"title": true, "description": true, "featuredProperties": true,
}

// validateFragmentRun wraps the run in a minimal synthetic page document and
// runs the document validation, then applies the fragment-specific
// structural-type rejection. Issue paths are /blocks/i/… — run-relative.
func validateFragmentRun(run []json.RawMessage, opts Options) ([]*jsonBlock, error) {
	payload, err := json.Marshal(map[string]any{
		"version": FormatVersion,
		"type":    "page",
		"blocks":  run,
	})
	if err != nil {
		return nil, fmt.Errorf("build synthetic fragment document: %w", err)
	}
	if _, err := validateToDoc(payload, opts.NormalizeIndent, opts.OnWarning); err != nil {
		return nil, err
	}
	jbs := make([]*jsonBlock, 0, len(run))
	var issues []Issue
	for i, raw := range run {
		var jb jsonBlock
		if err := json.Unmarshal(raw, &jb); err != nil {
			return nil, fmt.Errorf("decode block %d: %w", i, err)
		}
		if fragmentStructuralTypes[jb.Type] {
			issues = append(issues, Issue{
				Path:    fmt.Sprintf("/blocks/%d/type", i),
				Message: fmt.Sprintf("%q is a structural block (§7) — the editor owns it; it cannot appear in a fragment", jb.Type),
			})
		}
		jbs = append(jbs, &jb)
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	return jbs, nil
}

// UnmarshalBlocks converts a flat run of AnyBlock JSON blocks (§4) into
// model blocks. Indents are run-relative: 0 is the run's top level, and the
// run must obey V1 monotonicity internally (the first block at 0, each block
// at most one level deeper than its predecessor). The returned blocks slice
// holds every created block — for tables that includes the internal subtree
// (§6.1) — with the ChildrenIds graph already wired; topIds names the run's
// top-level blocks in order, ready for a state splice. Blocks without ids
// get generated ones (Options.GenerateId). Errors wrap *ValidationError
// with run-relative /blocks/i/… paths.
func UnmarshalBlocks(run []json.RawMessage, opts Options) (blocks []*model.Block, topIds []string, err error) {
	jbs, err := validateFragmentRun(run, opts)
	if err != nil {
		return nil, nil, err
	}
	imp := &importer{opts: opts, doc: &jsonDoc{}}
	root := &model.Block{}
	blocks, err = imp.flatSubtree(jbs, imp.blockIndents(jbs, -1), root, -1)
	if err != nil {
		return nil, nil, fmt.Errorf("build fragment blocks: %w", err)
	}
	return blocks, root.ChildrenIds, nil
}

// UnmarshalBlock converts one AnyBlock JSON block object into its model
// block(s): the addressed block first, followed by any internal blocks it
// owns (the table subtree, §6.1). forcedId, when non-empty, overrides the
// payload id — the edit path uses it to keep a replaced block's identity.
// The block is validated like a one-element run (so §5 shape checks apply);
// structural types are rejected. An indent field, if present, must be 0
// (V1 on the synthetic run).
func UnmarshalBlock(raw json.RawMessage, forcedId string, opts Options) ([]*model.Block, error) {
	jbs, err := validateFragmentRun([]json.RawMessage{raw}, opts)
	if err != nil {
		return nil, err
	}
	imp := &importer{opts: opts, doc: &jsonDoc{}}
	blocks, err := imp.blockFromJSON(jbs[0], forcedId)
	if err != nil {
		return nil, fmt.Errorf("build fragment block: %w", err)
	}
	return blocks, nil
}

// UnmarshalPropertyValue decodes one property value per its resolved §3
// format rules (dates parse, select option names resolve/create through
// Options.ResolveOptions, object/file refs resolve, scalars of list-shaped
// formats wrap into lists). It is the import twin of MarshalPropertyValue.
// A nil v yields an explicit null value (presence is preserved, §3).
func UnmarshalPropertyValue(key string, v any, opts Options) *types.Value {
	imp := &importer{opts: opts, doc: &jsonDoc{}}
	return imp.propertyValue(key, v)
}

// MarshalBlockSubtree serializes one block subtree into its flat JSON run
// (§4): subtree[0] is the root, emitted at indent 0; the remaining entries
// back the root's ChildrenIds graph (entries not reachable from the root are
// ignored, ids the slice does not carry are skipped — the same leniency as
// a whole-document export). Tables need their internal blocks in the slice
// to render (§6.1). The result is a compact JSON array of block objects.
func MarshalBlockSubtree(subtree []*model.Block, opts Options) (json.RawMessage, error) {
	if len(subtree) == 0 || subtree[0] == nil || subtree[0].Id == "" {
		return nil, fmt.Errorf("empty subtree")
	}
	e := &exporter{
		opts:     opts,
		snapshot: &model.SmartBlockSnapshotBase{Blocks: subtree},
		blocks:   map[string]*model.Block{},
		visited:  map[string]bool{},
	}
	e.indexBlocks()
	if opts.compactObjectRefs() || opts.compactBlockLabels() {
		e.buildCompactIds()
	}
	var out []any
	// topLevel=false: a fragment caller addresses the blocks explicitly, so
	// even a structural root renders rather than silently vanishing
	if err := e.appendBlocksFlat(&out, []string{subtree[0].Id}, 0, false); err != nil {
		return nil, fmt.Errorf("marshal block subtree: %w", err)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode block subtree: %w", err)
	}
	return data, nil
}

// ParseInlineText parses §8 inline Markdown into plain text and marks — the
// single-field import codec. Mention/object mark params stay as written
// (fragment callers resolve refs themselves; whole-document import resolves
// them through the refs legend).
func ParseInlineText(md string) (string, []*model.BlockContentTextMark, error) {
	return parseInline(md)
}

// RenderInlineText renders plain text and marks back into §8 inline
// Markdown — the single-field export codec, the exact inverse used by
// Marshal for every text-bearing block.
func RenderInlineText(text string, marks []*model.BlockContentTextMark) string {
	return renderInline(text, marks)
}

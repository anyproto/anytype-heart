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
	"title": true, "description": true, "featured_properties": true,
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
	imp := &importer{opts: opts, doc: opts.fragmentDoc()}
	root := &model.Block{}
	// §7a: the write path lifts too, or an API caller that pasted a `group`
	// mints a Layout_Div straight into a live object — one no read will ever
	// show and normalization never removes while it has children
	liftedJbs, liftedIndents := liftTransparentContainers(jbs, imp.blockIndents(jbs, -1))
	blocks, err = imp.flatSubtree(liftedJbs, liftedIndents, root, -1)
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
	// §7a: this entry point's contract is exactly one block, and a
	// transparent container is not one. Returning zero blocks would leave
	// the caller's edit silently unapplied — a replaceBlock that replaced
	// nothing — so it is named as the error it is.
	if transparentBlockTypes[jbs[0].Type] {
		return nil, &ValidationError{Issues: []Issue{{
			Path: "/blocks/0/type",
			Message: fmt.Sprintf("%q is a transparent container (§7a) — it contributes no block of its own, "+
				"so it cannot be the one block this call addresses", jbs[0].Type),
		}}}
	}
	imp := &importer{opts: opts, doc: opts.fragmentDoc()}
	blocks, err := imp.blockFromJSON(jbs[0], forcedId)
	if err != nil {
		return nil, fmt.Errorf("build fragment block: %w", err)
	}
	return blocks, nil
}

// UnmarshalPropertyValue decodes one property value per its resolved §3
// format rules (dates parse, select option names resolve/create through
// Options.ResolveOptions, object/file ids pass through, scalars of
// list-shaped formats wrap into lists). It is the import twin of
// MarshalPropertyValue. A nil v yields an explicit null value (presence is
// preserved, §3).
//
// `key` is a STORED key here, not a spelling — a value-level caller holds
// the key it is writing — so the property legend has nothing to do. The
// OPTION legend does: hand this call the {option name: option id} map for
// this key through Options.Legend.OptionIds, and a select value resolves by
// id first (§3 step 1) instead of by name. Without it a name shared by two
// options lands on whichever answers first, and an option renamed since the
// value was written mints a second option under the stale name — the two
// losses §9a exists to close, which this entry point had no way to avoid.
func UnmarshalPropertyValue(key string, v any, opts Options) *types.Value {
	imp := &importer{opts: opts, doc: opts.fragmentDoc()}
	return imp.propertyValue(key, key, v)
}

// MarshalBlockSubtree serializes one block subtree into a fragment envelope
// (§4): `blocks` is the flat run — subtree[0] is the root, emitted at indent
// 0, and the remaining entries back the root's ChildrenIds graph (entries not
// reachable from the root are ignored, ids the slice does not carry are
// skipped, the same leniency as a whole-document export; tables need their
// internal blocks in the slice to render, §6.1) — beside the legends those
// blocks owe, in the envelope's own member order:
//
//	{"property_keys": {…}, "type_keys": {…}, "option_ids": {…}, "blocks": […]}
//
// **The legends are why this is an object rather than the bare array it used
// to be.** A block run names properties at seven slots and options at two,
// and it names them in the DOCUMENT's spelling — which is the writer's
// vocabulary, not the reader's. The exporter computed all three legends here
// all along and discarded them at the return, so a `property` block came back
// as `{"key": "priority"}` with nothing saying which relation `priority` is,
// where the same block inside a whole document carries
// `property_keys: {"priority": "6a32d485…"}`. A reader resolved it through
// its own table, which is precisely the misresolution §3 wrote the legend to
// prevent. Feed the three maps to the reading side through Options.Legend.
//
// **OmitIds and the compaction flags are refused, not ignored.** This surface
// exists for wiring that edits a live document op-by-op, and both destroy the
// addresses that wiring runs on: OmitIds drops every block id, the view id
// and the filter id, so the run says what to write but not where; the block
// relabeling rewrites doc-local ids to short suffixes that are meaningful
// only inside the emitted run and are not the object's ids at all. Silently
// honouring either produced a fragment that reads correctly and cannot be
// applied. A caller that wants an id-less or relabeled rendering wants
// Marshal on the whole document.
func MarshalBlockSubtree(subtree []*model.Block, opts Options) (json.RawMessage, error) {
	if len(subtree) == 0 || subtree[0] == nil || subtree[0].Id == "" {
		return nil, fmt.Errorf("empty subtree")
	}
	if opts.OmitIds {
		return nil, fmt.Errorf("OmitIds is not available on a block subtree: " +
			"a fragment is addressed by the ids it carries (§9)")
	}
	if opts.compactBlockLabels() {
		return nil, fmt.Errorf("block-label compaction is not available on a block subtree: " +
			"the short labels are local to the emitted run, not the object's ids (§9a)")
	}
	e := &exporter{
		opts:     opts,
		snapshot: &model.SmartBlockSnapshotBase{Blocks: subtree},
		blocks:   map[string]*model.Block{},
		visited:  map[string]bool{},
	}
	e.indexBlocks()
	// the fragment's root is the caller's, not the one indexBlocks infers from
	// an id-less snapshot: the emit below starts at subtree[0], so that is the
	// entry point the id reservations have to be reachable from (§4).
	e.rootId = subtree[0].Id
	var out []any
	// topLevel=false: a fragment caller addresses the blocks explicitly, so
	// even a structural root renders rather than silently vanishing
	if err := e.appendBlocksFlat(&out, []string{subtree[0].Id}, 0, false); err != nil {
		return nil, fmt.Errorf("marshal block subtree: %w", err)
	}
	// the legends AFTER the emit: every key slot has claimed its term by now,
	// which is the same ordering buildDoc relies on (§9a)
	env := &omap{}
	if m := e.buildPropertyKeys(); m != nil {
		env.set("property_keys", m)
	}
	// The type half is UNREACHABLE from every fragment slot today: typeSlug is
	// called only from envelopeTypeTerms and buildTypeProperties, and neither
	// is on this path, so no subtree can owe a type_keys line. Kept anyway,
	// because the cost is three lines and the failure mode of removing it is a
	// fragment that silently omits a legend the day a block slot starts
	// carrying a type term. A probe confirms the branch never fires.
	if m := e.buildTypeKeys(); m != nil {
		env.set("type_keys", m)
	}
	if m := e.buildOptionIds(); m != nil {
		env.set("option_ids", m)
	}
	env.set("blocks", out)
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("encode block subtree: %w", err)
	}
	return data, nil
}

// ParseInlineText parses §8 inline Markdown into plain text and marks — the
// single-field import codec. Mention/object mark params stay as written —
// an object reference is never compacted, so there is nothing to resolve
// them through (§9a).
func ParseInlineText(md string) (string, []*model.BlockContentTextMark, error) {
	return parseInline(md)
}

// RenderInlineText renders plain text and marks back into §8 inline
// Markdown — the single-field export codec, the exact inverse used by
// Marshal for every text-bearing block.
func RenderInlineText(text string, marks []*model.BlockContentTextMark) string {
	return renderInline(text, marks)
}

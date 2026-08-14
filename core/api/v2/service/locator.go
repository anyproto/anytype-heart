package v2service

// locator.go resolves a PATCH op's block from CONTENT instead of an id —
// Wave 2.1a/2.1b (APIV2_TOKENS.md §5, APIV2.md §8.43, §8.45): on
// replaceText the find text doubles as the locator when id is omitted; on
// updateBlock and deleteBlock the same job is done by `match`, an exact
// substring of the block's text. The resolution rule is the shipped
// wrapper rule (§8.21 locateBlock), moved down a layer and run per-op
// against the applier's live document view under the object lock: the
// wrapper's version was a read-then-patch TOCTOU — GET, resolve
// client-side, PATCH by id, with the document free to move in between —
// and in-API resolution is what removes the race (§5.5).
//
// The load-bearing rule (§5.3): the text must identify exactly ONE block,
// or the op refuses — zero matches steer to the outline read, several
// matching blocks list ≤8 candidates with context. Never a guess: a
// silent wrong match is the failure this design exists to prevent, and on
// deleteBlock it is a wrongly deleted subtree. ONE resolver serves every
// op, so there is never a second rule or a second refusal vocabulary;
// what an op contributes is its scope (which blocks it can act on at all)
// and the name of the field that carried the text. Later slices (2.1c–d)
// grow this file with the `under`/`nth` scoping vocabulary and the
// moveBlock/insertBlocks anchors.
//
// Multiplicity WITHIN the one matched block is not this function's
// business: it identifies a block, and every op here acts on the block as
// a whole. replaceText alone splices text and therefore keeps its own
// more-context refusal (stateops.go applyReplaceText) once the block is
// resolved — that is replace_all's (and later nth's) territory, not a
// resolution failure.

import (
	"fmt"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// maxLocatorCandidates bounds how many candidate blocks an ambiguity
// refusal lists — the wrapper's measured refusal shape (§8.21: 54 tokens,
// repaired first-try).
const maxLocatorCandidates = 8

// locatorContextWindow is how much surrounding text a candidate carries on
// each side of the matched text.
const locatorContextWindow = 30

// locatorScope reports whether a block of this type is one the op could act
// on — the candidate set the locator scans. It is a per-OP fact, not a
// second resolution rule, and it exists because narrowing the scan is only
// safe where the excluded blocks could never be the intent: an excluded
// block cannot capture the match, but it also cannot make a wrong match
// AMBIGUOUS, which is the direction that hurts on a destructive op.
type locatorScope func(blockType string) bool

// textBlocksOnly is replaceText's scope: only text-bearing blocks (code and
// embed included, §8.4). replaceText can edit nothing else, so a block it
// would refuse must neither capture the match nor make a unique one
// ambiguous.
func textBlocksOnly(typ string) bool { return anyblockjson.TextBlockType(typ) }

// everyBlock is updateBlock's and deleteBlock's scope: those ops address any
// block, so nothing may be filtered out of their candidate set. Today the
// two scopes coincide — the exporter writes `text` only on the types
// TextBlockType covers — so this is not an observable difference but the
// record of WHY each op has the scope it has: were a non-text block ever to
// carry text, filtering it out of a deleteBlock's candidates would turn a
// two-block ambiguity into a silent, wrong, destructive match.
func everyBlock(string) bool { return true }

// resolveByText maps a locator's text to the ONE view block whose text
// contains it. field is the op field that carried it ("find" on
// replaceText, "match" on updateBlock/deleteBlock) — it appears in the
// refusals so the repair names the caller's own vocabulary. The doc is the
// applier's LIVE view — mid-batch, op i scans the document op i−1 left,
// whether that op maintained the view in place (replaceText, M7) or forced
// a rebuild.
func resolveByText(doc *v2EditDoc, text, field, path string, scope locatorScope) (int, error) {
	var matches []int
	for i, b := range doc.blocks {
		if !scope(blockType(b)) {
			continue
		}
		if blockText, _ := b["text"].(string); blockText != "" && strings.Contains(blockText, text) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		// 404-class (C6): the steer is the outline read, and the exact-copy
		// tip is the one the id path already ships — the snippet may have
		// missed only because text is markup source
		return -1, v2model.NotFound(
			fmt.Sprintf("no block contains %q — copy the %s text exactly, including inline markup (text is markdown source: ** [ ] etc. count)", text, field),
			v2model.Issue{
				Path:    path,
				Message: fmt.Sprintf("the %s text must appear in exactly one block for the locator to resolve", field),
				Hint:    "GET the object with ?outline=true to list them, then copy the text exactly as a read serves it — or give the block id",
			})
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q appears in %d blocks — retry with id naming one of:", text, len(matches))
		for k, i := range matches {
			if k == maxLocatorCandidates {
				fmt.Fprintf(&b, "\n  … and %d more", len(matches)-maxLocatorCandidates)
				break
			}
			blk := doc.blocks[i]
			blockText, _ := blk["text"].(string)
			fmt.Fprintf(&b, "\n  block %s (%s): %q", blockId(blk), blockType(blk), locatorContext(blockText, text))
		}
		return -1, v2model.AmbiguousInput(b.String(),
			v2model.Issue{
				Path:    path,
				Message: fmt.Sprintf("the %s text appears in %d blocks — a locator must identify exactly one", field, len(matches)),
				Hint:    fmt.Sprintf("add surrounding text to %s until it appears in one block only, or give the block id", field),
			})
	}
}

// locatorContext excerpts ~locatorContextWindow bytes around the locator
// text's first occurrence — enough context to tell candidate blocks apart
// without dumping whole blocks into the refusal. (The wrapper's
// snippetContext, moved down with the resolution it serves.)
func locatorContext(text, needle string) string {
	idx := strings.Index(text, needle)
	start := idx - locatorContextWindow
	prefix := "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	end := idx + len(needle) + locatorContextWindow
	suffix := "…"
	if end >= len(text) {
		end, suffix = len(text), ""
	}
	// never slice mid-rune: move both cuts forward to rune boundaries
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	return prefix + text[start:end] + suffix
}

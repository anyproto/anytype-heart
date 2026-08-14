package v2service

// locator.go resolves a PATCH op's block from CONTENT instead of an id —
// Wave 2.1a (APIV2_TOKENS.md §5, APIV2.md §8.43): on replaceText the find
// text doubles as the locator when id is omitted. The resolution rule is
// the shipped wrapper rule (§8.21 locateBlock), moved down a layer and run
// per-op against the applier's live document view under the object lock:
// the wrapper's version was a read-then-patch TOCTOU — GET, resolve
// client-side, PATCH by id, with the document free to move in between —
// and in-API resolution is what removes the race (§5.5).
//
// The load-bearing rule (§5.3): the text must identify exactly ONE block,
// or the op refuses — zero matches steer to the outline read, several
// matching blocks list ≤8 candidates with context, and several
// occurrences within the one matched block fall to the existing
// more-context refusal (replace_all's, and later nth's, territory).
// Never a guess: a silent wrong match is the failure this design exists
// to prevent. Later slices (2.1b–d) grow this file with `match` on the
// other block ops and the `under`/`nth` scoping vocabulary.

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

// resolveByFind maps replaceText's find text to the ONE view block whose
// text contains it. Only text-bearing blocks participate (code and embed
// included, §8.4): replaceText can only edit those, so a block the op
// would refuse must never capture the match nor make a unique one
// ambiguous. The doc is the applier's LIVE view — mid-batch, op i scans
// the document op i−1 left, whether that op maintained the view in place
// (replaceText, M7) or forced a rebuild.
func resolveByFind(doc *v2EditDoc, find, path string) (int, error) {
	var matches []int
	for i, b := range doc.blocks {
		if !anyblockjson.TextBlockType(blockType(b)) {
			continue
		}
		if text, _ := b["text"].(string); text != "" && strings.Contains(text, find) {
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
			fmt.Sprintf("no block contains %q — copy the find text exactly, including inline markup (text is markdown source: ** [ ] etc. count)", find),
			v2model.Issue{
				Path:    path,
				Message: "the find text must appear in exactly one block for the locator to resolve",
				Hint:    "GET the object with ?outline=true to list them, then copy the text exactly as a read serves it — or give the block id",
			})
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q appears in %d blocks — retry with id naming one of:", find, len(matches))
		for k, i := range matches {
			if k == maxLocatorCandidates {
				fmt.Fprintf(&b, "\n  … and %d more", len(matches)-maxLocatorCandidates)
				break
			}
			blk := doc.blocks[i]
			text, _ := blk["text"].(string)
			fmt.Fprintf(&b, "\n  block %s (%s): %q", blockId(blk), blockType(blk), locatorContext(text, find))
		}
		return -1, v2model.AmbiguousInput(b.String(),
			v2model.Issue{
				Path:    path,
				Message: fmt.Sprintf("the find text appears in %d blocks — a locator must identify exactly one", len(matches)),
				Hint:    "add surrounding text to find until it appears in one block only, or give the block id",
			})
	}
}

// locatorContext excerpts ~locatorContextWindow bytes around the find
// text's first occurrence — enough context to tell candidate blocks apart
// without dumping whole blocks into the refusal. (The wrapper's
// snippetContext, moved down with the resolution it serves.)
func locatorContext(text, find string) string {
	idx := strings.Index(text, find)
	start := idx - locatorContextWindow
	prefix := "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	end := idx + len(find) + locatorContextWindow
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

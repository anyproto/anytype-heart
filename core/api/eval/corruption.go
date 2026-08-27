// Package eval holds the Phase-0 scoring primitives of the API v2 eval
// harness (APIV2.md §2 Phase 0, §8 harness ordering): the DELEGATE-52
// corruption metric, token/turn counters, and the task fixtures. The
// agent-loop runner that drives models against a scratch space pairs with
// Phase 3a — the first point at which benchmark B1 has competing edit
// methods to score — and is intentionally absent here.
package eval

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/snapshotdiff"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// CorruptionReport is the DELEGATE-52 backtranslation result: after
// applying a forward edit instruction and its inverse in sequence, any
// residual drift against the untouched document is corruption.
type CorruptionReport struct {
	// Findings are the per-axis drift findings from the state-diff /
	// text-multiset comparator (detail changes, lost text blocks).
	Findings []string
	// TextLost / TextAdded count text-multiset entries that disappeared
	// from / appeared in the document across the round trip.
	TextLost  int
	TextAdded int
}

// Clean reports a drift-free round trip.
func (r CorruptionReport) Clean() bool {
	return len(r.Findings) == 0 && r.TextAdded == 0
}

// ScoreCorruption compares the untouched snapshot with the snapshot after
// the forward+inverse edit sequence. Unlike the round-trip verifier it is
// order-sensitive: a backtranslation must restore exact document order, so
// pure reordering (same text multiset, different sequence) is corruption.
func ScoreCorruption(original, after *model.SmartBlockSnapshotBase, sbType model.SmartBlockType, opts anyblockjson.Options) CorruptionReport {
	report := CorruptionReport{Findings: snapshotdiff.Compare(original, after, sbType, opts)}

	origTexts := snapshotdiff.TextInventory(original)
	afterTexts := snapshotdiff.TextInventory(after)
	for text, n := range origTexts {
		if afterTexts[text] < n {
			report.TextLost += n - afterTexts[text]
		}
	}
	for text, n := range afterTexts {
		if origTexts[text] < n {
			report.TextAdded += n - origTexts[text]
		}
	}
	// pure reordering keeps the multiset but the sequence differs — invisible
	// to Compare's multiset, so the restructure fixture would falsely score
	// Clean without this.
	if report.TextLost == 0 && report.TextAdded == 0 &&
		!equalSeq(snapshotdiff.TextSequence(original), snapshotdiff.TextSequence(after)) {
		report.Findings = append(report.Findings, "text block order changed")
	}
	return report
}

func equalSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ScoreCorruptionJSON scores two AnyBlock JSON documents — the harness's
// natural inputs, since API reads and writes speak the format. Both
// documents import under the same options before comparison.
func ScoreCorruptionJSON(originalDoc, afterDoc []byte, opts anyblockjson.Options) (CorruptionReport, error) {
	// the comparator needs the smartblock type to know which bundled/derived
	// slots the format legitimately omits (OmittedBundledRelation,
	// DroppedTypeProvenanceKey, …); the original document's own type is the
	// authority, so take it from its import rather than assuming a page
	sbType, original, err := anyblockjson.Unmarshal(originalDoc, opts)
	if err != nil {
		return CorruptionReport{}, fmt.Errorf("import original document: %w", err)
	}
	_, after, err := anyblockjson.Unmarshal(afterDoc, opts)
	if err != nil {
		return CorruptionReport{}, fmt.Errorf("import edited document: %w", err)
	}
	return ScoreCorruption(original, after, sbType, opts), nil
}

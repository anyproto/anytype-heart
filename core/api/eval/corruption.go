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
// the forward+inverse edit sequence.
func ScoreCorruption(original, after *model.SmartBlockSnapshotBase, opts anyblockjson.Options) CorruptionReport {
	report := CorruptionReport{Findings: snapshotdiff.Compare(original, after, opts)}

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
	return report
}

// ScoreCorruptionJSON scores two AnyBlock JSON documents — the harness's
// natural inputs, since API reads and writes speak the format. Both
// documents import under the same options before comparison.
func ScoreCorruptionJSON(originalDoc, afterDoc []byte, opts anyblockjson.Options) (CorruptionReport, error) {
	_, original, err := anyblockjson.Unmarshal(originalDoc, opts)
	if err != nil {
		return CorruptionReport{}, fmt.Errorf("import original document: %w", err)
	}
	_, after, err := anyblockjson.Unmarshal(afterDoc, opts)
	if err != nil {
		return CorruptionReport{}, fmt.Errorf("import edited document: %w", err)
	}
	return ScoreCorruption(original, after, opts), nil
}

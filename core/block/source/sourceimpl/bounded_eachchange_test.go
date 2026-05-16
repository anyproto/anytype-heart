package sourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildBoundedEachChange_ResolvesOnlyGivenIdsSkipsUnresolved(t *testing.T) {
	pairs := map[string]readPair{
		"a": {"o1", 1},
		"b": {"o2", 2},
	}
	resolve := func(id string) (readPair, bool) { p, ok := pairs[id]; return p, ok }

	got := map[string]readPair{}
	each := buildBoundedEachChange([]string{"a", "missing", "b"}, resolve)
	each(func(id string, p readPair) { got[id] = p })

	assert.Equal(t, map[string]readPair{"a": {"o1", 1}, "b": {"o2", 2}}, got,
		"yields resolved candidates only; unresolved id skipped")
}

func TestBoundedVsFullStream_SameEmission(t *testing.T) {
	// all changes in tree storage: message ids + a non-message edit + a "late"
	// in-past insert (old OrderId, newer AddSeq).
	all := map[string]readPair{
		"G": {"o1", 1}, "m1": {"o2", 2}, "x1": {"o3", 3}, "M": {"o4", 4},
		"edit1": {"o5", 5}, // non-message change (no chat row)
		"late":  {"o2", 9}, // in-past insert: must stay unread
		"readM": {"o2", 2}, // a message already read (not a candidate)
	}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }

	// legacy stream: every change in storage
	full := func(yield func(string, readPair)) {
		for id, p := range all {
			yield(id, p)
		}
	}
	// bounded stream: only unread message-row candidates (G,m1,x1,M,late) —
	// excludes the non-message edit and the already-read message.
	bounded := buildBoundedEachChange([]string{"G", "m1", "x1", "M", "late"}, resolve)

	for _, frontier := range [][]string{
		{"M"},         // linear
		{"M", "late"}, // includes the in-past insert as a seen head
		{"x1"},        // partial
	} {
		var gotFull, gotBounded []string
		wf := newWatermark(func(ids []string) { gotFull = append(gotFull, ids...) })
		wf.advance(frontier, resolve, full)
		wb := newWatermark(func(ids []string) { gotBounded = append(gotBounded, ids...) })
		wb.advance(frontier, resolve, bounded)

		// Bounded emission must equal full emission intersected with the
		// candidate set (the only ids that can flip a counter).
		candidateSet := map[string]bool{"G": true, "m1": true, "x1": true, "M": true, "late": true}
		var fullCand []string
		for _, id := range gotFull {
			if candidateSet[id] {
				fullCand = append(fullCand, id)
			}
		}
		assert.ElementsMatch(t, fullCand, gotBounded,
			"frontier %v: bounded emission must equal full emission ∩ candidates", frontier)
	}
}

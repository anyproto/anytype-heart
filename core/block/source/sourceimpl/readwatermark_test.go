package sourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDominated(t *testing.T) {
	// OrderIds are width-consistent (real lexids sort lexicographically ==
	// topologically; naive "o9"/"o10" would not — "o9" > "o10").
	// linear: read up to (o05,a5) → (o03,a3) dominated, (o07,a4) not (orderId after)
	f := []readPair{{"o05", 5}}
	assert.True(t, dominated(readPair{"o03", 3}, f))
	assert.False(t, dominated(readPair{"o07", 4}, f))

	// late in-past insert: orderId old but AddSeq newer than frontier → NOT read
	assert.False(t, dominated(readPair{"o02", 9}, f), "in-past insert (newer AddSeq) must be unread")

	// multi-head: per-head dominance, NOT per-axis max
	multi := []readPair{{"o10", 5}, {"o04", 20}}
	assert.False(t, dominated(readPair{"o07", 8}, multi),
		"X not dominated by any single head (per-axis max would wrongly say read)")
	assert.True(t, dominated(readPair{"o03", 4}, multi)) // dominated by H2 (o04,a20)
	assert.True(t, dominated(readPair{"o09", 5}, multi)) // dominated by H1 (o10,a5)
}

func TestWatermark_AdvanceFiresDominatedAndDefersPending(t *testing.T) {
	all := map[string]readPair{
		"G": {"o1", 1}, "m1": {"o2", 2}, "x1": {"o3", 3}, "M": {"o4", 4},
		"late": {"o2", 9}, // in-past orderId, newer AddSeq
	}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }
	eachChange := func(yield func(string, readPair)) {
		for id, p := range all {
			yield(id, p)
		}
	}
	var got []string
	w := newWatermark(func(ids []string) { got = append(got, ids...) })

	// seen up to M(o4,a4): G,m1,x1,M dominated; "late" NOT (a9>a4)
	w.advance([]string{"M", "absent"}, resolve, eachChange)
	assert.ElementsMatch(t, []string{"G", "m1", "x1", "M"}, got)
	assert.Contains(t, w.pending, "absent") // unresolved seen id deferred

	// "absent" arrives later (resolvable) → re-resolved, no panic
	got = nil
	all["absent"] = readPair{"o5", 5}
	w.advance(nil, resolve, eachChange) // re-resolve pending only
	assert.NotContains(t, w.pending, "absent")
}

func TestWatermark_RebuildFromReducedSeenShrinksRead(t *testing.T) {
	all := map[string]readPair{"a": {"o1", 1}, "b": {"o2", 2}, "c": {"o3", 3}}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }
	each := func(y func(string, readPair)) {
		for id, p := range all {
			y(id, p)
		}
	}
	var got []string
	w := newWatermark(func(ids []string) { got = append(got, ids...) })
	w.advance([]string{"c"}, resolve, each) // read all
	assert.ElementsMatch(t, []string{"a", "b", "c"}, got)

	// explicit unread → chat rebuilds with reduced seen {a}: a FRESH engine
	// (mirrors InitDiffManager allocating manager.wm = newWatermark(...))
	got = nil
	w2 := newWatermark(func(ids []string) { got = append(got, ids...) })
	w2.advance([]string{"a"}, resolve, each)
	assert.ElementsMatch(t, []string{"a"}, got, "only a is read after reduced-seen rebuild")
}

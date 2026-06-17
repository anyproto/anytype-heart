package subscription

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// orderedAmendName is the Amend event for a single-key name change on the
// ordered-sub.
func orderedAmendName(id, name string) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfObjectDetailsAmend{
		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Id: id,
			Details: []*pb.EventObjectDetailsAmendKeyValue{
				{Key: bundle.RelationKeyName.String(), Value: domain.String(name).ToProto()},
			},
			SubIds: []string{"ordered-sub"},
		},
	})
}

// TestMinimalReorderNamesMover pins the minimal-window-diff contract: a sorted
// reorder emits Position events that NAME THE MOVED object(s), not the ones
// they displace, and emits the provable minimum count. This restores the
// pre-GO-7320 (Myers-diff) event shape while keeping the afterId-already-placed
// invariant. The previous left-to-right reconciliation named the displaced
// objects (e.g. [A,B,C]->[B,C,A] emitted Position{B,""}+Position{C,B}); these
// cases assert the new single-mover events.
func TestMinimalReorderNamesMover(t *testing.T) {
	t.Run("move to back names the mover with one Position", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("A", "alice"),
			givenNamedParticipant("B", "bob"),
			givenNamedParticipant("C", "cara"),
		})
		resp, err := fx.Search(givenOrderedRequest(0, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"A", "B", "C"}, recordIds(resp.Records))

		// A renamed to sort last -> new order [B, C, A]
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("A", "zoe"),
		})

		// names the MOVER (A), not the displaced B/C; exactly one Position
		want := []*pb.EventMessage{
			orderedPositionEvent("A", "C"),
			orderedAmendName("A", "zoe"),
		}
		got := waitMessages(t, resp.Output, 2)
		assert.Equal(t, want, got)

		// a faithful client still converges to [B, C, A]
		client := newClientList([]string{"A", "B", "C"})
		for _, m := range got {
			client.apply(t, m)
		}
		assert.Equal(t, []string{"B", "C", "A"}, client.list)
	})

	t.Run("far move emits a single Position, not one per passed object", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("A", "a"),
			givenNamedParticipant("B", "b"),
			givenNamedParticipant("C", "c"),
			givenNamedParticipant("D", "d"),
			givenNamedParticipant("E", "e"),
		})
		resp, err := fx.Search(givenOrderedRequest(0, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"A", "B", "C", "D", "E"}, recordIds(resp.Records))

		// A jumps across B,C,D,E to the tail: ONE Position (the old engine
		// emitted four, naming each displaced object).
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("A", "z"),
		})

		want := []*pb.EventMessage{
			orderedPositionEvent("A", "E"),
			orderedAmendName("A", "z"),
		}
		got := waitMessages(t, resp.Output, 2)
		assert.Equal(t, want, got)

		client := newClientList([]string{"A", "B", "C", "D", "E"})
		for _, m := range got {
			client.apply(t, m)
		}
		assert.Equal(t, []string{"B", "C", "D", "E", "A"}, client.list)
	})

	t.Run("move to front names the mover", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("A", "b"),
			givenNamedParticipant("B", "c"),
			givenNamedParticipant("C", "d"),
		})
		resp, err := fx.Search(givenOrderedRequest(0, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"A", "B", "C"}, recordIds(resp.Records))

		// C renamed to sort first -> [C, A, B]
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("C", "a"),
		})

		want := []*pb.EventMessage{
			orderedPositionEvent("C", ""),
			orderedAmendName("C", "a"),
		}
		got := waitMessages(t, resp.Output, 2)
		assert.Equal(t, want, got)

		client := newClientList([]string{"A", "B", "C"})
		for _, m := range got {
			client.apply(t, m)
		}
		assert.Equal(t, []string{"C", "A", "B"}, client.list)
	})
}

// diffOps drives the real windowDiffOps over a (oldWin, newWin) transition by
// constructing a minimal coreSub and returning the emitted ops. windowDiffOps
// reads only c.win/c.oldWin and stamps c into each subOp (used by encodeOp).
func diffOps(oldWin, newWin []string) []subOp {
	c := &coreSub{subId: "ordered-sub", spaceId: testSpaceId, oldWin: oldWin}
	c.win = make([]*visEntry, len(newWin))
	for i, id := range newWin {
		c.win[i] = &visEntry{
			id: id,
			prev: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String(id),
			}),
		}
	}
	out := &opBatch{}
	c.windowDiffOps(out)
	return out.ops
}

// lcsLen is an independent longest-common-subsequence length (NOT the engine's
// LIS code) so the minimality assertion is cross-checked against a different
// implementation.
func lcsLen(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			switch {
			case a[i-1] == b[j-1]:
				dp[i][j] = dp[i-1][j-1] + 1
			case dp[i-1][j] >= dp[i][j-1]:
				dp[i][j] = dp[i-1][j]
			default:
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	return dp[len(a)][len(b)]
}

// minPositions is the theoretical minimum number of Position events for a
// transition: the survivors that cannot stay on a common subsequence of the
// old and new orders must each move exactly once.
func minPositions(oldWin, newWin []string) int {
	inNew := map[string]bool{}
	for _, id := range newWin {
		inNew[id] = true
	}
	inOld := map[string]bool{}
	for _, id := range oldWin {
		inOld[id] = true
	}
	var oldSurv, newSurv []string
	for _, id := range oldWin {
		if inNew[id] {
			oldSurv = append(oldSurv, id)
		}
	}
	for _, id := range newWin {
		if inOld[id] {
			newSurv = append(newSurv, id)
		}
	}
	return len(oldSurv) - lcsLen(oldSurv, newSurv)
}

func randomSubsetPerm(rnd *rand.Rand, universe []string) []string {
	out := make([]string, 0, len(universe))
	for _, id := range universe {
		if rnd.Intn(2) == 0 {
			out = append(out, id)
		}
	}
	rnd.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// TestWindowDiffOpsMinimalAndConvergent fuzzes the real windowDiffOps over
// random (oldWin, newWin) transitions including adds, removes and reorders. For
// each it asserts (1) a faithful client (the repo's own dispatcher replay model
// from ordered_test.go) converges exactly to newWin with no afterId-not-present
// violation, and (2) the number of Position events equals the independent
// LCS-based minimum.
func TestWindowDiffOpsMinimalAndConvergent(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	// 10 ids -> windows up to length 10, longer LIS chains and more
	// simultaneous movers than a small universe would exercise.
	universe := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	const trials = 20000
	for trial := 0; trial < trials; trial++ {
		oldWin := randomSubsetPerm(rnd, universe)
		newWin := randomSubsetPerm(rnd, universe)
		ops := diffOps(oldWin, newWin)

		// structural invariants: no Add/Position afterId references an id
		// removed this batch, and each opSet is immediately followed by its
		// own opAdd (a Set never ships without placing its id).
		removed := map[string]bool{}
		for i := range ops {
			if ops[i].kind == opRemove {
				removed[ops[i].id] = true
			}
		}
		for i := range ops {
			op := ops[i]
			if (op.kind == opAdd || op.kind == opPosition) && op.afterId != "" {
				require.Falsef(t, removed[op.afterId],
					"trial %d afterId %q removed this batch", trial, op.afterId)
			}
			if op.kind == opSet {
				require.Truef(t, i+1 < len(ops) && ops[i+1].kind == opAdd && ops[i+1].id == op.id,
					"trial %d opSet for %q not immediately followed by its opAdd", trial, op.id)
			}
		}

		// convergence + afterId-present invariant (checked inside clientList.position)
		client := newClientList(oldWin)
		for i := range ops {
			client.apply(t, encodeOp(&ops[i]))
		}
		require.Equalf(t, newWin, client.list,
			"trial %d did not converge: old=%v new=%v", trial, oldWin, newWin)

		// minimality
		posCount := 0
		for i := range ops {
			if ops[i].kind == opPosition {
				posCount++
			}
		}
		require.Equalf(t, minPositions(oldWin, newWin), posCount,
			"trial %d non-minimal Position count: old=%v new=%v", trial, oldWin, newWin)
	}
}

// lisLenOracle is an independent O(n^2) DP longest-increasing-subsequence
// length, used to cross-check longestIncreasingSubseq.
func lisLenOracle(a []int) int {
	best := 0
	dp := make([]int, len(a))
	for i := range a {
		dp[i] = 1
		for j := 0; j < i; j++ {
			if a[j] < a[i] && dp[j]+1 > dp[i] {
				dp[i] = dp[j] + 1
			}
		}
		if dp[i] > best {
			best = dp[i]
		}
	}
	return best
}

// TestLongestIncreasingSubseq exercises the LIS helper directly (it is
// otherwise only reached through windowDiffOps): the returned indices must be
// a valid strictly-increasing subsequence whose length equals an independent
// DP oracle, across edge inputs and random permutations.
func TestLongestIncreasingSubseq(t *testing.T) {
	require.Nil(t, longestIncreasingSubseq(nil))
	require.Nil(t, longestIncreasingSubseq([]int{}))
	require.Equal(t, []int{0}, longestIncreasingSubseq([]int{5}))

	check := func(a []int, exact bool) {
		idx := longestIncreasingSubseq(a)
		seen := map[int]bool{}
		for _, k := range idx {
			require.GreaterOrEqual(t, k, 0)
			require.Less(t, k, len(a))
			require.False(t, seen[k], "duplicate index %d in %v", k, a)
			seen[k] = true
		}
		s := append([]int(nil), idx...)
		sort.Ints(s)
		for i := 1; i < len(s); i++ {
			require.Greater(t, s[i], s[i-1])                // valid subsequence (increasing positions)
			require.Greater(t, a[s[i]], a[s[i-1]], "%v", a) // strictly increasing values
		}
		if exact {
			require.Equalf(t, lisLenOracle(a), len(idx), "LIS length for %v", a)
		} else {
			require.LessOrEqual(t, len(idx), lisLenOracle(a))
		}
	}

	rnd := rand.New(rand.NewSource(11))
	for n := 1; n <= 40; n++ {
		for trial := 0; trial < 30; trial++ {
			check(rnd.Perm(n), true) // distinct values
		}
	}
	// duplicate (contract-violating) inputs must not corrupt: still a valid
	// strictly-increasing subsequence, no panic, no out-of-range index.
	for _, a := range [][]int{{1, 1, 2}, {2, 2, 2}, {0, 0, 1, 1, 2}, {3, 1, 1, 2, 2}} {
		check(a, false)
	}
}

// TestWindowDiffOpsEdgeWindows covers degenerate (oldWin,newWin) transitions
// the random fuzz hits only by luck (it never produces both-empty under its
// seed): empty windows, all-new, all-removed, full replacement, a single
// survivor, identity and full reversal. Each must converge, keep Set==Add, and
// stay minimal.
func TestWindowDiffOpsEdgeWindows(t *testing.T) {
	cases := []struct {
		name           string
		oldWin, newWin []string
	}{
		{"both empty", nil, nil},
		{"all new", nil, []string{"a", "b", "c"}},
		{"all removed", []string{"a", "b", "c"}, nil},
		{"full replace", []string{"a", "b"}, []string{"c", "d"}},
		{"single survivor among removals", []string{"a", "b", "c"}, []string{"b"}},
		{"single same", []string{"a"}, []string{"a"}},
		{"single replaced", []string{"a"}, []string{"b"}},
		{"identity", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"full reversal", []string{"a", "b", "c", "d"}, []string{"d", "c", "b", "a"}},
		{"survivor reorder with churn", []string{"a", "b", "c", "d"}, []string{"e", "c", "a", "f"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := diffOps(tc.oldWin, tc.newWin)
			client := newClientList(tc.oldWin)
			setCount, addCount, posCount := 0, 0, 0
			for i := range ops {
				switch ops[i].kind {
				case opSet:
					setCount++
				case opAdd:
					addCount++
				case opPosition:
					posCount++
				}
				client.apply(t, encodeOp(&ops[i]))
			}
			if len(tc.newWin) == 0 {
				require.Empty(t, client.list)
			} else {
				require.Equal(t, tc.newWin, client.list, "convergence")
			}
			require.Equal(t, addCount, setCount, "every entering id gets Set+Add")
			require.Equal(t, minPositions(tc.oldWin, tc.newWin), posCount, "minimal Position count")
		})
	}
}

// TestMultiMoverSingleBatch drives TWO simultaneous sort-key changes through
// the real processBatch/finalize/windowDiffOps pipeline (the integration tests
// otherwise move at most one object per batch). Renaming a->"z" (to the back)
// and e->"0" (to the front) in one batch must name BOTH movers minimally and
// converge the client to [e,b,c,d,a].
func TestMultiMoverSingleBatch(t *testing.T) {
	fx := newEngineFixture(t)
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		givenNamedParticipant("a", "a"),
		givenNamedParticipant("b", "b"),
		givenNamedParticipant("c", "c"),
		givenNamedParticipant("d", "d"),
		givenNamedParticipant("e", "e"),
	})
	resp, err := fx.Search(givenOrderedRequest(0, 0))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c", "d", "e"}, recordIds(resp.Records))

	svc := fx.Service.(*service)
	svc.mu.Lock()
	st := svc.spaces[testSpaceId]
	svc.mu.Unlock()
	require.NotNil(t, st)

	// Coalesce both renames into a single batch: a->z (to back), e->0 (to front).
	st.stopWorker()
	a := givenNamedParticipant("a", "z")
	e := givenNamedParticipant("e", "0")
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{a, e})
	st.processBatch([]feedItem{
		{id: "a", details: a.Details()},
		{id: "e", details: e.Details()},
	})
	st.drainOutbox()

	// 2 Positions (the movers) + 2 Amends (the renamed names); no Counters
	// since the total is unchanged.
	got := waitMessages(t, resp.Output, 4)
	var positioned []string
	for _, m := range got {
		if p := m.GetSubscriptionPosition(); p != nil {
			positioned = append(positioned, p.Id)
		}
	}
	require.ElementsMatch(t, []string{"a", "e"}, positioned, "both movers named")

	client := newClientList([]string{"a", "b", "c", "d", "e"})
	for _, m := range got {
		client.apply(t, m)
	}
	require.Equal(t, []string{"e", "b", "c", "d", "a"}, client.list)
}

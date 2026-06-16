package subscription

import (
	"math/rand"
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

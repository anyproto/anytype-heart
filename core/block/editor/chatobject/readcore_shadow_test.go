package chatobject

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fakeReadCoreProvider wraps the fixture's mock source (which satisfies
// source.Store) and additionally implements source.ReadCoreSnapshotProvider
// over a synthetic DAG — the production store's shape (the store IS the Store
// and ALSO exposes the snapshot).
type fakeReadCoreProvider struct {
	*mock_source.MockStore
	frontier     []string
	localHeads   []string
	metas        map[string]chatmodel.ChangeMeta
	calls        int
	resolveCalls int
}

func (f *fakeReadCoreProvider) ReadCoreSnapshot(name string, fn func(frontier []string, localHeads []string, resolve func(id string) ([]string, string, bool))) bool {
	f.calls++
	fn(f.frontier, f.localHeads, func(id string) ([]string, string, bool) {
		f.resolveCalls++
		m, ok := f.metas[id]
		return m.PrevIds, m.OrderId, ok
	})
	return true
}

func (fx *fixture) addPeerRowWithRead(t *testing.T, id, orderId, creator string, read bool) {
	t.Helper()
	msg := &chatmodel.Message{ChatMessage: &model.ChatMessage{
		Id:      id,
		OrderId: orderId,
		Creator: creator,
		Read:    read,
		Message: &model.ChatMessageMessageContent{Text: id},
	}}
	require.NoError(t, fx.repository.AddTestMessage(context.Background(), msg))
}

// canonicalProviderDag: G; a1 ∥ b1 (the cross-device shape), frontier {b1}.
// Per read*, a1 is unread (concurrent with the read head) — the CORE band.
func canonicalProviderDag(mock *mock_source.MockStore) *fakeReadCoreProvider {
	return &fakeReadCoreProvider{
		MockStore:  mock,
		frontier:   []string{"b1"},
		localHeads: []string{"a1", "b1"},
		metas: map[string]chatmodel.ChangeMeta{
			"G":  {OrderId: "o01"},
			"a1": {PrevIds: []string{"G"}, OrderId: "o02"},
			"b1": {PrevIds: []string{"G"}, OrderId: "o03"},
		},
	}
}

func TestReadCoreShadow_DisabledIsNoOp(t *testing.T) {
	fx := newFixture(t)
	prov := canonicalProviderDag(fx.source)
	fx.storeSource = prov

	fx.shadowReadCoreCount(context.Background(), chatmodel.CounterTypeMessage)

	assert.Zero(t, prov.calls, "disabled shadow must not consult the snapshot provider")
}

func TestComputeReadCoreCount_NonProviderDegrades(t *testing.T) {
	fx := newFixture(t)
	// the default mock source does NOT implement ReadCoreSnapshotProvider
	_, _, ok, err := fx.computeReadCoreCount(context.Background(), chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	assert.False(t, ok, "non-provider source degrades to a no-op")
}

// TestComputeReadCoreCount_AgreeAndDiverge pins both shadow outcomes on the
// canonical a1∥b1 shape:
//   - bool path in the convergent state (a1 unread) -> CORE agrees;
//   - bool path mis-flagged (a1 read while not causally covered — the class of
//     state an apply-order-dependent marker produces) -> CORE reports 1 vs
//     bool 0, exactly the divergence the shadow exists to surface.
func TestComputeReadCoreCount_AgreeAndDiverge(t *testing.T) {
	ctx := context.Background()

	t.Run("agree", func(t *testing.T) {
		fx := newFixture(t)
		fx.storeSource = canonicalProviderDag(fx.source)
		fx.addPeerRowWithRead(t, "a1", "o02", "alice", false) // unread, matches read*
		fx.addPeerRowWithRead(t, "b1", "o03", "bob", true)    // covered by frontier

		core, boolN, ok, err := fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, 1, core, "band{a1}: concurrent with the read head")
		assert.Equal(t, core, boolN, "bool path agrees in the convergent state")
	})

	t.Run("diverge", func(t *testing.T) {
		fx := newFixture(t)
		fx.storeSource = canonicalProviderDag(fx.source)
		fx.addPeerRowWithRead(t, "a1", "o02", "alice", true) // wrongly flagged read
		fx.addPeerRowWithRead(t, "b1", "o03", "bob", true)

		core, boolN, ok, err := fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, 1, core, "read* says a1 is unread regardless of the flag")
		assert.Equal(t, 0, boolN)
		assert.NotEqual(t, core, boolN, "the divergence the shadow logs")
	})
}

// TestComputeReadCoreCount_CacheAndIncremental pins the runtime-flow contract
// (spec §5): the walk runs only when the frontier changes; between frontier
// changes the band is maintained incrementally (Theorem 3) and queries reuse
// the cache — observed here via the provider's resolve-call counter.
func TestComputeReadCoreCount_CacheAndIncremental(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	prov := canonicalProviderDag(fx.source)
	fx.storeSource = prov
	fx.addPeerRowWithRead(t, "a1", "o02", "alice", false)
	fx.addPeerRowWithRead(t, "b1", "o03", "bob", true)

	// first call: cold in-process state -> walks
	core, _, ok, err := fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 1, core, "band{a1}")
	assert.Positive(t, prov.resolveCalls, "first call must walk")

	// second call, same frontier: cache hit, zero walking
	walked := prov.resolveCalls
	core, _, _, err = fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	assert.Equal(t, 1, core)
	assert.Equal(t, walked, prov.resolveCalls, "unchanged frontier must not re-walk")

	// a late in-past insert arrives: the tree gains the change, the handler
	// fires the Theorem-3 hook — the count updates with STILL no walk
	prov.metas["p0"] = chatmodel.ChangeMeta{PrevIds: []string{"G"}, OrderId: "o01x"}
	prov.localHeads = []string{"a1", "b1", "p0"}
	fx.addPeerRowWithRead(t, "p0", "o01x", "carol", false)
	fx.readCore.onMessageCreated("p0", "o01x", "carol", false)

	core, _, _, err = fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	assert.Equal(t, 2, core, "band{a1,p0} after the incremental append")
	assert.Equal(t, walked, prov.resolveCalls, "incremental maintenance must not trigger a walk")

	// frontier advances (user read a1 too): cache miss -> one fresh walk
	prov.frontier = []string{"a1", "b1"}
	core, _, _, err = fx.computeReadCoreCount(ctx, chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	assert.Greater(t, prov.resolveCalls, walked, "frontier change must re-walk")
	assert.Equal(t, 1, core, "only the in-past insert p0 stays unread")
}

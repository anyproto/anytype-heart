package chatobject

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

const rcMe = "me"

func newTestReadCore(t *testing.T) *readCoreManager {
	return newReadCoreManager(deviceAnystore(t), "obj1", rcMe)
}

func bandIdsOf(m *readCoreManager, frontier []string) []string {
	_, ids, ok := m.cachedCut(frontier)
	if !ok {
		return nil
	}
	sort.Strings(ids)
	return ids
}

// The Theorem-3 incremental rule: a newly attached change can never be an
// ancestor of an already-resolved frontier head, so an arriving counted
// message at/below the cut joins the (shared, D4) band with no ancestry check.
func TestReadCoreManager_Theorem3Rule(t *testing.T) {
	frontier := []string{"h1"}
	seed := func(t *testing.T) *readCoreManager {
		m := newTestReadCore(t)
		m.refresh(frontier, chatmodel.BandResult{MaxFrontierOrderId: "o05", ResolvedHeads: frontier})
		return m
	}

	t.Run("peer message below the cut joins the band", func(t *testing.T) {
		m := seed(t)
		m.onMessageCreated("late", "o03", "alice")
		assert.Equal(t, []string{"late"}, bandIdsOf(m, frontier))
	})

	t.Run("tail arrival is not band-tracked", func(t *testing.T) {
		m := seed(t)
		m.onMessageCreated("tip", "o09", "alice")
		assert.Empty(t, bandIdsOf(m, frontier), "g > maxF is the indexed tail's job")
	})

	t.Run("own message never joins", func(t *testing.T) {
		m := seed(t)
		m.onMessageCreated("mine", "o03", rcMe)
		assert.Empty(t, bandIdsOf(m, frontier))
	})

	t.Run("no cut means no band tracking", func(t *testing.T) {
		m := newTestReadCore(t)
		m.refresh(nil, chatmodel.BandResult{}) // no resolved frontier
		m.onMessageCreated("x", "o01", "alice")
		maxF, ids, ok := m.cachedCut(nil)
		require.True(t, ok)
		assert.Empty(t, maxF)
		assert.Empty(t, ids)
	})

	t.Run("invalid state ignores arrivals", func(t *testing.T) {
		m := newTestReadCore(t)
		m.onMessageCreated("x", "o01", "alice") // before any walk
		_, _, ok := m.cachedCut(frontier)
		assert.False(t, ok)
	})

	t.Run("delete removes from the band", func(t *testing.T) {
		m := seed(t)
		m.onMessageCreated("late", "o03", "alice")
		m.onMessageDeleted("late")
		assert.Empty(t, bandIdsOf(m, frontier))
	})
}

// cachedCut contract: misses on frontier change, on pending heads, and before
// the first walk; hits with the incrementally maintained band otherwise.
func TestReadCoreManager_CachedCut(t *testing.T) {
	m := newTestReadCore(t)
	frontier := []string{"h2", "h1"} // raw order must not matter

	m.refresh(frontier, chatmodel.BandResult{
		MaxFrontierOrderId: "o05",
		Candidates:         []string{"c1"},
		ResolvedHeads:      []string{"h1", "h2"},
	})

	maxF, ids, ok := m.cachedCut([]string{"h1", "h2"})
	require.True(t, ok, "order-insensitive frontier identity")
	assert.Equal(t, "o05", maxF)
	assert.Equal(t, []string{"c1"}, ids)

	_, _, ok = m.cachedCut([]string{"h1", "h3"})
	assert.False(t, ok, "different frontier -> walk required")

	// pending heads always force a re-walk: they may resolve at any moment
	// and move the cut without the frontier ids changing.
	m.refresh(frontier, chatmodel.BandResult{
		MaxFrontierOrderId: "o05",
		ResolvedHeads:      []string{"h1"},
		PendingHeads:       []string{"h2"},
	})
	_, _, ok = m.cachedCut(frontier)
	assert.False(t, ok)
}

// Persistence round-trip + the shadow-stage trust policy: the persisted doc
// restores for telemetry but is NOT live state (a fresh process re-walks).
func TestReadCoreManager_PersistRoundtrip(t *testing.T) {
	ctx := context.Background()
	db := deviceAnystore(t)
	m1 := newReadCoreManager(db, "obj1", rcMe)
	frontier := []string{"h1"}
	m1.refresh(frontier, chatmodel.BandResult{
		MaxFrontierOrderId: "o05",
		Candidates:         []string{"b2", "b1"},
		ResolvedHeads:      frontier,
	})
	m1.onMessageCreated("late", "o02", "alice")
	m1.persistDirty(ctx)

	m2 := newReadCoreManager(db, "obj1", rcMe)
	persisted, ok := m2.loadPersisted(ctx)
	require.True(t, ok)
	assert.Equal(t, "o05", persisted.maxF)
	assert.Equal(t, readCoreFrontierHash(frontier), persisted.frontierHash)
	assert.Equal(t, []string{"b1", "b2", "late"}, persisted.band, "sorted, incremental append included")

	_, _, live := m2.cachedCut(frontier)
	assert.False(t, live, "persisted state is not trusted as live until the first in-process walk")
}

// The equality gate for incremental maintenance: after Theorem-3 appends and
// deletes, the cached band must equal a fresh walk over the updated DAG.
func TestReadCoreManager_IncrementalEqualsFreshWalk(t *testing.T) {
	metas := map[string]chatmodel.ChangeMeta{
		"G":  {OrderId: "o01"},
		"m1": {PrevIds: []string{"G"}, OrderId: "o02"},
		"m2": {PrevIds: []string{"m1"}, OrderId: "o03"},
	}
	resolve := func(id string) (chatmodel.ChangeMeta, bool) {
		m, ok := metas[id]
		return m, ok
	}
	frontier := []string{"m2"}
	heads := []string{"m2"}

	m := newTestReadCore(t)
	walk := chatmodel.ComputeBand(frontier, heads, resolve)
	m.refresh(frontier, walk)
	assert.Empty(t, bandIdsOf(m, frontier), "linear history: empty band")

	// a late in-past insert arrives (concurrent branch below the cut)
	metas["late"] = chatmodel.ChangeMeta{PrevIds: []string{"G"}, OrderId: "o02x"}
	heads = []string{"m2", "late"}
	m.onMessageCreated("late", "o02x", "alice")

	fresh := chatmodel.ComputeBand(frontier, heads, resolve)
	sort.Strings(fresh.Candidates)
	assert.Equal(t, fresh.Candidates, bandIdsOf(m, frontier),
		"incremental band == fresh walk after the in-past insert")

	// and a tail arrival changes neither
	metas["tip"] = chatmodel.ChangeMeta{PrevIds: []string{"m2"}, OrderId: "o04"}
	heads = []string{"tip", "late"}
	m.onMessageCreated("tip", "o04", "bob")
	fresh = chatmodel.ComputeBand(frontier, heads, resolve)
	sort.Strings(fresh.Candidates)
	assert.Equal(t, fresh.Candidates, bandIdsOf(m, frontier))
}

package chatobject

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/util/storeutil"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

// readCoreManager caches the causal-ordinal CORE read-state (spec §5 "Runtime
// flows"): the frontier cut (maxF) and the band — unread change ids at/below
// the cut. Per decision D4 there is exactly ONE frontier (the message
// frontier) and ONE cached state: the mention counter is the same unread set
// filtered by hasMention at query time, so both counters are served from this
// state. Per decision D5 the frontier is monotone (markunread is deprecated),
// so there are no regression flows.
//
// Between frontier changes the band is maintained incrementally by the
// Theorem-3 rule: a newly attached change can never be a causal ancestor of an
// already-resolved frontier head (parent-first attachment), so an arriving
// counted message at/below the cut joins the band with NO ancestry check;
// everything past the cut is the indexed tail's job.
//
// The cache is device-local, persisted in crdt.db (collection
// "<objectId>readcore", a single doc). In the shadow stage a persisted doc is
// NOT trusted as live state across restarts — in-past inserts could have
// arrived while the process was down; the first use in a process always
// re-walks and the persisted copy serves as a cold-start staleness signal
// (logged once). Trusting it outright needs the Stage-2 apply-cursor
// (lastAddSeq gap replay), at which point cold start becomes O(gap).
//
// Locking: callers inside the source's ReadCoreSnapshot callback hold the
// object-tree lock and then take m.mu; the handler hooks take only m.mu.
// No path acquires the tree lock while holding m.mu — no inversion.
// Persistence never runs under the tree lock (persistDirty is called after
// the snapshot callback returns).
type readCoreManager struct {
	mu         sync.Mutex
	db         anystore.DB
	collName   string
	coll       anystore.Collection // opened lazily on first persistence use
	myIdentity string

	state       readCoreState
	staleLogged bool
}

type readCoreState struct {
	valid        bool
	maxF         string // "" = no resolved frontier (no cut, band must be empty)
	frontierHash string
	pending      []string // frontier ids not locally resolvable at walk time
	band         map[string]struct{}
	dirty        bool
}

const readCoreCacheDocId = "core"

func newReadCoreManager(db anystore.DB, objectId, myIdentity string) *readCoreManager {
	return &readCoreManager{
		db:         db,
		collName:   objectId + "readcore",
		myIdentity: myIdentity,
	}
}

// readCoreFrontierHash identifies a raw frontier as a set (order-insensitive).
func readCoreFrontierHash(frontier []string) string {
	ids := append([]string(nil), frontier...)
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

// cachedCut returns the cached (maxF, band) when the state is valid for the
// given raw frontier AND has no pending heads. A pending head may become
// resolvable at any moment and move the cut without the frontier ids changing,
// so pending always forces a re-walk (rare; safe over-count meanwhile).
func (m *readCoreManager) cachedCut(frontier []string) (maxF string, band []string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &m.state
	if !st.valid || len(st.pending) > 0 || st.frontierHash != readCoreFrontierHash(frontier) {
		return "", nil, false
	}
	band = make([]string, 0, len(st.band))
	for id := range st.band {
		band = append(band, id)
	}
	return st.maxF, band, true
}

// refresh replaces the state with a fresh walk result. Persistence is
// deferred to persistDirty (never under the tree lock).
func (m *readCoreManager) refresh(frontier []string, walk chatmodel.BandResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &m.state
	st.valid = true
	st.maxF = walk.MaxFrontierOrderId
	st.frontierHash = readCoreFrontierHash(frontier)
	st.pending = append([]string(nil), walk.PendingHeads...)
	st.band = make(map[string]struct{}, len(walk.Candidates))
	for _, id := range walk.Candidates {
		st.band[id] = struct{}{}
	}
	st.dirty = true
}

// onMessageCreated applies the Theorem-3 incremental rule for a freshly
// materialized message (called from ChatHandler.BeforeCreate). Own messages
// are never counted; the mention/message distinction is a query-time filter
// (D4), so the shared band tracks every counted peer message below the cut.
// No-op until the first walk validated the state, and when there is no cut.
func (m *readCoreManager) onMessageCreated(id, orderId, creator string) {
	if creator == m.myIdentity {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &m.state
	if !st.valid || st.maxF == "" || orderId > st.maxF {
		return // tail (or no state): the indexed range covers it
	}
	st.band[id] = struct{}{}
	st.dirty = true
}

// onMessageDeleted drops a deleted message from the cached band (the live
// collection stops counting it in the tail automatically).
func (m *readCoreManager) onMessageDeleted(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &m.state
	if !st.valid {
		return
	}
	if _, inBand := st.band[id]; inBand {
		delete(st.band, id)
		st.dirty = true
	}
}

// persistDirty writes a changed state to the device-local cache collection.
// Best-effort: failures are logged, never propagated (the cache is
// reconstructible by a walk).
func (m *readCoreManager) persistDirty(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &m.state
	if !st.valid || !st.dirty {
		return
	}
	if err := m.persistLocked(ctx, st); err != nil {
		log.Warn("readcore: persist cache", zap.Error(err))
		return
	}
	st.dirty = false
}

func (m *readCoreManager) collection(ctx context.Context) (anystore.Collection, error) {
	if m.coll != nil {
		return m.coll, nil
	}
	coll, err := m.db.Collection(ctx, m.collName)
	if err != nil {
		return nil, err
	}
	m.coll = coll
	return coll, nil
}

func (m *readCoreManager) persistLocked(ctx context.Context, st *readCoreState) error {
	coll, err := m.collection(ctx)
	if err != nil {
		return err
	}
	arena := &anyenc.Arena{}
	doc := arena.NewObject()
	doc.Set("id", arena.NewString(readCoreCacheDocId))
	doc.Set("maxF", arena.NewString(st.maxF))
	doc.Set("hash", arena.NewString(st.frontierHash))
	doc.Set("pending", storeutil.NewStringArrayValue(st.pending, arena))
	band := make([]string, 0, len(st.band))
	for id := range st.band {
		band = append(band, id)
	}
	sort.Strings(band)
	doc.Set("band", storeutil.NewStringArrayValue(band, arena))
	doc.Set("updatedAt", arena.NewNumberInt(int(time.Now().Unix())))
	return coll.UpsertOne(ctx, doc)
}

// persistedCacheState is the deserialized cold-start cache doc (telemetry /
// tests; not trusted as live state in the shadow stage — see the type comment).
type persistedCacheState struct {
	maxF         string
	frontierHash string
	pending      []string
	band         []string
}

func (m *readCoreManager) loadPersisted(ctx context.Context) (persistedCacheState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	coll, err := m.collection(ctx)
	if err != nil {
		return persistedCacheState{}, false
	}
	doc, err := coll.FindId(ctx, readCoreCacheDocId)
	if err != nil {
		return persistedCacheState{}, false
	}
	v := doc.Value()
	return persistedCacheState{
		maxF:         v.GetString("maxF"),
		frontierHash: v.GetString("hash"),
		pending:      storeutil.StringsFromArrayValue(v, "pending"),
		band:         storeutil.StringsFromArrayValue(v, "band"),
	}, true
}

// logIfStale compares a fresh walk against the persisted cold-start copy and
// logs once per process when they differ — the staleness signal that decides
// when the Stage-2 apply-cursor (trustable cold start) is worth shipping.
func (m *readCoreManager) logIfStale(ctx context.Context, walk chatmodel.BandResult, frontier []string) {
	persisted, ok := m.loadPersisted(ctx)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staleLogged {
		return
	}
	fresh := append([]string(nil), walk.Candidates...)
	sort.Strings(fresh)
	if persisted.frontierHash != readCoreFrontierHash(frontier) ||
		persisted.maxF != walk.MaxFrontierOrderId ||
		strings.Join(persisted.band, "\x00") != strings.Join(fresh, "\x00") {
		m.staleLogged = true
		log.Info("readcore: persisted cache was stale at cold start")
	}
}

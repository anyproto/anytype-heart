package treesyncer

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

var log = logger.NewNamed(treemanager.CName)

type executor struct {
	pool *streampool.ExecPool
	objs map[string]struct{}
	sync.Mutex
}

func newExecutor(workers, size int) *executor {
	return &executor{
		pool: streampool.NewExecPool(workers, size),
		objs: map[string]struct{}{},
	}
}

func (e *executor) tryAdd(id string, action func()) (err error) {
	e.Lock()
	defer e.Unlock()
	if _, exists := e.objs[id]; exists {
		return nil
	}
	e.objs[id] = struct{}{}
	return e.pool.TryAdd(func() {
		action()
		e.Lock()
		defer e.Unlock()
		delete(e.objs, id)
	})
}

func (e *executor) run() {
	e.pool.Run()
}

func (e *executor) close() {
	e.pool.Close()
}

type SyncedTreeRemover interface {
	app.ComponentRunnable
	RemoveAllExcept(senderId string, differentRemoteIds []string)
}

type SyncDetailsUpdater interface {
	app.Component
	UpdateSpaceDetails(existing, missing []string, spaceId string)
}

// PriorityProvider supplies tree ids that should be head-synced first within a
// diffsync cycle (GO-7302). It is optional: when no registered component
// implements it, existing trees keep their diff order. GetPriorityIds is queried
// once per SyncAll so priority can track navigation (e.g. the currently-open
// chat). Only ids that are also in the diff's existing set take effect.
// ReleasePriorityIds is called when the space's tree syncer closes so the
// provider can drop per-space resources backing GetPriorityIds.
type PriorityProvider interface {
	GetPriorityIds(spaceId string) []string
	ReleasePriorityIds(spaceId string)
}

type treeSyncer struct {
	sync.Mutex
	mainCtx            context.Context
	cancel             context.CancelFunc
	requests           int
	spaceId            string
	timeout            time.Duration
	spaceSettingsId    string
	requestPools       map[string]*executor
	headPools          map[string]*executor
	treeManager        treemanager.TreeManager
	isRunning          bool
	isSyncing          bool
	refreshable        *refresher[[]peer.Peer]
	nodeConf           nodeconf.NodeConf
	peerManager        peermanager.PeerManager
	syncedTreeRemover  SyncedTreeRemover
	syncDetailsUpdater SyncDetailsUpdater
	priority           PriorityProvider
}

func NewTreeSyncer(spaceId string) treesyncer.TreeSyncer {
	mainCtx, cancel := context.WithCancel(context.Background())
	return &treeSyncer{
		mainCtx:      mainCtx,
		cancel:       cancel,
		requests:     10,
		spaceId:      spaceId,
		timeout:      time.Second * 30,
		requestPools: map[string]*executor{},
		headPools:    map[string]*executor{},
	}
}

func (t *treeSyncer) Init(a *app.App) (err error) {
	t.isSyncing = true
	spaceStorage := app.MustComponent[spacestorage.SpaceStorage](a)
	t.spaceSettingsId = spaceStorage.StateStorage().SettingsId()
	t.treeManager = app.MustComponent[treemanager.TreeManager](a)
	t.peerManager = app.MustComponent[peermanager.PeerManager](a)
	t.refreshable = newRefresher(func(ctx context.Context) []peer.Peer {
		peers, err := t.peerManager.GetResponsiblePeers(ctx)
		if err != nil {
			log.Error("failed to get responsible peers", zap.Error(err))
			return nil
		}
		return peers
	})
	t.nodeConf = app.MustComponent[nodeconf.NodeConf](a)
	t.syncedTreeRemover = app.MustComponent[SyncedTreeRemover](a)
	t.syncDetailsUpdater = app.MustComponent[SyncDetailsUpdater](a)
	// optional: head-sync ordering hook (GO-7302). Absent -> diff order kept.
	if pp, err := app.GetComponent[PriorityProvider](a); err == nil {
		t.priority = pp
	}
	return nil
}

func (t *treeSyncer) Name() (name string) {
	return treesyncer.CName
}

func (t *treeSyncer) Run(ctx context.Context) (err error) {
	return nil
}

func (t *treeSyncer) Close(ctx context.Context) (err error) {
	t.Lock()
	defer t.Unlock()
	if t.refreshable != nil {
		t.refreshable.Close()
	}
	t.cancel()
	t.isRunning = false
	for _, pool := range t.headPools {
		pool.close()
	}
	for _, pool := range t.requestPools {
		pool.close()
	}
	if t.priority != nil {
		t.priority.ReleasePriorityIds(t.spaceId)
	}
	return nil
}

func (t *treeSyncer) StartSync() {
	t.Lock()
	defer t.Unlock()
	t.isRunning = true
	log.Info("starting request pool", zap.String("spaceId", t.spaceId))
	for _, p := range t.requestPools {
		p.run()
	}
	for _, p := range t.headPools {
		p.run()
	}
}

func (t *treeSyncer) StopSync() {
	t.Lock()
	defer t.Unlock()
	t.isRunning = false
	t.isSyncing = false
}

func (t *treeSyncer) ShouldSync(peerId string) bool {
	t.Lock()
	defer t.Unlock()
	return t.isSyncing
}

func (t *treeSyncer) SyncAll(ctx context.Context, p peer.Peer, existing, missing []string) (err error) {
	// Resolve priority order before taking the lock: the first GetPriorityIds call
	// per space may run a store query (lazy subscription start), and blocking I/O
	// must not happen under the treesyncer mutex. The head pool has a single FIFO
	// worker, so enqueue order is processing order: priority trees go to the front
	// so they head-sync first (GO-7302).
	headIds := existing
	if t.priority != nil {
		headIds = prioritizeFront(existing, t.priority.GetPriorityIds(t.spaceId))
	}
	t.Lock()
	defer t.Unlock()
	peerId := p.Id()
	isResponsible := slices.Contains(t.nodeConf.NodeIds(t.spaceId), peerId)
	t.sendSyncEvents(lo.Filter(existing, func(id string, index int) bool {
		return id != t.spaceSettingsId
	}), missing, isResponsible)
	reqExec, exists := t.requestPools[peerId]
	if !exists {
		reqExec = newExecutor(t.requests, 0)
		if t.isRunning {
			reqExec.run()
		}
		t.requestPools[peerId] = reqExec
	}
	headExec, exists := t.headPools[peerId]
	if !exists {
		headExec = newExecutor(1, 0)
		if t.isRunning {
			headExec.run()
		}
		t.headPools[peerId] = headExec
	}
	for _, id := range headIds {
		idCopy := id
		err = headExec.tryAdd(idCopy, func() {
			t.updateTree(p, idCopy)
		})
		if err != nil {
			log.Error("failed to add to head queue", zap.Error(err))
		}
	}
	for _, id := range missing {
		idCopy := id
		err = reqExec.tryAdd(idCopy, func() {
			t.requestTree(p, idCopy)
		})
		if err != nil {
			log.Error("failed to add to request queue", zap.Error(err))
		}
	}
	t.syncedTreeRemover.RemoveAllExcept(peerId, existing)
	return nil
}

func (t *treeSyncer) sendSyncEvents(existing, missing []string, nodePeer bool) {
	if !nodePeer {
		return
	}
	t.sendDetailsUpdates(existing, missing)
}

func (t *treeSyncer) sendDetailsUpdates(existing, missing []string) {
	t.syncDetailsUpdater.UpdateSpaceDetails(existing, missing, t.spaceId)
}

func (t *treeSyncer) IsRunning() bool {
	t.Lock()
	defer t.Unlock()
	return t.isRunning
}

func (t *treeSyncer) RefreshTrees(ids []string) error {
	if !t.IsRunning() {
		return nil
	}
	t.refreshable.doAfter(func(peers []peer.Peer) {
		if len(peers) == 0 {
			log.Warn("no responsible peers found for tree refresh", zap.Strings("treeIds", ids), zap.String("spaceId", t.spaceId))
			return
		}
		p := peers[0]
		t.Lock()
		headExec, exists := t.headPools[p.Id()]
		if !exists {
			headExec = newExecutor(1, 0)
			if t.isRunning {
				headExec.run()
			}
			t.headPools[p.Id()] = headExec
		}
		t.Unlock()
		for _, id := range ids {
			err := headExec.tryAdd(id, func() {
				t.updateTree(p, id)
			})
			if err != nil {
				log.Debug("failed to add to head queue", zap.Error(err))
			}
		}
	})
	return nil
}

func (t *treeSyncer) requestTree(p peer.Peer, id string) {
	log := log.With(zap.String("treeId", id))
	peerId := p.Id()
	ctx := peer.CtxWithPeerId(t.mainCtx, peerId)
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	tr, err := t.treeManager.GetTree(ctx, t.spaceId, id)
	if err != nil {
		log.Warn("can't load missing tree", zap.Error(err))
		return
	} else {
		log.Debug("loaded missing tree")
	}
	tr.Lock()
	if objecttree.IsEmptyDerivedTree(tr) {
		tr.Unlock()
		t.pingTree(p, tr)
	} else {
		tr.Unlock()
	}
}

func (t *treeSyncer) updateTree(p peer.Peer, id string) {
	log := log.With(zap.String("treeId", id), zap.String("spaceId", t.spaceId))
	peerId := p.Id()
	ctx := peer.CtxWithPeerId(t.mainCtx, peerId)
	tr, err := t.treeManager.GetTree(ctx, t.spaceId, id)
	if err != nil {
		log.Warn("can't load existing tree", zap.Error(err))
		return
	}
	t.pingTree(p, tr)
}

// prioritizeFront reorders ids so that those present in priority come first, in
// the order given by priority, followed by the remaining ids in their original
// relative order. Priority entries not present in ids are ignored. The head pool
// drains FIFO on a single worker, so this fully determines processing order.
func prioritizeFront(ids, priority []string) []string {
	if len(priority) == 0 || len(ids) == 0 {
		return ids
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	hot := make(map[string]struct{}, len(priority))
	front := make([]string, 0, len(priority))
	for _, id := range priority {
		if _, ok := idSet[id]; !ok {
			continue
		}
		if _, dup := hot[id]; dup {
			continue
		}
		hot[id] = struct{}{}
		front = append(front, id)
	}
	if len(front) == 0 {
		return ids
	}
	result := make([]string, 0, len(ids))
	result = append(result, front...)
	for _, id := range ids {
		if _, ok := hot[id]; !ok {
			result = append(result, id)
		}
	}
	return result
}

func (t *treeSyncer) pingTree(p peer.Peer, tr objecttree.ObjectTree) {
	syncTree, ok := tr.(synctree.SyncTree)
	if !ok {
		log.Warn("not a sync tree")
		return
	}
	if err := syncTree.SyncWithPeer(p.Context(), p); err != nil {
		log.Warn("synctree.SyncWithPeer error", zap.Error(err))
	} else {
		log.Debug("success synctree.SyncWithPeer")
	}
}

package treesyncer

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/streampool"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/syncstatus/helpers"
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

type Updater interface {
	app.ComponentRunnable
	SendUpdate(spaceSync *helpers.SpaceSync)
}

type treeSyncer struct {
	sync.Mutex
	mainCtx         context.Context
	cancel          context.CancelFunc
	requests        int
	spaceId         string
	timeout         time.Duration
	requestPools    map[string]*executor
	headPools       map[string]*executor
	treeManager     treemanager.TreeManager
	isRunning       bool
	isSyncing       bool
	spaceSyncStatus Updater
	peerManager     peermanager.PeerManager
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
	t.treeManager = app.MustComponent[treemanager.TreeManager](a)
	t.spaceSyncStatus = app.MustComponent[Updater](a)
	t.peerManager = app.MustComponent[peermanager.PeerManager](a)
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
	t.cancel()
	t.isRunning = false
	for _, pool := range t.headPools {
		pool.close()
	}
	for _, pool := range t.requestPools {
		pool.close()
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

func (t *treeSyncer) SyncAll(ctx context.Context, peerId string, existing, missing []string) error {
	t.Lock()
	defer t.Unlock()
	var (
		err      error
		nodePeer = slices.Contains(t.peerManager.GetNodeResponsiblePeers(), peerId)
	)
	defer t.sendResultEvent(err, nodePeer, peerId)
	t.sendSyncingEvent(peerId, existing, missing, nodePeer)
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
	for _, id := range existing {
		idCopy := id
		err = headExec.tryAdd(idCopy, func() {
			t.updateTree(peerId, idCopy)
		})
		if err != nil {
			log.Error("failed to add to head queue", zap.Error(err))
		}
	}
	for _, id := range missing {
		idCopy := id
		err = reqExec.tryAdd(idCopy, func() {
			t.requestTree(peerId, idCopy)
		})
		if err != nil {
			log.Error("failed to add to request queue", zap.Error(err))
		}
	}
	return nil
}

func (t *treeSyncer) sendSyncingEvent(peerId string, existing []string, missing []string, nodePeer bool) {
	if !nodePeer {
		return
	}
	if t.peerManager.IsPeerOffline(peerId) {
		t.spaceSyncStatus.SendUpdate(helpers.MakeSyncStatus(t.spaceId, helpers.Offline, 0, helpers.Null, helpers.Objects))
		return
	}
	if len(existing) != 0 || len(missing) != 0 {
		t.spaceSyncStatus.SendUpdate(helpers.MakeSyncStatus(t.spaceId, helpers.Syncing, len(existing)+len(missing), helpers.Null, helpers.Objects))
	}
}

func (t *treeSyncer) sendResultEvent(err error, nodePeer bool, peerId string) {
	if nodePeer && !t.peerManager.IsPeerOffline(peerId) {
		if err != nil {
			t.spaceSyncStatus.SendUpdate(helpers.MakeSyncStatus(t.spaceId, helpers.Error, 0, helpers.NetworkError, helpers.Objects))
		} else {
			t.spaceSyncStatus.SendUpdate(helpers.MakeSyncStatus(t.spaceId, helpers.Synced, 0, helpers.Null, helpers.Objects))
		}
	}
}

func (t *treeSyncer) requestTree(peerId, id string) {
	log := log.With(zap.String("treeId", id))
	ctx := peer.CtxWithPeerId(t.mainCtx, peerId)
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	_, err := t.treeManager.GetTree(ctx, t.spaceId, id)
	if err != nil {
		log.Warn("can't load missing tree", zap.Error(err))
	} else {
		log.Debug("loaded missing tree")
	}
}

func (t *treeSyncer) updateTree(peerId, id string) {
	log := log.With(zap.String("treeId", id), zap.String("spaceId", t.spaceId))
	ctx := peer.CtxWithPeerId(t.mainCtx, peerId)
	tr, err := t.treeManager.GetTree(ctx, t.spaceId, id)
	if err != nil {
		log.Warn("can't load existing tree", zap.Error(err))
		return
	}
	syncTree, ok := tr.(synctree.SyncTree)
	if !ok {
		log.Warn("not a sync tree")
	}
	if err = syncTree.SyncWithPeer(ctx, peerId); err != nil {
		log.Warn("synctree.SyncWithPeer error", zap.Error(err))
	} else {
		log.Debug("success synctree.SyncWithPeer")
	}
}

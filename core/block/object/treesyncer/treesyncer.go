package treesyncer

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"modernc.org/memory"
)

var log = logger.NewNamed(treemanager.CName)

type DynamicDelayLimiter struct {
	baseDelay time.Duration
	stepUp    time.Duration
	stepDown  time.Duration

	mu           sync.Mutex
	currentDelay time.Duration
}

// NewDynamicDelayLimiter initializes the limiter with:
//   - base: the minimum delay
//   - up: how much each Acquire() call increases the subsequent delay
//   - down: how much each Release() call decreases the delay
func NewDynamicDelayLimiter(base, up, down time.Duration) *DynamicDelayLimiter {
	return &DynamicDelayLimiter{
		baseDelay:    base,
		stepUp:       up,
		stepDown:     down,
		currentDelay: base,
	}
}

// Acquire blocks for the current delay, then increases the delay for the next caller.
func (d *DynamicDelayLimiter) Acquire() {
	d.mu.Lock()
	delayToWait := d.currentDelay
	d.currentDelay += d.stepUp
	d.mu.Unlock()

	// Sleep outside the lock so we don't block other goroutines.
	time.Sleep(delayToWait)
}

// Release indicates we're done with the resource and decreases the delay (without going below base).
func (d *DynamicDelayLimiter) Release() {
	d.mu.Lock()
	defer d.mu.Unlock()

	newDelay := d.currentDelay - d.stepDown
	if newDelay < d.baseDelay {
		newDelay = d.baseDelay
	}
	d.currentDelay = newDelay
}

// CurrentDelay returns the delay for debugging/monitoring purposes.
func (d *DynamicDelayLimiter) CurrentDelay() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.currentDelay
}

var limiter = NewDynamicDelayLimiter(0, time.Millisecond*100, time.Millisecond*100)

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

func init() {
	memory.PrintMsg = func(add int, total int, plus bool) {
		if plus {
			println("[x]: 5allocating more", fmt.Sprintf("%dKb", add/1024), fmt.Sprintf("%dKb", total/1024))
		} else {
			println("[x]: 4freeing some", fmt.Sprintf("%dKb", add/1024), fmt.Sprintf("%dKb", total/1024))
		}
	}
}

type countManager interface {
	treemanager.TreeManager
	GetTreeCount() int
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
	treeManager        countManager
	isRunning          bool
	isSyncing          bool
	nodeConf           nodeconf.NodeConf
	syncedTreeRemover  SyncedTreeRemover
	syncDetailsUpdater SyncDetailsUpdater
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
	t.treeManager = app.MustComponent[countManager](a)
	t.nodeConf = app.MustComponent[nodeconf.NodeConf](a)
	t.syncedTreeRemover = app.MustComponent[SyncedTreeRemover](a)
	t.syncDetailsUpdater = app.MustComponent[SyncDetailsUpdater](a)
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

func (t *treeSyncer) SyncAll(ctx context.Context, p peer.Peer, existing, missing []string) (err error) {
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
	for _, id := range existing {
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
			limiter.Acquire()
			defer limiter.Release()
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

func (t *treeSyncer) requestTree(p peer.Peer, id string) {
	log := log.With(zap.String("treeId", id))
	peerId := p.Id()
	ctx := peer.CtxWithPeerId(t.mainCtx, peerId)
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	tr, err := t.treeManager.GetTree(ctx, t.spaceId, id)
	if err != nil {
		println("[x]: failed to get tree", err.Error())
		log.Warn("can't load missing tree", zap.Error(err))
		return
	} else {
		log.Debug("loaded missing tree")
	}
	if objecttree.IsEmptyDerivedTree(tr) {
		t.pingTree(p, tr)
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

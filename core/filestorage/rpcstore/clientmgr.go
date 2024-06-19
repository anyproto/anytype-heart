package rpcstore

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/cheggaaa/mb/v3"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const (
	maxConnections = 10
	maxTasks       = 100
)

type operationNameKeyType string

const operationNameKey operationNameKeyType = "operationName"

var (
	clientCreateTimeout = 1 * time.Minute
)

func newClientManager(pool pool.Pool, peerStore peerstore.PeerStore, peerUpdateCh chan struct{}) *clientManager {
	cm := &clientManager{
		mb: mb.New[*task](maxTasks),
		ocache: ocache.New(
			func(ctx context.Context, id string) (value ocache.Object, err error) {
				return nil, fmt.Errorf("load func shouldn't be used")
			},
			ocache.WithTTL(time.Minute*5),
			ocache.WithLogger(log.Sugar()),
			ocache.WithGCPeriod(0),
		),
		checkPeersCh: peerUpdateCh,
		pool:         pool,
		peerStore:    peerStore,
	}
	cm.ctx, cm.ctxCancel = context.WithCancel(context.Background())
	cm.ctx = context.WithValue(cm.ctx, operationNameKey, "checkPeerLoop")
	go cm.checkPeerLoop()
	return cm
}

// clientManager manages clients, removes unused ones, and adds new ones if necessary
type clientManager struct {
	mb           *mb.MB[*task]
	ctx          context.Context
	ctxCancel    context.CancelFunc
	ocache       ocache.OCache
	checkPeersCh chan struct{}

	pool      pool.Pool
	peerStore peerstore.PeerStore

	mu sync.RWMutex
}

func (m *clientManager) add(ctx context.Context, ts ...*task) (err error) {
	m.mu.Lock()
	if m.ocache.Len() == 0 {
		if err = m.checkPeers(ctx, true); err != nil {
			m.mu.Unlock()
			return
		}
	}
	m.mu.Unlock()
	return m.mb.Add(ctx, ts...)
}

func (m *clientManager) WriteOp(ctx context.Context, ready chan result, f func(c *client) error, c cid.Cid) (err error) {
	return m.addOp(ctx, true, ready, f, c)
}

func (m *clientManager) ReadOp(ctx context.Context, ready chan result, f func(c *client) error, c cid.Cid) (err error) {
	return m.addOp(ctx, false, ready, f, c)
}

func (m *clientManager) addOp(ctx context.Context, write bool, ready chan result, f func(c *client) error, c cid.Cid) (err error) {
	t := getTask()
	t.ctx = ctx
	t.cid = c
	t.exec = f
	t.spaceId = fileblockstore.CtxGetSpaceId(ctx)
	t.onFinished = m.onTaskFinished
	t.ready = ready
	t.write = write
	return m.add(ctx, t)
}

func (m *clientManager) onTaskFinished(t *task, c *client, taskErr error) {
	var taskClientIds []string
	m.ocache.ForEach(func(v ocache.Object) (isContinue bool) {
		cl := v.(*client)
		if cl.peerId == c.peerId {
			return true
		}
		if cl.checkSpaceFilter(t) {
			taskClientIds = append(taskClientIds, cl.peerId)
		}
		return true
	})
	if taskErr != nil {
		for _, peerId := range taskClientIds {
			log.Info("retrying task", zap.Error(taskErr), zap.String("cid", t.cid.String()))
			if !slices.Contains(t.denyPeerIds, peerId) {
				t.denyPeerIds = append(t.denyPeerIds, c.peerId)
				err := m.add(t.ctx, t)
				if err != nil {
					taskErr = err
					break
				}
				return
			}
		}
	}
	log.Debug("finishing task task", zap.String("cid", t.cid.String()))
	t.ready <- result{err: taskErr}
	t.release()
}

func (m *clientManager) checkPeerLoop() {
	_ = m.checkPeers(m.ctx, false)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.checkPeersCh:
			_ = m.checkPeers(m.ctx, false)
		case <-ticker.C:
			_ = m.checkPeers(m.ctx, false)
		}
	}
}

func (m *clientManager) checkPeers(ctx context.Context, needClient bool) (err error) {
	// start GC to remove unused clients
	m.ocache.GC()
	if m.ocache.Len() >= maxConnections {
		// reached connection limit, can't add new nodePeerIds
		return
	}
	if !needClient && m.mb.Len() == 0 {
		// has empty queue, no need new nodePeerIds
		return
	}

	addPeer := func(peerId string) (added bool) {
		added = true
		if _, cerr := m.ocache.Pick(ctx, peerId); cerr == ocache.ErrNotExists {
			var cancel context.CancelFunc
			ctx, cancel := context.WithTimeout(ctx, clientCreateTimeout)
			cl, e := newClient(ctx, m.pool, peerId, m.mb)
			if e != nil {
				opName, _ := ctx.Value(operationNameKey).(string)
				log.Info("can't create client", zap.String("operation", opName), zap.Error(e))
				cancel()
				added = false
				return
			}
			_ = m.ocache.Add(peerId, cl)
			cancel()
			added = true
		}
		return
	}

	// try to add new nodePeerIds
	nodePeerIds := m.peerStore.ResponsibleFilePeers()
	rand.Shuffle(len(nodePeerIds), func(i, j int) {
		nodePeerIds[i], nodePeerIds[j] = nodePeerIds[j], nodePeerIds[i]
	})
	for _, peerId := range nodePeerIds {
		if addPeer(peerId) {
			break
		}
	}
	localPeerIds := m.peerStore.AllLocalPeers()
	for _, peerId := range localPeerIds {
		addPeer(peerId)
	}
	if m.ocache.Len() == 0 {
		return fmt.Errorf("no connection to any file client")
	}
	return nil
}

func (m *clientManager) Close() (err error) {
	m.ctxCancel()
	if err = m.mb.Close(); err != nil {
		log.Error("mb close error", zap.Error(err))
	}
	return m.ocache.Close()
}

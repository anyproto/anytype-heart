package threads

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	net2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	util2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	ma "github.com/multiformats/go-multiaddr"
	db2 "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

var log = logging.Logger("anytype-threads")

type service struct {
	t                 net2.NetBoostrapper
	db                *db.DB
	threadsCollection *db.Collection
	threadsGetter     ThreadsGetter
	device            wallet.Keypair
	account           wallet.Keypair
	repoRootPath      string
	newHeadProcessor  func(id thread.ID) error
	newThreadChan     chan<- string

	replicatorAddr ma.Multiaddr
	ctx            context.Context
	ctxCancel      context.CancelFunc
	sync.Mutex
}

func New(threadsAPI net2.NetBoostrapper, threadsGetter ThreadsGetter, repoRootPath string, deviceKeypair wallet.Keypair, accountKeypair wallet.Keypair, newHeadProcessor func(id thread.ID) error, newThreadChan chan string, replicatorAddr ma.Multiaddr) Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &service{
		t:                threadsAPI,
		threadsGetter:    threadsGetter,
		device:           deviceKeypair,
		repoRootPath:     repoRootPath,
		account:          accountKeypair,
		newHeadProcessor: newHeadProcessor,
		replicatorAddr:   replicatorAddr,
		ctx:              ctx,
		ctxCancel:        cancel,
	}
}

type Service interface {
	ThreadsCollection() (*db.Collection, error)
	CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error)
	ListThreadIdsByType(blockType smartblock.SmartBlockType) ([]thread.ID, error)

	DeleteThread(id string) error
	InitNewThreadsChan(ch chan<- string) error // can be called only once

	EnsurePredefinedThreads(ctx context.Context, newAccount bool) (DerivedSmartblockIds, error)
	Close() error
}

type ThreadsGetter interface {
	Threads() (thread.IDSlice, error)
}

func (s *service) InitNewThreadsChan(ch chan<- string) error {
	s.Lock()
	defer s.Unlock()
	if s.newThreadChan != nil {
		return fmt.Errorf("already set")
	}

	s.newThreadChan = ch
	return nil
}

func (s *service) getNewThreadChan() chan<- string {
	s.Lock()
	defer s.Unlock()
	return s.newThreadChan
}

func (s *service) ThreadsCollection() (*db.Collection, error) {
	if s.threadsCollection == nil {
		return nil, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	return s.threadsCollection, nil
}

func (s *service) Close() error {
	// close global service context to stop all work
	s.ctxCancel()
	// lock in order to wait for work to finish and, e.g. db to init
	s.Lock()
	defer s.Unlock()
	if db := s.db; db != nil {
		return db.Close()
	}
	return nil
}

func (s *service) CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error) {
	if s.threadsCollection == nil {
		return thread.Info{}, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	// todo: we have a possible trouble here, using thread.AccessControlled uvariant without actually storing the cid with access control
	thrdId, err := threadCreateID(thread.AccessControlled, blockType)
	if err != nil {
		return thread.Info{}, err
	}
	followKey, err := symmetric.NewRandom()
	if err != nil {
		return thread.Info{}, err
	}

	readKey, err := symmetric.NewRandom()
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err := s.t.CreateThread(context.TODO(), thrdId, net.WithThreadKey(thread.NewKey(followKey, readKey)), net.WithLogKey(s.device))
	if err != nil {
		return thread.Info{}, err
	}

	var replAddrWithThread ma.Multiaddr
	if s.replicatorAddr != nil {
		replAddrWithThread, err = util2.MultiAddressAddThread(s.replicatorAddr, thrdId)
		if err != nil {
			return thread.Info{}, err
		}
		hasReplAddress := util2.MultiAddressHasReplicator(thrd.Addrs, s.replicatorAddr)

		if !hasReplAddress && replAddrWithThread != nil {
			thrd.Addrs = append(thrd.Addrs, replAddrWithThread)
		}
	}

	threadInfo := threadInfo{
		ID:    db2.InstanceID(thrd.ID.String()),
		Key:   thrd.Key.String(),
		Addrs: util2.MultiAddressesToStrings(thrd.Addrs),
	}

	// todo: wait for threadsCollection to push?
	_, err = s.threadsCollection.Create(util.JSONFromInstance(threadInfo))
	if err != nil {
		log.With("thread", thrd.ID.String()).Errorf("failed to create thread at collection: %s: ", err.Error())
	}

	if replAddrWithThread != nil {
		go func() {
			attempt := 0
			// todo: rewrite to job queue in badger
			for {
				attempt++
				p, err := s.t.AddReplicator(context.TODO(), thrd.ID, replAddrWithThread)
				if err != nil {
					log.Errorf("failed to add log replicator after %d attempt: %s", attempt, err.Error())
					select {
					case <-time.After(time.Second * 3 * time.Duration(attempt)):
					case <-s.ctx.Done():
						return
					}
					continue
				}

				log.With("thread", thrd.ID.String()).Infof("added log replicator after %d attempt: %s", attempt, p.String())
				return
			}
		}()
	}

	return thrd, nil
}

func (s *service) DeleteThread(id string) error {
	if s.threadsCollection == nil {
		return fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	tid, err := thread.Decode(id)
	if err != nil {
		return fmt.Errorf("incorrect block id: %w", err)
	}

	err = s.t.DeleteThread(context.Background(), tid)
	if err != nil {
		return err
	}

	err = s.threadsCollection.Delete(db2.InstanceID(id))
	if err != nil {
		// todo: here we can get an error if we didn't yet added thead keys into DB
		log.With("thread", id).Error("DeleteThread failed to remove thread from collection: %s", err.Error())
	}
	return nil
}

func (s *service) ListThreadIdsByType(blockType smartblock.SmartBlockType) ([]thread.ID, error) {
	threads, err := s.threadsGetter.Threads()
	if err != nil {
		return nil, err
	}

	var filtered []thread.ID
	for _, thrdId := range threads {
		t, err := smartblock.SmartBlockTypeFromThreadID(thrdId)
		if err != nil {
			log.Errorf("SmartBlockTypeFromThreadID failed: %s", err.Error())
			continue
		}

		if t == blockType {
			filtered = append(filtered, thrdId)
		}
	}

	return filtered, nil
}

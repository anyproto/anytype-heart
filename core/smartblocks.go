package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	util2 "github.com/anytypeio/go-anytype-library/util"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	db2 "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

var ErrBlockSnapshotNotFound = fmt.Errorf("block snapshot not found")

func (a *Anytype) GetBlock(id string) (SmartBlock, error) {
	parts := strings.Split(id, "/")

	_, err := thread.Decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("incorrect block id: %w", err)
	}
	smartBlock, err := a.GetSmartBlock(parts[0])
	if err != nil {
		return nil, err
	}

	return smartBlock, nil
}

func (a *Anytype) DeleteBlock(id string) error {
	tid, err := thread.Decode(id)
	if err != nil {
		return fmt.Errorf("incorrect block id: %w", err)
	}

	if err = a.t.DeleteThread(context.Background(), tid); err != nil {
		return err
	}

	if err = a.localStore.Pages.DeletePage(id); err != nil {
		return err
	}

	if err = a.threadsCollection.Delete(db2.InstanceID(id)); err != nil {
		// todo: here we can get an error if we didn't yet added thead keys into DB
		log.With("thread", id).Error("DeleteBlock failed to remove thread from collection: %s", err.Error())
	}

	return nil
}

/*func (a *Anytype) blockToVersion(block *model.Block, parentSmartBlockVersion BlockVersion, versionId string, creator string, date *types.Timestamp) BlockVersion {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
		return &smartBlockSnapshot{
			model: &storage.SmartBlockWithMeta{
				Block: block,
			},
			versionId: versionId,
			creator:      creator,
			date:      date,
			service:      a,
		}
	default:
		return &SimpleBlockVersion{
			model:                   block,
			parentSmartBlockVersion: parentSmartBlockVersion,
			service:                    a,
		}
	}
}*/
func (a *Anytype) pullThread(ctx context.Context, id thread.ID) (headsChanged bool, err error) {
	thrd, err := a.t.GetThread(context.Background(), id)
	if err != nil {
		return false, err
	}

	var headPerLog = make(map[peer.ID]cid.Cid, len(thrd.Logs))
	for _, log := range thrd.Logs {
		headPerLog[log.ID] = log.Head
	}

	err = a.t.PullThread(ctx, id)
	if err != nil {
		return false, err
	}

	thrd, err = a.t.GetThread(context.Background(), id)
	if err != nil {
		return false, err
	}

	for _, log := range thrd.Logs {
		if v, exists := headPerLog[log.ID]; !exists {
			headsChanged = true
			break
		} else {
			if !log.Head.Equals(v) {
				headsChanged = true
				break
			}
		}
	}

	return
}

func (a *Anytype) initThreadsDB() error {
	if a.db != nil {
		return nil
	}

	accountID, err := a.threadDeriveID(threadDerivedIndexAccount)
	if err != nil {
		return err
	}

	d, err := db.NewDB(context.Background(), a.t, accountID, db.WithNewRepoPath(filepath.Join(a.opts.Repo, "collections")))
	if err != nil {
		return err
	}

	a.db = d

	a.threadsCollection = a.db.GetCollection(threadInfoCollectionName)
	err = a.listenExternalNewThreads()
	if err != nil {
		return fmt.Errorf("failed to listen external new threads: %w", err)
	}

	if a.threadsCollection == nil {
		a.threadsCollection, err = a.db.NewCollection(threadInfoCollection)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *Anytype) createPredefinedBlocksIfNotExist(accountSelect bool) error {
	// account
	a.lock.Lock()
	defer a.lock.Unlock()
	account, justCreated, err := a.predefinedThreadAdd(threadDerivedIndexAccount, false, false, false)
	if err != nil {
		return err
	}

	a.predefinedBlockIds.Account = account.ID.String()
	if a.db == nil {
		err = a.initThreadsDB()
		if err != nil {
			return fmt.Errorf("initThreadsDB failed: %w", err)
		}

		err = a.handleAllMissingDbRecords(account.ID.String())
		if err != nil {
			return fmt.Errorf("handleAllMissingDbRecords failed: %w", err)
		}

		go func() {
			err = a.addMissingReplicators()
			if err != nil {
				log.Errorf("addMissingReplicators: %s", err.Error())
			}
		}()

		go func() {
			err = a.addMissingThreadsToCollection()
			if err != nil {
				log.Errorf("addMissingThreadsToCollection: %s", err.Error())
			}
		}()
		err = a.addMissingThreadsFromCollection()
		if err != nil {
			return fmt.Errorf("addMissingThreadsFromCollection failed: %w", err)
		}
	}

	accountThreadPullDone := make(chan struct{})
	if accountSelect {
		// accountSelect common case
		go func() {
			defer close(accountThreadPullDone)
			// pull only after adding collection to handle all events
			_, err = a.pullThread(context.TODO(), account.ID)
			if err != nil {
				log.Errorf("failed to pull accountThread")
			}
		}()

		if justCreated {
			// this is the case of accountSelect after accountRecovery
			// we need to wait for account thread pull to be done
			<-accountThreadPullDone
			if err != nil {
				return err
			}
		}
	}

	// profile
	profile, _, err := a.predefinedThreadAdd(threadDerivedIndexProfilePage, accountSelect, true, false)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Profile = profile.ID.String()

	// archive
	archive, _, err := a.predefinedThreadAdd(threadDerivedIndexArchive, accountSelect, true, false)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Archive = archive.ID.String()

	// set pages
	setPages, _, err := a.predefinedThreadAdd(threadDerivedIndexSetPages, accountSelect, true, false)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.SetPages = setPages.ID.String()

	// home
	home, _, err := a.predefinedThreadAdd(threadDerivedIndexHome, accountSelect, true, true)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Home = home.ID.String()

	return nil
}

func (a *Anytype) newBlockThread(blockType smartblock.SmartBlockType) (thread.Info, error) {
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

	thrd, err := a.t.CreateThread(context.TODO(), thrdId, net.WithThreadKey(thread.NewKey(followKey, readKey)), net.WithLogKey(a.opts.Device))
	if err != nil {
		return thread.Info{}, err
	}

	threadComp, err := ma.NewComponent(thread.Name, thrd.ID.String())
	if err != nil {
		return thread.Info{}, err
	}

	hasCafeAddress := false
	var cafeAddrWithThread ma.Multiaddr
	if a.opts.CafeP2PAddr != nil {
		cafeAddrWithThread = a.opts.CafeP2PAddr.Encapsulate(threadComp)
	}

	var multiAddrs []ma.Multiaddr
	for _, addr := range thrd.Addrs {
		if cafeAddrWithThread != nil && addr.Equal(cafeAddrWithThread) {
			hasCafeAddress = true
		}

		multiAddrs = append(multiAddrs, addr)
	}

	if !hasCafeAddress && cafeAddrWithThread != nil {
		multiAddrs = append(multiAddrs, cafeAddrWithThread)
	}

	threadInfo := threadInfo{
		ID:    db2.InstanceID(thrd.ID.String()),
		Key:   thrd.Key.String(),
		Addrs: util2.MultiAddressesToStrings(multiAddrs),
	}

	// todo: wait for threadsCollection to push?
	_, err = a.threadsCollection.Create(util.JSONFromInstance(threadInfo))
	if err != nil {
		log.With("thread", thrd.ID.String()).Errorf("failed to create thread at collection: %s: ", err.Error())
	}

	if a.opts.CafeP2PAddr != nil {
		a.replicationWG.Add(1)
		go func() {
			defer a.replicationWG.Done()

			attempt := 0
			// todo: rewrite to job queue in badger
			for {
				attempt++
				p, err := a.t.AddReplicator(context.TODO(), thrd.ID, a.opts.CafeP2PAddr)
				if err != nil {
					log.Errorf("failed to add log replicator after %d attempt: %s", attempt, err.Error())
					select {
					case <-time.After(time.Second * 3 * time.Duration(attempt)):
					case <-a.shutdownStartsCh:
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

func (a *Anytype) GetSmartBlock(id string) (*smartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd.ID == thread.Undef {
		tid, err := thread.Decode(id)
		if err != nil {
			return nil, err
		}

		thrd, err = a.t.GetThread(context.TODO(), tid)
		if err != nil {
			return nil, err
		}
	}

	return &smartBlock{thread: thrd, node: a}, nil
}

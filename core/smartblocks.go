package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	util2 "github.com/anytypeio/go-anytype-library/util"
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

	err = a.t.DeleteThread(context.Background(), tid)
	if err != nil {
		return err
	}

	err = a.localStore.Pages.Delete(id)
	if err != nil {
		return err
	}

	return a.threadsCollection.Delete(db2.InstanceID(id))
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
func (a *Anytype) pullThread(ctx context.Context, id thread.ID) error {
	if sb, err := a.GetSmartBlock(id.String()); err == nil {
		snap, err := sb.GetLastSnapshot()
		if err != nil {
			if err == ErrBlockSnapshotNotFound {
				log.Infof("pullThread %s before: empty", id.String())
			} else {
				log.Errorf("pullThread %s before: %s", id.String(), err.Error())
			}
		} else {
			log.Infof("pullThread %s before: %s", id.String(), snap.State().String())
		}
	}

	err := a.t.PullThread(ctx, id)
	if err != nil {
		return err
	}

	if sb, err := a.GetSmartBlock(id.String()); err == nil {
		snap, err := sb.GetLastSnapshot()
		if err != nil {
			if err == ErrBlockSnapshotNotFound {
				log.Infof("pullThread %s after: empty", id.String())
			} else {
				log.Errorf("pullThread %s after: %s", id.String(), err.Error())
			}
		} else {
			log.Infof("pullThread %s after: %s", id.String(), snap.State().String())
			ls := snap.(smartBlockSnapshot)
			err := sb.indexSnapshot(&ls)
			if err != nil {
				log.Errorf("pullThread: failed to index the new snapshot for %s: %s", id.String(), err.Error())
			}
		}
	}

	go func() {
		// todo: do we need timeout here?
		err := a.smartBlockChanges.SendWithTimeout(id, time.Second*30)
		if err != nil {
			log.Errorf("processNewExternalThread: smartBlockChanges send failed: %s", err.Error())
		}
	}()

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
		d, err := db.NewDB(context.Background(), a.t, account.ID, db.WithNewDBRepoPath(filepath.Join(a.opts.Repo, "collections")))
		if err != nil {
			return err
		}

		a.db = d
		err = a.listenExternalNewThreads()
		if err != nil {
			return fmt.Errorf("failed to listen external new threads: %w", err)
		}
		a.threadsCollection = a.db.GetCollection(threadInfoCollectionName)

		if a.threadsCollection == nil {
			a.threadsCollection, err = a.db.NewCollection(threadInfoCollection)
			if err != nil {
				return err
			}
		}
	}

	accountThreadPullDone := make(chan struct{})
	if accountSelect {
		// accountSelect common case
		go func() {
			defer close(accountThreadPullDone)
			// pull only after adding collection to handle all events
			err = a.pullThread(context.TODO(), account.ID)
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

	// home
	home, _, err := a.predefinedThreadAdd(threadDerivedIndexHome, accountSelect, true, true)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Home = home.ID.String()

	return nil
}

func (a *Anytype) newBlockThread(blockType SmartBlockType) (thread.Info, error) {
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

	if a.opts.CafeP2PAddr != nil {
		a.replicationWG.Add(1)
		go func() {
			defer a.replicationWG.Done()

			// todo: rewrite to job queue in badger
			for {
				p, err := a.t.AddReplicator(context.TODO(), thrd.ID, a.opts.CafeP2PAddr)
				if err != nil {
					log.Errorf("failed to add log replicator: %s", err.Error())
					select {
					case <-time.After(time.Second * 30):
					case <-a.shutdownStartsCh:
						return
					}
					continue
				}

				log.With("thread", thrd.ID.String()).Infof("added log replicator: %s", p.String())
				threadInfo := threadInfo{
					ID:    db2.InstanceID(thrd.ID.String()),
					Key:   thrd.Key.String(),
					Addrs: util2.MultiAddressesToStrings(thrd.Addrs),
				}

				// todo: wait for threadsCollection to push?
				_, err = a.threadsCollection.Create(util.JSONFromInstance(threadInfo))
				if err != nil {
					log.With("thread", thrd.ID.String()).Errorf("failed to create thread at collection: %s: ", err.Error())
				}
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

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
		}
	}

	return nil
}

func (a *Anytype) createPredefinedBlocksIfNotExist(syncSnapshotIfNotExist bool) error {
	// account
	a.lock.Lock()
	defer a.lock.Unlock()
	account, err := a.predefinedThreadAdd(threadDerivedIndexAccount, false)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Account = account.ID.String()
	if a.db == nil {
		d, err := db.NewDB(context.Background(), a.t, account.ID, db.WithNewDBRepoPath(filepath.Join(a.repoPath, "collections")))
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

	// pull only after adding collection to handle all events
	if syncSnapshotIfNotExist {
		err = a.pullThread(context.TODO(), account.ID)
		if err != nil {
			return err
		}
	}

	// profile
	profile, err := a.predefinedThreadAdd(threadDerivedIndexProfilePage, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Profile = profile.ID.String()

	// archive
	archive, err := a.predefinedThreadAdd(threadDerivedIndexArchive, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Archive = archive.ID.String()

	// home
	home, err := a.predefinedThreadAdd(threadDerivedIndexHome, syncSnapshotIfNotExist)
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

	thrd, err := a.t.CreateThread(context.TODO(), thrdId, net.WithThreadKey(thread.NewKey(followKey, readKey)), net.WithLogKey(a.device))
	if err != nil {
		return thread.Info{}, err
	}

	if a.cafeP2PAddr != nil {
		a.replicationWG.Add(1)
		go func() {
			// todo: rewrite to job queue in badger
			for {
				defer a.replicationWG.Done()

				p, err := a.t.AddReplicator(context.TODO(), thrd.ID, a.cafeP2PAddr)
				if err != nil {
					log.Errorf("failed to add log replicator: %s", err.Error())
					select {
					case <-time.After(time.Second * 30):
					case <-a.shutdownCh:
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

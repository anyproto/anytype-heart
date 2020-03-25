package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/crypto/symmetric"

	"github.com/textileio/go-threads/core/thread"
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

/*func (a *Anytype) blockToVersion(block *model.Block, parentSmartBlockVersion BlockVersion, versionId string, user string, date *types.Timestamp) BlockVersion {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
		return &smartBlockSnapshot{
			model: &storage.SmartBlockWithMeta{
				Block: block,
			},
			versionId: versionId,
			user:      user,
			date:      date,
			node:      a,
		}
	default:
		return &SimpleBlockVersion{
			model:                   block,
			parentSmartBlockVersion: parentSmartBlockVersion,
			node:                    a,
		}
	}
}*/

func (a *Anytype) createPredefinedBlocksIfNotExist(syncSnapshotIfNotExist bool) error {
	// account
	account, err := a.predefinedThreadAdd(threadDerivedIndexAccount, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Account = account.ID.String()

	// profile
	profile, err := a.predefinedThreadAdd(threadDerivedIndexProfilePage, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Profile = profile.ID.String()

	// archive
	thread, err := a.predefinedThreadAdd(threadDerivedIndexArchive, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Archive = thread.ID.String()

	// home
	thread, err = a.predefinedThreadAdd(threadDerivedIndexHomeDashboard, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Home = thread.ID.String()

	return nil
}

func (a *Anytype) newBlockThread(blockType SmartBlockType) (thread.Info, error) {
	thrdId, err := newThreadID(thread.AccessControlled, blockType)
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

	return a.ts.CreateThread(context.TODO(), thrdId, net.ThreadKey(thread.NewKey(followKey, readKey)), net.LogKey(a.device))
}

func (a *Anytype) GetSmartBlock(id string) (*smartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd.ID == thread.Undef {
		tid, err := thread.Decode(id)
		if err != nil {
			return nil, err
		}

		thrd, err = a.ts.GetThread(context.TODO(), tid)
		if err != nil {
			return nil, err
		}
	}

	return &smartBlock{thread: thrd, node: a}, nil
}

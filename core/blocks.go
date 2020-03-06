package core

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	uuid "github.com/satori/go.uuid"
	"github.com/textileio/go-threads/core/thread"
)

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
	// profile
	profile, err := a.predefinedThreadAdd(threadDerivedIndexProfilePage, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}

	a.predefinedBlockIds.Profile = profile.ID.String()

	// archive
	thread, err := a.predefinedThreadAdd(threadDerivedIndexArchiveDashboard, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Archive = thread.ID.String()
	block, err := a.GetBlock(thread.ID.String())
	if err != nil {
		return err
	}

	if snapshot, _ := block.GetLastSnapshot(); snapshot == nil || snapshot.Blocks() == nil {
		// snapshot not yet created
		log.Debugf("create predefined archive block")
		_, err = block.PushSnapshot(SmartBlockState{}, &Meta{}, []*model.Block{})

		if err != nil {
			return err
		}
	}

	// home
	thread, err = a.predefinedThreadAdd(threadDerivedIndexHomeDashboard, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Home = thread.ID.String()

	block, err = a.GetBlock(thread.ID.String())
	if err != nil {
		return err
	}

	if snapshot, _ := block.GetLastSnapshot(); snapshot == nil || snapshot.Blocks() == nil {
		// snapshot not yet created
		log.Debugf("create predefined home block")
		archiveLinkId := block.ID() + "/" + uuid.NewV4().String()
		_, err = block.PushSnapshot(SmartBlockState{}, &Meta{}, []*model.Block{
			{
				Id: archiveLinkId,
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: a.predefinedBlockIds.Archive,
					Style:         model.BlockContentLink_Dataview,
				}},
			}})

		if err != nil {
			return err
		}
	}

	/*err = a.textile().SnapshotThreads()
	if err != nil {
		log.Errorf("SnapshotThreads error: %s")
	}*/
	return nil
}

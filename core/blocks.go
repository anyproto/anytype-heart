package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
	mh "github.com/multiformats/go-multihash"
	uuid "github.com/satori/go.uuid"
)

func (a *Anytype) GetBlock(id string) (Block, error) {
	_, err := mh.FromB58String(id)
	if err == nil {
		smartBlock, err := a.GetSmartBlock(id)
		if err != nil {
			return nil, err
		}

		return smartBlock, nil
	}

	// todo: allow to query simple blocks via smart blocks
	return nil, fmt.Errorf("for now only smartblocks are queriable")
}

func isSmartBlock(block *model.Block) bool {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
		return true
	default:
		return false
	}
}

func (a *Anytype) blockToVersion(block *model.Block, parentSmartBlockVersion BlockVersion, versionId string, user string, date *types.Timestamp) BlockVersion {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
		return &SmartBlockVersion{
			model: &storage.BlockWithMeta{
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
}

func (a *Anytype) createPredefinedBlocksIfNotExist(syncSnapshotIfNotExist bool) error {
	// archive
	thread, err := a.predefinedThreadAdd(threadDerivedIndexArchiveDashboard, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Archive = thread.Id
	block, err := a.GetBlock(thread.Id)
	if err != nil {
		return err
	}

	if version, _ := block.GetCurrentVersion(); version == nil || version.Model() == nil || version.Model().Content == nil {
		// version not yet created
		log.Debugf("create predefined archive block")
		_, err = block.AddVersion(&model.Block{
			Id: block.GetId(),
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"name": {Kind: &types.Value_StringValue{StringValue: "Archive"}},
					"icon": {Kind: &types.Value_StringValue{StringValue: ":package:"}},
				},
			},
			Content: &model.BlockContentOfDashboard{
				Dashboard: &model.BlockContentDashboard{
					Style: model.BlockContentDashboard_Archive,
				},
			},
		})

		if err != nil {
			return err
		}
	}

	// home
	thread, err = a.predefinedThreadAdd(threadDerivedIndexHomeDashboard, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}
	a.predefinedBlockIds.Home = thread.Id

	block, err = a.GetBlock(thread.Id)
	if err != nil {
		return err
	}

	if version, _ := block.GetCurrentVersion(); version == nil || version.Model() == nil || version.Model().Content == nil {
		// version not yet created
		log.Debugf("create predefined home block")
		archiveLinkId := block.GetId() + "/" + uuid.NewV4().String()
		_, err = block.AddVersions([]*model.Block{
			{
				Id:          block.GetId(),
				ChildrenIds: []string{archiveLinkId},
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"name": {Kind: &types.Value_StringValue{StringValue: "Home"}},
						"icon": {Kind: &types.Value_StringValue{StringValue: ":house:"}},
					},
				},
				Content: &model.BlockContentOfDashboard{
					Dashboard: &model.BlockContentDashboard{
						Style: model.BlockContentDashboard_MainScreen,
					},
				},
			},
			{
				Id: archiveLinkId,
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: a.predefinedBlockIds.Archive,
					Style:         model.BlockContentLink_Dataview,
				}},
			},
		})
		if err != nil {
			return err
		}
	}

	err = a.textile().SnapshotThreads()
	if err != nil {
		log.Errorf("SnapshotThreads error: %s")
	}
	return nil
}

package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
	uuid "github.com/satori/go.uuid"
	"github.com/textileio/go-threads/core/thread"
)

func (a *Anytype) GetBlock(id string) (Block, error) {
	_, err := thread.Decode(id)
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
	// profile
	profile, err := a.predefinedThreadAdd(threadDerivedIndexProfilePage, syncSnapshotIfNotExist)
	if err != nil {
		return err
	}

	a.predefinedBlockIds.Profile = profile.ID.String()
	// no need to create the version here

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
	a.predefinedBlockIds.Home = thread.ID.String()

	block, err = a.GetBlock(thread.ID.String())
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

	/*err = a.textile().SnapshotThreads()
	if err != nil {
		log.Errorf("SnapshotThreads error: %s")
	}*/
	return nil
}

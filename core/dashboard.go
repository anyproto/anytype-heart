package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/proto"
)

type Dashboard struct {
	*SmartBlock
}

func (dashboard *Dashboard) GetVersion(id string) (BlockVersion, error) {
	smartBlockVersion, err := dashboard.SmartBlock.GetVersion(id)
	if err != nil {
		return nil, fmt.Errorf("GetVersion error: %s", err.Error())
	}

	version := &DashboardVersion{SmartBlockVersion: smartBlockVersion}
	return version, nil
}

func (dashboard *Dashboard) GetCurrentVersion() (BlockVersion, error) {
	// todo: implement HEAD instead of always returning the last version
	versions, err := dashboard.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (dashboard *Dashboard) GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error) {
	sbversions, err := dashboard.SmartBlock.GetVersions(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	if len(sbversions) == 0 {
		return versions, nil
	}

	for _, sbversion := range sbversions {
		versions = append(versions, &DashboardVersion{SmartBlockVersion: sbversion})
	}

	return versions, nil
}

func (dashboard *Dashboard) mergeWithLastVersion(newVersion *PageVersion) *PageVersion {
	lastVersion, _ := dashboard.GetCurrentVersion()
	if lastVersion == nil {
		return newVersion
	}

	var dependentBlocks = lastVersion.DependentBlocks()
	newVersion.pb.BlockById = make(map[string]*model.Block, len(dependentBlocks))
	for id, dependentBlock := range dependentBlocks {
		newVersion.pb.BlockById[id] = dependentBlock.Model()
	}

	if newVersion.pb.Block.Fields == nil {
		newVersion.pb.Block.Fields = lastVersion.Model().Fields
	}

	if newVersion.pb.Block.Content == nil {
		newVersion.pb.Block.Content = lastVersion.Model().Content
	}

	if newVersion.pb.Block.ChildrenIds == nil {
		newVersion.pb.Block.ChildrenIds = lastVersion.Model().ChildrenIds
	}

	if newVersion.pb.Block.Permissions == nil {
		newVersion.pb.Block.Permissions = lastVersion.Model().Permissions
	}

	lastVersionB, _ := proto.Marshal(lastVersion.(*PageVersion).pb.Block.Content.(*model.BlockContentOfPage).Page)
	newVersionB, _ := proto.Marshal(newVersion.pb.Block.Content.(*model.BlockContentOfPage).Page)
	if string(lastVersionB) == string(newVersionB) {
		log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
		// do not insert the new version if no blocks have changed
		newVersion.versionId = lastVersion.VersionId()
		newVersion.user = lastVersion.User()
		newVersion.date = lastVersion.Date()
		return newVersion
	}
	return newVersion
}

// NewBlock should be used as constructor for the new block
func (dashboard *Dashboard) NewBlock(block model.Block) (Block, error) {
	return dashboard.newBlock(block, dashboard)
}

func (dashboard *Dashboard) AddVersion(block *model.Block) (BlockVersion, error) {
	if block.Id == "" {
		return nil, fmt.Errorf("block has empty id")
	}

	newVersion := &PageVersion{&SmartBlockVersion{pb: &storage.BlockWithDependentBlocks{Block: block}}}

	if newVersionContent, ok := block.Content.(*model.BlockContentOfDashboard); !ok {
		return nil, fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Block.Content = newVersionContent
	}

	newVersion = dashboard.mergeWithLastVersion(newVersion)
	if newVersion.versionId != "" {
		// nothing changes
		// todo: should we return error here to handle this specific case?
		return newVersion, nil
	}

	var err error
	newVersion.versionId, newVersion.user, newVersion.date, err = dashboard.SmartBlock.AddVersion(newVersion.pb)
	if err != nil {
		return nil, err
	}

	return newVersion, nil
}

func (dashboard *Dashboard) AddVersions(blocks []*model.Block) ([]BlockVersion, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks specified")
	}

	dashboardVersion := &DashboardVersion{&SmartBlockVersion{pb: &storage.BlockWithDependentBlocks{}}}
	lastVersion, _ := dashboard.GetCurrentVersion()
	if lastVersion != nil {
		var dependentBlocks = lastVersion.DependentBlocks()
		dashboardVersion.pb.BlockById = make(map[string]*model.Block, len(dependentBlocks))
		for id, dependentBlock := range dependentBlocks {
			dashboardVersion.pb.BlockById[id] = dependentBlock.Model()
		}
	} else {
		dashboardVersion.pb.BlockById = make(map[string]*model.Block, len(blocks))
	}

	blockVersions := make([]BlockVersion, 0, len(blocks))

	for _, block := range blocks {
		if block.Id == "" {
			return nil, fmt.Errorf("block has empty id")
		}

		if block.Id == dashboard.GetId() {
			if block.ChildrenIds != nil {
				dashboardVersion.pb.Block.ChildrenIds = block.ChildrenIds
			}

			if block.Content != nil {
				dashboardVersion.pb.Block.Content = block.Content
			}

			if block.Fields != nil {
				dashboardVersion.pb.Block.Fields = block.Fields
			}

			if block.Permissions != nil {
				dashboardVersion.pb.Block.Permissions = block.Permissions
			}

			// only add dashboardVersion in case it was intentionally passed to AddVersions blocks
			blockVersions = append(blockVersions, dashboardVersion)
		} else {

			if _, exists := dashboardVersion.pb.BlockById[block.Id]; !exists {
				dashboardVersion.pb.BlockById[block.Id] = block
			} else {
				if isSmartBlock(block) {
					// ignore smart blocks as they will be added on the fly
					continue
				}

				if block.ChildrenIds != nil {
					dashboardVersion.pb.BlockById[block.Id].ChildrenIds = block.ChildrenIds
				}

				if block.Permissions != nil {
					dashboardVersion.pb.BlockById[block.Id].Permissions = block.Permissions
				}

				if block.Fields != nil {
					dashboardVersion.pb.BlockById[block.Id].Fields = block.Fields
				}

				if block.Content != nil {
					dashboardVersion.pb.BlockById[block.Id].Content = block.Content
				}
			}

			blockVersions = append(blockVersions, dashboard.node.blockToVersion(block, dashboardVersion, "", "", nil))
		}
	}

	return blockVersions, nil
}

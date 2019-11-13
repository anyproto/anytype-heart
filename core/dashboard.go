package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

type Dashboard struct {
	SmartBlock
}

func (dashboard *Dashboard) GetVersion(id string) (BlockVersion, error) {
	file, block, err := dashboard.SmartBlock.GetVersionBlock(id)
	if err != nil {
		return nil, fmt.Errorf("GetVersionBlock error: %s", err.Error())
	}

	version := &DashboardVersion{pb: block, VersionId: file.Block, Date: util.CastTimestampToGogo(file.Date), User: file.User.Address}

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
	files, blocks, err := dashboard.SmartBlock.GetVersionsFiles(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	if len(files) == 0 {
		return versions, nil
	}

	for index, item := range files {
		version := &DashboardVersion{VersionId: item.Block, Date: util.CastTimestampToGogo(item.Date), User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		version.pb = blocks[index]
		versions = append(versions, version)
	}

	return versions, nil
}

func (dashboard *Dashboard) AddVersion(dependentBlocks map[string]BlockVersion, fields *types.Struct, children []string, content model.IsBlockContent) error {
	newVersion := &DashboardVersion{pb: &storage.BlockWithDependentBlocks{}}

	if newVersionContent, ok := content.(*model.BlockContentOfDashboard); !ok {
		return fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Block.Content = newVersionContent
	}

	lastVersion, err := dashboard.GetCurrentVersion()
	if lastVersion != nil {
		if fields == nil {
			fields = lastVersion.GetFields()
		}

		if content == nil {
			content = lastVersion.GetContent()
		}

		if dependentBlocks == nil {
			dependentBlocks = lastVersion.GetDependentBlocks()
		}

		if children == nil {
			children = lastVersion.GetChildrenIds()
		}

		lastVersionB, _ := proto.Marshal(lastVersion.(*DashboardVersion).pb.Block.Content.(*model.BlockContentOfDashboard).Dashboard)
		newVersionB, _ := proto.Marshal(newVersion.pb.Block.Content.(*model.BlockContentOfDashboard).Dashboard)
		if string(lastVersionB) == string(newVersionB) {
			log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
			// do not insert the new version if no blocks have changed
			newVersion.VersionId = lastVersion.GetVersionId()
			newVersion.User = lastVersion.GetUser()
			newVersion.Date = lastVersion.GetDate()
		} else {
			fmt.Printf("version differs:new %s\n%s\n\n---\n\nlast %s\n%s", newVersion.VersionId, string(newVersionB), lastVersion.GetVersionId(), string(lastVersionB))
		}
	}

	if newVersion.VersionId != "" {
		/*	err = dashboard.Modify(dashboard.ChildrenIds, newVersion.Name, newVersion.Icon)
			if err != nil {
				return nil, err
			}*/
		return nil
	}

	newVersion.VersionId, newVersion.User, newVersion.Date, err = dashboard.SmartBlock.AddVersion(newVersion.pb)
	if err != nil {
		return err
	}
	return nil
}

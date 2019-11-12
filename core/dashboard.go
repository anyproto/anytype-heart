package core

import (
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/gogo/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
)

type Dashboard struct {
	SmartBlock
}

func (dashboard *Dashboard) GetVersion(id string) (BlockVersion, error) {
	files, err := dashboard.node.Textile.Node().File(id)
	if err != nil {
		return nil, err
	}

	if len(files.Files) == 0 {
		return nil, errors.New("version block not found")
	}

	blockVersion := &pb.Block{}

	plaintext, err := readFile(dashboard.node.Textile.Node(), files.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	err = proto.Unmarshal(plaintext, blockVersion)
	if err != nil {
		return nil, fmt.Errorf("dashboard version proto unmarshal error: %s", err.Error())
	}

	version := &DashboardVersion{pb: blockVersion, VersionId: files.Block, Date: files.Date, User: files.User.Address}

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
		version := &DashboardVersion{VersionId: item.Block, Date: item.Date, User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		version.pb = blocks[index]
		versions = append(versions, version)
	}

	return versions, nil
}

func (dashboard *Dashboard) AddVersion(dependentBlocks map[string]BlockVersion, fields *structpb.Struct, children []string, content pb.IsBlockContent) error {
	newVersion := &DashboardVersion{pb: &pb.Block{}}

	if newVersionContent, ok := content.(*pb.BlockContentOfDashboard); !ok {
		return fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Content = newVersionContent
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

		lastVersionB, _ := proto.Marshal(lastVersion.(*DashboardVersion).pb.Content.(*pb.BlockContentOfDashboard).Dashboard)
		newVersionB, _ := proto.Marshal(newVersion.pb.Content.(*pb.BlockContentOfDashboard).Dashboard)
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

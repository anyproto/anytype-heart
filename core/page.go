package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

const (
	mergeFileCaption = "Merge"
	defaultDocName   = "Untitled"
	smartBlockSchema = "QmTShDQr2PeWEXE5D8r77Lz5NeyLK7NNRENXtQHTvqo9F5"
)

var errorNotFound = fmt.Errorf("not found")

type Page struct {
	SmartBlock
}

func (page *Page) GetVersion(id string) (BlockVersion, error) {
	file, block, err := page.SmartBlock.GetVersionBlock(id)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	version := &PageVersion{pb: block, VersionId: file.Block, Date: util.CastTimestampToGogo(file.Date), User: file.User.Address}

	return version, nil
}

func (page *Page) GetCurrentVersion() (BlockVersion, error) {
	// todo: implement HEAD instead of always returning the last version
	versions, err := page.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (page *Page) GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error) {
	files, blocks, err := page.SmartBlock.GetVersionsFiles(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	if len(files) == 0 {
		return versions, nil
	}

	for index, item := range files {
		version := &PageVersion{VersionId: item.Block, Date: util.CastTimestampToGogo(item.Date), User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		version.pb = blocks[index]
		versions = append(versions, version)
	}

	return versions, nil
}

func (page *Page) AddVersion(dependentBlocks map[string]BlockVersion, fields *types.Struct, children []string, content model.IsBlockContent) error {
	newVersion := &PageVersion{pb: &storage.BlockWithDependentBlocks{}}

	if newVersionContent, ok := content.(*model.BlockContentOfPage); !ok {
		return fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Block.Content = newVersionContent
	}

	lastVersion, err := page.GetCurrentVersion()
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

		lastVersionB, _ := proto.Marshal(lastVersion.(*PageVersion).pb.Block.Content.(*model.BlockContentOfPage).Page)
		newVersionB, _ := proto.Marshal(newVersion.pb.Block.Content.(*model.BlockContentOfPage).Page)
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
		/*	err = page.Modify(page.ChildrenIds, newVersion.Name, newVersion.Icon)
			if err != nil {
				return nil, err
			}*/
		return nil
	}

	newVersion.VersionId, newVersion.User, newVersion.Date, err = page.SmartBlock.AddVersion(newVersion.pb)
	if err != nil {
		return err
	}
	return nil
}

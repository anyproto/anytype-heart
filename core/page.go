package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/proto"
)

const (
	mergeFileCaption = "Merge"
	defaultDocName   = "Untitled"
	smartBlockSchema = "QmTShDQr2PeWEXE5D8r77Lz5NeyLK7NNRENXtQHTvqo9F5"
)

var errorNotFound = fmt.Errorf("not found")

type Page struct {
	*SmartBlock
}

func (page *Page) GetVersion(id string) (BlockVersion, error) {
	smartBlockVersion, err := page.SmartBlock.GetVersion(id)
	if err != nil {
		return nil, fmt.Errorf("GetVersion error: %s", err.Error())
	}

	version := &PageVersion{SmartBlockVersion: smartBlockVersion}
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
	sbversions, err := page.SmartBlock.GetVersions(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	if len(sbversions) == 0 {
		return versions, nil
	}

	for _, sbversion := range sbversions {
		versions = append(versions, &PageVersion{SmartBlockVersion: sbversion})
	}

	return versions, nil
}

func (page *Page) mergeWithLastVersion(newVersion *PageVersion) *PageVersion {
	lastVersion, _ := page.GetCurrentVersion()
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
func (page *Page) NewBlock(block model.Block) (Block, error) {
	return page.newBlock(block, page)
}

func (page *Page) AddVersion(block *model.Block) (BlockVersion, error) {
	if block.Id == "" {
		return nil, fmt.Errorf("block has empty id")
	}

	newVersion := &PageVersion{&SmartBlockVersion{pb: &storage.BlockWithDependentBlocks{Block: block}}}

	if newVersionContent, ok := block.Content.(*model.BlockContentOfPage); !ok {
		return nil, fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Block.Content = newVersionContent
	}

	newVersion = page.mergeWithLastVersion(newVersion)
	if newVersion.versionId != "" {
		// nothing changes
		// todo: should we return error here to handle this specific case?
		return newVersion, nil
	}

	var err error
	newVersion.versionId, newVersion.user, newVersion.date, err = page.SmartBlock.AddVersion(newVersion.pb)
	if err != nil {
		return nil, err
	}

	return newVersion, nil
}

func (page *Page) AddVersions(blocks []*model.Block) ([]BlockVersion, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks specified")
	}

	pageVersion := &PageVersion{&SmartBlockVersion{pb: &storage.BlockWithDependentBlocks{}}}
	lastVersion, _ := page.GetCurrentVersion()
	if lastVersion != nil {
		var dependentBlocks = lastVersion.DependentBlocks()
		pageVersion.pb.BlockById = make(map[string]*model.Block, len(dependentBlocks))
		for id, dependentBlock := range dependentBlocks {
			pageVersion.pb.BlockById[id] = dependentBlock.Model()
		}
	} else {
		pageVersion.pb.BlockById = make(map[string]*model.Block, len(blocks))
	}

	blockVersions := make([]BlockVersion, 0, len(blocks))

	for _, block := range blocks {
		if block.Id == "" {
			return nil, fmt.Errorf("block has empty id")
		}

		if block.Id == page.GetId() {
			if block.ChildrenIds != nil {
				pageVersion.pb.Block.ChildrenIds = block.ChildrenIds
			}

			if block.Content != nil {
				pageVersion.pb.Block.Content = block.Content
			}

			if block.Fields != nil {
				pageVersion.pb.Block.Fields = block.Fields
			}

			if block.Permissions != nil {
				pageVersion.pb.Block.Permissions = block.Permissions
			}

			// only add pageVersion in case it was intentionally passed to AddVersions blocks
			blockVersions = append(blockVersions, pageVersion)
		} else {

			if _, exists := pageVersion.pb.BlockById[block.Id]; !exists {
				pageVersion.pb.BlockById[block.Id] = block
			} else {
				if isSmartBlock(block) {
					// ignore smart blocks as they will be added on the fly
					continue
				}

				if block.ChildrenIds != nil {
					pageVersion.pb.BlockById[block.Id].ChildrenIds = block.ChildrenIds
				}

				if block.Permissions != nil {
					pageVersion.pb.BlockById[block.Id].Permissions = block.Permissions
				}

				if block.Fields != nil {
					pageVersion.pb.BlockById[block.Id].Fields = block.Fields
				}

				if block.Content != nil {
					pageVersion.pb.BlockById[block.Id].Content = block.Content
				}
			}

			blockVersions = append(blockVersions, page.node.blockToVersion(block, pageVersion, "", "", nil))
		}
	}

	return blockVersions, nil
}

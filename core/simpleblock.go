package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
)

var ErrorNotSupportedForSimpleBlocks = fmt.Errorf("not supported for simple blocks")

type SimpleBlock struct {
	id               string
	parentSmartBlock *SmartBlock
	node             *Anytype
}

func (simpleBlock *SimpleBlock) GetId() string {
	return simpleBlock.parentSmartBlock.GetId() + "/" + simpleBlock.id
}

func (simpleBlock *SimpleBlock) GetVersion(id string) (BlockVersion, error) {
	parentBlockVersion, err := simpleBlock.parentSmartBlock.GetVersion(id)
	if err != nil {
		return nil, err
	}

	if simpleBlockVersion, exists := parentBlockVersion.DependentBlocks()[simpleBlock.id]; !exists {
		return nil, fmt.Errorf("simpleBlock not found for this version")
	} else {
		return simpleBlockVersion, nil
	}
}

func (simpleBlock *SimpleBlock) GetVersionMeta(id string) (BlockVersionMeta, error) {
	return nil, ErrorNotSupportedForSimpleBlocks
}

func (simpleBlock *SimpleBlock) GetCurrentVersionId() (string, error) {
	parentBlockVersion, err := simpleBlock.parentSmartBlock.GetCurrentVersionId()
	if err != nil {
		return "", err
	}

	return parentBlockVersion, nil
}

func (simpleBlock *SimpleBlock) GetCurrentVersion() (BlockVersion, error) {
	parentBlockVersion, err := simpleBlock.parentSmartBlock.GetCurrentVersion()
	if err != nil {
		return nil, err
	}

	if simpleBlockVersion, exists := parentBlockVersion.DependentBlocks()[simpleBlock.id]; !exists {
		return nil, ErrorNoBlockVersionsFound
	} else {
		return simpleBlockVersion, nil
	}
}

func (simpleBlock *SimpleBlock) GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error) {
	parentBlockVersions, err := simpleBlock.parentSmartBlock.GetVersions(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	for _, parentBlockVersion := range parentBlockVersions {
		if simpleBlockVersion, exists := parentBlockVersion.DependentBlocks()[simpleBlock.id]; !exists {
			// simpleBlock doesn't exist for this version
			// because versions sorted from the newer to older we can break here
			break
		} else {
			versions = append(versions, simpleBlockVersion)
		}
	}

	return versions, nil
}

// NewBlock should be used as constructor for the new block
func (simpleBlock *SimpleBlock) NewBlock(block model.Block) (Block, error) {
	return simpleBlock.parentSmartBlock.NewBlock(block)
}

func (simpleBlock *SimpleBlock) AddVersion(block *model.Block) (BlockVersion, error) {
	switch block.Content.(type) {
	case *model.BlockContentOfPage, *model.BlockContentOfDashboard, *model.BlockContentOfDataview:
		return nil, fmt.Errorf("got smartBlock model instead of simpleBlock")
	}

	lastVersion, _ := simpleBlock.GetCurrentVersion()
	if lastVersion != nil {
		if block.Content == nil {
			block.Content = lastVersion.Model().Content
		}

		if block.ChildrenIds == nil {
			block.ChildrenIds = lastVersion.Model().ChildrenIds
		}
	}

	versions, err := simpleBlock.parentSmartBlock.AddVersions([]*model.Block{block})
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("failed to addversion to the parent smartblock")
	}

	return versions[0], nil
}

func (simpleBlock *SimpleBlock) AddVersions(blocks []*model.Block) ([]BlockVersion, error) {
	return nil, ErrorNotSupportedForSimpleBlocks
}

func (simpleBlock *SimpleBlock) EmptyVersion() BlockVersion {
	restr := blockRestrictionsEmpty()
	return &SimpleBlockVersion{
		model: &model.Block{
			Id:           simpleBlock.id,
			Restrictions: &restr,
		},
		//todo: not possible to pass parentSmartBlockVersion here
		// do we actually need it?
		//parentSmartBlockVersion:
		node: simpleBlock.node,
	}
}

func (simpleBlock *SimpleBlock) SubscribeNewVersionsOfBlocks(sinceVersionId string, includeSinceVersion bool, blocks chan<- []BlockVersion) (cancelFunc func(), err error) {
	// not supported yet, need to use parent smartblock instead
	close(blocks)

	return nil, ErrorNotSupportedForSimpleBlocks
}

func (simpleBlock *SimpleBlock) SubscribeMetaOfNewVersionsOfBlock(sinceVersionId string, includeSinceVersion bool, block chan<- BlockVersionMeta) (cancelFunc func(), err error) {
	// not supported
	close(block)

	return nil, ErrorNotSupportedForSimpleBlocks
}

func (simpleBlock *SimpleBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func(), err error) {
	// not supported
	close(events)
	return nil, ErrorNotSupportedForSimpleBlocks
}

func (simpleBlock *SimpleBlock) PublishClientEvent(event proto.Message) error {
	// not supported
	return ErrorNotSupportedForSimpleBlocks
}

package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

type SimpleBlock struct {
	id               string
	parentSmartBlock Block
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

	if simpleBlockVersion, exists := parentBlockVersion.GetDependentBlocks()[simpleBlock.id]; !exists {
		return nil, fmt.Errorf("simpleBlock not found for this version")
	} else {
		return simpleBlockVersion, nil
	}
}

func (simpleBlock *SimpleBlock) GetCurrentVersion() (BlockVersion, error) {
	parentBlockVersion, err := simpleBlock.parentSmartBlock.GetCurrentVersion()
	if err != nil {
		return nil, err
	}

	if simpleBlockVersion, exists := parentBlockVersion.GetDependentBlocks()[simpleBlock.id]; !exists {
		return nil, fmt.Errorf("simpleBlock not found for this version")
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
		if simpleBlockVersion, exists := parentBlockVersion.GetDependentBlocks()[simpleBlock.id]; !exists {
			// simpleBlock doesn't exist for this version
			// because versions sorted from the newer to older we can break here
			break
		} else {
			versions = append(versions, simpleBlockVersion)
		}
	}

	return versions, nil
}

func (simpleBlock *SimpleBlock) AddVersion(dependentBlocks map[string]BlockVersion, fields *types.Struct, children []string, content model.IsBlockContent) error {
	if fields != nil {
		return fmt.Errorf("simpleBlock simpleBlocks can't store fields")
	}

	if dependentBlocks != nil {
		return fmt.Errorf("simpleBlock simpleBlocks can't store dependent simpleBlocks")
	}

	newVersion := &SimpleBlockVersion{pb: &model.Block{}}

	switch content.(type) {
	case *model.BlockContentOfPage, *model.BlockContentOfDashboard, *model.BlockContentOfDataview:
		return fmt.Errorf("unxpected smartsimpleBlock type")
	}

	lastVersion, err := simpleBlock.GetCurrentVersion()
	if lastVersion != nil {
		// todo: fix a warning here
		if content == nil {
			content = lastVersion.GetContent()
		}

		if children == nil {
			children = lastVersion.GetChildrenIds()
		}
	}
	newVersion.pb.Content = content
	newVersion.pb.ChildrenIds = children

	parentBlockVersion, err := simpleBlock.parentSmartBlock.GetCurrentVersion()
	if err != nil {
		return err
	}

	parentSmartBlockDependentBlocks := parentBlockVersion.GetDependentBlocks()

	parentSmartBlockDependentBlocks[simpleBlock.id] = newVersion

	return simpleBlock.parentSmartBlock.AddVersion(parentSmartBlockDependentBlocks, nil, nil, nil)
}

func (simpleBlock *SimpleBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func()) {
	// not supported
	close(events)
	return func() {}
}

func (simpleBlock *SimpleBlock) PublishClientEvent(event proto.Message) {
	// not supported
	return
}

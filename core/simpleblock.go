package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/gogo/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
)

type SimpleBlock struct {
	id               string
	parentSmartBlock Block
	node             *Anytype
}

func (block *SimpleBlock) GetId() string {
	return block.parentSmartBlock.GetId() + "/" + block.id
}

func (block *SimpleBlock) GetVersion(id string) (BlockVersion, error) {
	parentBlockVersion, err := block.parentSmartBlock.GetVersion(id)
	if err != nil {
		return nil, err
	}

	if blockVersion, exists := parentBlockVersion.GetDependentBlocks()[block.id]; !exists {
		return nil, fmt.Errorf("block not found for this version")
	} else {
		return blockVersion, nil
	}
}

func (block *SimpleBlock) GetCurrentVersion() (BlockVersion, error) {
	parentBlockVersion, err := block.parentSmartBlock.GetCurrentVersion()
	if err != nil {
		return nil, err
	}

	if blockVersion, exists := parentBlockVersion.GetDependentBlocks()[block.id]; !exists {
		return nil, fmt.Errorf("block not found for this version")
	} else {
		return blockVersion, nil
	}
}

func (block *SimpleBlock) GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error) {
	parentBlockVersions, err := block.parentSmartBlock.GetVersions(offset, limit, metaOnly)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	for _, parentBlockVersion := range parentBlockVersions {
		if blockVersion, exists := parentBlockVersion.GetDependentBlocks()[block.id]; !exists {
			// block doesn't exist for this version
			// because versions sorted from the newer to older we can break here
			break
		} else {
			versions = append(versions, blockVersion)
		}
	}

	return versions, nil
}

func (block *SimpleBlock) AddVersion(dependentBlocks map[string]BlockVersion, fields *structpb.Struct, children []string, content pb.IsBlockContent) error {
	if fields != nil {
		return fmt.Errorf("simple blocks can't store fields")
	}

	if dependentBlocks != nil {
		return fmt.Errorf("simple blocks can't store dependent blocks")
	}

	newVersion := &SimpleBlockVersion{pb: &pb.Block{}}

	switch content.(type) {
	case *pb.BlockContentOfPage, *pb.BlockContentOfDashboard, *pb.BlockContentOfDataview:
		return fmt.Errorf("unxpected smartblock type")
	}

	lastVersion, err := block.GetCurrentVersion()
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

	parentBlockVersion, err := block.parentSmartBlock.GetCurrentVersion()
	if err != nil {
		return err
	}

	parentSmartBlockDependentBlocks := parentBlockVersion.GetDependentBlocks()

	parentSmartBlockDependentBlocks[block.id] = newVersion

	return block.parentSmartBlock.AddVersion(parentSmartBlockDependentBlocks, nil, nil, nil)
}

func (block *SimpleBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func()) {
	// not supported
	close(events)
	return func() {}
}

func (block *SimpleBlock) PublishClientEvent(event proto.Message) {
	// not supported
	return
}

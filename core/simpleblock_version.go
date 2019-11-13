package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
)

type SimpleBlockVersion struct {
	pb                      *model.Block
	parentSmartBlockVersion BlockVersion
	node                    *Anytype
}

func (blockVersion *SimpleBlockVersion) GetBlockId() string {
	return blockVersion.parentSmartBlockVersion.GetBlockId() + "/" + blockVersion.pb.Id
}

func (blockVersion *SimpleBlockVersion) GetVersionId() string {
	return blockVersion.parentSmartBlockVersion.GetVersionId()
}

func (blockVersion *SimpleBlockVersion) GetUser() string {
	return blockVersion.parentSmartBlockVersion.GetUser()
}

func (blockVersion *SimpleBlockVersion) GetDate() *types.Timestamp {
	return blockVersion.parentSmartBlockVersion.GetDate()
}

func (blockVersion *SimpleBlockVersion) GetChildrenIds() []string {
	return blockVersion.pb.ChildrenIds
}

func (blockVersion *SimpleBlockVersion) GetPermissions() *model.BlockPermissions {
	return blockVersion.pb.GetPermissions()
}

func (blockVersion *SimpleBlockVersion) GetExternalFields() *types.Struct {
	// simple blocks can't have fields
	return nil
}

func (blockVersion *SimpleBlockVersion) GetFields() *types.Struct {
	// simple blocks can't have fields
	return nil
}

func (blockVersion *SimpleBlockVersion) GetContent() model.IsBlockContent {
	return blockVersion.pb.Content
}

func (blockVersion *SimpleBlockVersion) GetDependentBlocks() map[string]BlockVersion {
	// simple blocks don't store dependent blocks
	return nil
}

func (blockVersion *SimpleBlockVersion) GetNewVersionsOfBlocks(blocks chan<- []BlockVersion) (cancelFunc func()) {
	// not supported yet, need to use parent smartblock instead
	close(blocks)
	return func() {}
}

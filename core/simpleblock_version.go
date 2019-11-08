package core

import (
	"github.com/anytypeio/go-anytype-library/pb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type SimpleBlockVersion struct {
	pb                      *pb.Block
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

func (blockVersion *SimpleBlockVersion) GetDate() *timestamp.Timestamp {
	return blockVersion.parentSmartBlockVersion.GetDate()
}

func (blockVersion *SimpleBlockVersion) GetChildrenIds() []string {
	return blockVersion.pb.ChildrenIds
}

func (blockVersion *SimpleBlockVersion) GetPermissions() *pb.BlockPermissions {
	return blockVersion.pb.GetPermissions()
}

func (blockVersion *SimpleBlockVersion) GetExternalFields() *structpb.Struct {
	// simple blocks can't have fields
	return nil
}

func (blockVersion *SimpleBlockVersion) GetFields() *structpb.Struct {
	// simple blocks can't have fields
	return nil
}

func (blockVersion *SimpleBlockVersion) GetContent() pb.IsBlockContent {
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

package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
)

type PageVersion struct {
	pb        *storage.BlockWithDependentBlocks
	VersionId string
	User      string
	Date      *types.Timestamp
}

func (pageVersion *PageVersion) GetVersionId() string {
	return pageVersion.VersionId
}

func (pageVersion *PageVersion) GetBlockId() string {
	return pageVersion.pb.Block.Id
}

func (pageVersion *PageVersion) GetUser() string {
	return pageVersion.User
}

func (pageVersion *PageVersion) GetDate() *types.Timestamp {
	return pageVersion.Date
}

func (pageVersion *PageVersion) GetNewVersionsOfBlocks(blocks chan<- []BlockVersion) (cancelFunc func()) {
	// todo: to be implemented
	close(blocks)
	return func() {}
}

func (ver *PageVersion) GetDependentBlocks() map[string]BlockVersion {
	var m = make(map[string]BlockVersion, len(ver.pb.BlockById))
	for blockId, block := range ver.pb.BlockById {
		switch block.Content.(type) {
		case *model.BlockContentOfDashboard:
			m[blockId] = &DashboardVersion{pb: &storage.BlockWithDependentBlocks{Block: block}, VersionId: ver.VersionId, User: ver.User, Date: ver.Date}
		case *model.BlockContentOfPage:
			m[blockId] = &PageVersion{pb: &storage.BlockWithDependentBlocks{Block: block}, VersionId: ver.VersionId, User: ver.User, Date: ver.Date}
		}
	}
	return m
}

func (pageVersion *PageVersion) GetChildrenIds() []string {
	return pageVersion.pb.Block.ChildrenIds
}

func (ver *PageVersion) GetFields() *types.Struct {
	return ver.pb.Block.Fields
}

func (ver *PageVersion) GetExternalFields() *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		"name": ver.pb.Block.Fields.Fields["name"],
		"icon": ver.pb.Block.Fields.Fields["icon"],
	}}
}

func (ver *PageVersion) GetPermissions() *model.BlockPermissions {
	return ver.pb.Block.Permissions
}

func (ver *PageVersion) GetContent() model.IsBlockContent {
	return ver.pb.Block.Content
}

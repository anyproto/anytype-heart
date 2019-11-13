package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
)

type DashboardVersion struct {
	pb        *storage.BlockWithDependentBlocks
	VersionId string
	User      string
	Date      *types.Timestamp
}

func (ver *DashboardVersion) GetVersionId() string {
	return ver.VersionId
}

func (ver *DashboardVersion) GetBlockId() string {
	return ver.pb.Block.Id
}

func (ver *DashboardVersion) GetUser() string {
	return ver.User
}

func (ver *DashboardVersion) GetDate() *types.Timestamp {
	return ver.Date
}

func (ver *DashboardVersion) GetNewVersionsOfBlocks(blocks chan<- []BlockVersion) (cancelFunc func()) {
	// todo: to be implemented
	close(blocks)
	return func() {}
}

func (ver *DashboardVersion) GetDependentBlocks() map[string]BlockVersion {
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

func (ver *DashboardVersion) GetChildrenIds() []string {
	return ver.pb.Block.ChildrenIds
}

func (ver *DashboardVersion) GetFields() *types.Struct {
	return ver.pb.Block.Fields
}

func (ver *DashboardVersion) GetExternalFields() *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		"name": ver.pb.Block.Fields.Fields["name"],
		"icon": ver.pb.Block.Fields.Fields["icon"],
	}}
}

func (ver *DashboardVersion) GetPermissions() *model.BlockPermissions {
	return ver.pb.Block.Permissions
}

func (ver *DashboardVersion) GetContent() model.IsBlockContent {
	return ver.pb.Block.Content
}

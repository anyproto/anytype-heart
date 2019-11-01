package core

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type DashboardVersion struct {
	VersionId   string
	DashboardId string
	User        string
	Date        *timestamp.Timestamp
	Content     *BlockContentDashboard
}

func (dashboardVersion *DashboardVersion) GetVersionId() string {
	return dashboardVersion.VersionId
}

func (dashboardVersion *DashboardVersion) GetBlockId() string {
	return dashboardVersion.DashboardId
}

func (dashboardVersion *DashboardVersion) GetUser() string {
	return dashboardVersion.User
}

func (dashboardVersion *DashboardVersion) GetDate() *timestamp.Timestamp {
	return dashboardVersion.Date
}

func (dashboardVersion *DashboardVersion) GetBlocks() map[string]*Block {
	// todo: remove non-smart blocks
	return dashboardVersion.Content.Blocks.BlockById
}

func (ver *DashboardVersion) GetFields() *structpb.Struct {
	return ver.GetExternalFields()
}

func (ver *DashboardVersion) GetExternalFields() *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": {Kind: &structpb.Value_StringValue{""}},
		"icon": {Kind: &structpb.Value_StringValue{""}},
	}}
}

func (ver *DashboardVersion) GetSmartBlocksTree(anytype *Anytype) ([]SmartBlock, error) {
	return []SmartBlock{}, nil
}

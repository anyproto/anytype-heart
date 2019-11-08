package core

import (
	"github.com/anytypeio/go-anytype-library/pb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type DashboardVersion struct {
	pb        *pb.Block
	VersionId string
	User      string
	Date      *timestamp.Timestamp
}

func (ver *DashboardVersion) GetVersionId() string {
	return ver.VersionId
}

func (ver *DashboardVersion) GetBlockId() string {
	return ver.pb.Id
}

func (ver *DashboardVersion) GetUser() string {
	return ver.User
}

func (ver *DashboardVersion) GetDate() *timestamp.Timestamp {
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
		m[blockId] = &DashboardVersion{pb: block, VersionId: ver.VersionId, User: ver.User, Date: ver.Date}
	}
	return m
}

func (ver *DashboardVersion) GetChildrenIds() []string {
	return ver.pb.ChildrenIds
}

func (ver *DashboardVersion) GetFields() *structpb.Struct {
	return ver.pb.Fields
}

func (ver *DashboardVersion) GetExternalFields() *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": ver.pb.Fields.Fields["name"],
		"icon": ver.pb.Fields.Fields["icon"],
	}}
}

func (ver *DashboardVersion) GetPermissions() *pb.BlockPermissions {
	return ver.pb.Permissions
}

func (ver *DashboardVersion) GetContent() pb.IsBlockContent {
	return ver.pb.Content
}

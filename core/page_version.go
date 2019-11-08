package core

import (
	"github.com/anytypeio/go-anytype-library/pb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type PageVersion struct {
	pb        *pb.Block
	VersionId string
	User      string
	Date      *timestamp.Timestamp
}

func (pageVersion *PageVersion) GetVersionId() string {
	return pageVersion.VersionId
}

func (pageVersion *PageVersion) GetBlockId() string {
	return pageVersion.pb.Id
}

func (pageVersion *PageVersion) GetUser() string {
	return pageVersion.User
}

func (pageVersion *PageVersion) GetDate() *timestamp.Timestamp {
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
		m[blockId] = &PageVersion{pb: block, VersionId: ver.VersionId, User: ver.User, Date: ver.Date}
	}
	return m
}

func (pageVersion *PageVersion) GetChildrenIds() []string {
	return pageVersion.pb.ChildrenIds
}

func (ver *PageVersion) GetFields() *structpb.Struct {
	return ver.pb.Fields
}

func (ver *PageVersion) GetExternalFields() *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": ver.pb.Fields.Fields["name"],
		"icon": ver.pb.Fields.Fields["icon"],
	}}
}

func (ver *PageVersion) GetPermissions() *pb.BlockPermissions {
	return ver.pb.Permissions
}

func (ver *PageVersion) GetContent() pb.IsBlockContent {
	return ver.pb.Content
}

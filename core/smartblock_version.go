package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
)

type SmartBlockVersion struct {
	pb        *storage.BlockWithDependentBlocks
	versionId string
	user      string
	date      *types.Timestamp
	node      *Anytype
}

func (version *SmartBlockVersion) Model() *model.Block {
	return version.pb.Block
}

func (version *SmartBlockVersion) VersionId() string {
	return version.versionId
}

func (version *SmartBlockVersion) User() string {
	return version.user
}

func (version *SmartBlockVersion) Date() *types.Timestamp {
	return version.date
}

func (version *SmartBlockVersion) GetContent() model.IsBlockContent {
	return version.pb.Block.Content
}

func (version *SmartBlockVersion) DependentBlocks() map[string]BlockVersion {
	var m = make(map[string]BlockVersion, len(version.pb.BlockById))
	for blockId, block := range version.pb.BlockById {
		switch block.Content.(type) {
		case *model.BlockContentOfDashboard:
			m[blockId] = &DashboardVersion{&SmartBlockVersion{
				pb: &storage.BlockWithDependentBlocks{
					Block: block,
				},
				versionId: version.versionId,
				user:      version.user,
				date:      version.date,
			}}
		case *model.BlockContentOfPage:
			m[blockId] = &PageVersion{&SmartBlockVersion{
				pb:        &storage.BlockWithDependentBlocks{Block: block},
				versionId: version.versionId,
				user:      version.user,
				date:      version.date,
			}}
		}
	}
	return m
}

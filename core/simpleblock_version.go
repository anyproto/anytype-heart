package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
)

type SimpleBlockVersion struct {
	model                   *model.Block
	parentSmartBlockVersion BlockVersion
	node                    *Anytype
}

func (version *SimpleBlockVersion) Model() *model.Block {
	return version.model
}

func (version *SimpleBlockVersion) VersionId() string {
	return version.parentSmartBlockVersion.VersionId()
}

func (version *SimpleBlockVersion) User() string {
	return version.parentSmartBlockVersion.User()
}

func (version *SimpleBlockVersion) Date() *types.Timestamp {
	return version.parentSmartBlockVersion.Date()
}

func (version *SimpleBlockVersion) ExternalFields() *types.Struct {
	// simple blocks can't have fields
	return nil
}

func (version *SimpleBlockVersion) DependentBlocks() map[string]BlockVersion {
	// simple blocks don't store dependent blocks
	return nil
}

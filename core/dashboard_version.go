package core

import (
	"github.com/gogo/protobuf/types"
)

type DashboardVersion struct {
	*SmartBlockVersion
}

func (version *DashboardVersion) ExternalFields() *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		"name": version.Model().Fields.Fields["name"],
		"icon": version.Model().Fields.Fields["icon"],
	}}
}

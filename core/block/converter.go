package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

func versionToModel(ver anytype.BlockVersion) *model.Block {
	return &model.Block{
		Id:          ver.GetBlockId(),
		Fields:      ver.GetFields(),
		Permissions: ver.GetPermissions(),
		ChildrenIds: ver.GetChildrenIds(),
		Content:     ver.GetContent(),
	}
}

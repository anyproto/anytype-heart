package pb

import (
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
)

type CollectionProvider interface {
	ProvideCollection(snapshots []*common.Snapshot,
		widget *common.Snapshot,
		oldToNewID map[string]string,
		params *pb.RpcObjectImportRequestPbParams,
		workspaceSnapshot *common.Snapshot,
		isNewSpace bool,
	) ([]*common.Snapshot, string, error)
}

func GetProvider(importType pb.RpcObjectImportRequestPbParamsType, service *collection.Service) CollectionProvider {
	if importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE {
		return NewGalleryImport(service)
	}
	return NewSpaceImport(service)
}

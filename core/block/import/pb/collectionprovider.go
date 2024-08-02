package pb

import (
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common/types"
	"github.com/anyproto/anytype-heart/pb"
)

type CollectionProvider interface {
	ProvideCollection(snapshots []*types.Snapshot, widget *types.Snapshot, oldToNewID map[string]string, params *pb.RpcObjectImportRequestPbParams, workspaceSnapshot *types.Snapshot, isNewSpace bool) ([]*types.Snapshot, error)
}

func GetProvider(importType pb.RpcObjectImportRequestPbParamsType, service *collection.Service) CollectionProvider {
	if importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE {
		return NewGalleryImport(service)
	}
	return NewSpaceImport(service)
}

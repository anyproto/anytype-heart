package pb

import (
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
)

type CollectionProvider interface {
	ProvideCollection(
		snapshots *common.SnapshotList,
		oldToNewID map[string]string,
		params *pb.RpcObjectImportRequestPbParams,
		isNewSpace bool,
	) ([]*common.Snapshot, error)
}

func GetProvider(importType pb.RpcObjectImportRequestPbParamsType, service *collection.Service) CollectionProvider {
	if importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE {
		return NewGalleryImport(service)
	}
	return NewSpaceImport(service)
}

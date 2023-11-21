package pb

import (
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pb"
)

type CollectionProvider interface {
	ProvideCollection(snapshots []*converter.Snapshot,
		widget *converter.Snapshot,
		oldToNewID map[string]string,
		params *pb.RpcObjectImportRequestPbParams,
		workspaceSnapshot *converter.Snapshot,
	) (*converter.Snapshot, error)
}

func GetProvider(importType pb.RpcObjectImportRequestPbParamsType, service *collection.Service) CollectionProvider {
	if importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE {
		return NewGalleryImport(service)
	}
	return NewSpaceImport(service)
}

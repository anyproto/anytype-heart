package pb

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

type SnapshotProvider interface {
	ProvideSnapshots(progress process.Progress, req *pb.RpcObjectImportRequest, allErrors *converter.ConvertError) ([]*converter.Snapshot, *converter.Snapshot, *converter.Snapshot)
	ProvideCollection(snapshots []*converter.Snapshot, widget *converter.Snapshot, oldToNewID map[string]string, params *pb.RpcObjectImportRequestPbParams, workspaceSnapshot *converter.Snapshot) (*converter.Snapshot, error)
}

func GetProvider(importType pb.RpcObjectImportRequestPbParamsType, service *collection.Service, accountService account.Service) SnapshotProvider {
	if importType == pb.RpcObjectImportRequestPbParams_EXPERIENCE {
		return NewGalleryImport(service)
	}
	return NewSpaceImport(accountService, service)
}
func GetParams(params pb.IsRpcObjectImportRequestParams) (*pb.RpcObjectImportRequestPbParams, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfPbParams); ok {
		return p.PbParams, nil
	}
	return nil, fmt.Errorf("PB: getParams wrong parameters format")
}

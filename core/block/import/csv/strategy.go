package csv

import (
	"github.com/anyproto/anytype-heart/core/block/import/common/types"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

type Strategy interface {
	CreateObjects(path string, csvTable [][]string, params *pb.RpcObjectImportRequestCsvParams, progress process.Progress) (string, []*types.Snapshot, error)
}

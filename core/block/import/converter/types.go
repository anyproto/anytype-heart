package converter

import (
	"context"
	"io"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type ObjectTreeCreator interface {
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, release func(), err error)
}

// Converter incapsulate logic with transforming some data to smart blocks
type Converter interface {
	GetSnapshots(req *pb.RpcObjectImportRequest) *Response
	Name() string
}

// ImageGetter returns image for given converter in frontend
type ImageGetter interface {
	GetImage() ([]byte, int64, int64, error)
}

// IOReader combine name of the file and it's io reader
type IOReader struct {
	Name   string
	Reader io.ReadCloser
}
type Snapshot struct {
	Id       string
	SbType   coresb.SmartBlockType
	FileName string
	Snapshot *model.SmartBlockSnapshotBase
}

// Response expected response of each converter, incapsulate blocks snapshots and converting errors
type Response struct {
	Snapshots []*Snapshot
	Error     ConvertError
}

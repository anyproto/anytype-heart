package converter

import (
	"context"
	"io"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ObjectTreeCreator interface {
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, release func(), err error)
}

// Converter incapsulate logic with transforming some data to smart blocks
type Converter interface {
	GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*Response, ConvertError)
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
	Snapshot *pb.ChangeSnapshot
}

// Relation are stored during GetSnapshots step in converter and create them in RelationCreator
type Relation struct {
	BlockID string // if relations is used as a block
	*model.Relation
}

// Response expected response of each converter, incapsulate blocks snapshots and converting errors
type Response struct {
	Snapshots []*Snapshot
	Relations map[string][]*Relation // object id to its relations
	Error     ConvertError
}

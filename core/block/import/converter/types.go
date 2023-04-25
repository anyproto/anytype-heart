package converter

import (
	"context"
	"io"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// Functions to create in-tree and plugin converters
var converterCreators []Creator

// Creator function to register converter
type Creator = func(s core.Service, col *collection.Service) Converter

// RegisterFunc add converter creation function to converterCreators
func RegisterFunc(c Creator) {
	converterCreators = append(converterCreators, c)
}

type ObjectTreeCreator interface {
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, release func(), err error)
}

// Converter incapsulate logic with transforming some data to smart blocks
type Converter interface {
	GetSnapshots(req *pb.RpcObjectImportRequest, progress *process.Progress) (*Response, ConvertError)
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

func GetConverters() []func(s core.Service, service *collection.Service) Converter {
	return converterCreators
}

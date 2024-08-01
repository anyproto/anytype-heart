package common

import (
	"context"
	"io"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ObjectTreeCreator interface {
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc smartblock.InitFunc) (sb smartblock.SmartBlock, release func(), err error)
}

// Converter incapsulate logic with transforming some data to smart blocks
type Converter interface {
	GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*Response, *ConvertError)
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

// TODO Add spaceID?
type Snapshot struct {
	Id       string
	FileName string
	Snapshot *SnapshotModel
}

type SnapshotModel struct {
	SbType   coresb.SmartBlockType
	LogHeads map[string]string
	Data     *SnapshotModelData
	FileKeys []*pb.ChangeFileKeys
}

type SnapshotModelData struct {
	Blocks                   []*model.Block
	Details                  *domain.Details
	FileKeys                 *types.Struct
	ExtraRelations           []*model.Relation
	ObjectTypes              []string
	Collections              *types.Struct
	RemovedCollectionKeys    []string
	RelationLinks            []*model.RelationLink
	Key                      string
	OriginalCreatedTimestamp int64
	FileInfo                 *model.FileInfo
}

// Response expected response of each converter, incapsulate blocks snapshots and converting errors
type Response struct {
	Snapshots        []*Snapshot
	RootCollectionID string
}

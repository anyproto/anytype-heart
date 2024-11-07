package common

import (
	"context"
	"fmt"
	"io"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	Data     *StateSnapshot
	FileKeys []*pb.ChangeFileKeys
}

func (sn *SnapshotModel) ToProto() *pb.ChangeSnapshot {
	return &pb.ChangeSnapshot{
		Data:     sn.Data.ToProto(),
		LogHeads: sn.LogHeads,
		FileKeys: sn.FileKeys,
	}
}

type StateSnapshot struct {
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

func (sn *StateSnapshot) ToProto() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks:                   sn.Blocks,
		Details:                  sn.Details.ToProto(),
		FileKeys:                 sn.FileKeys,
		ExtraRelations:           sn.ExtraRelations,
		ObjectTypes:              sn.ObjectTypes,
		Collections:              sn.Collections,
		RemovedCollectionKeys:    sn.RemovedCollectionKeys,
		RelationLinks:            sn.RelationLinks,
		Key:                      sn.Key,
		OriginalCreatedTimestamp: sn.OriginalCreatedTimestamp,
		FileInfo:                 sn.FileInfo,
	}
}

func NewStateSnapshotFromProto(sn *model.SmartBlockSnapshotBase) *StateSnapshot {
	return &StateSnapshot{
		Blocks:                   sn.Blocks,
		Details:                  domain.NewDetailsFromProto(sn.Details),
		FileKeys:                 sn.FileKeys,
		ExtraRelations:           sn.ExtraRelations,
		ObjectTypes:              sn.ObjectTypes,
		Collections:              sn.Collections,
		RemovedCollectionKeys:    sn.RemovedCollectionKeys,
		RelationLinks:            sn.RelationLinks,
		Key:                      sn.Key,
		OriginalCreatedTimestamp: sn.OriginalCreatedTimestamp,
		FileInfo:                 sn.FileInfo,
	}
}

// Adds missing unique key for supported smartblock types
func migrateAddMissingUniqueKey(snapshot *SnapshotModel) {
	id := snapshot.Data.Details.GetString(bundle.RelationKeyId)
	uk, err := domain.UnmarshalUniqueKey(id)
	if err != nil {
		// Maybe it's a relation option?
		if bson.IsObjectIdHex(id) {
			uk = domain.MustUniqueKey(coresb.SmartBlockTypeRelationOption, id)
		} else {
			// Means that smartblock type is not supported
			return
		}
	}
	snapshot.Data.Key = uk.InternalKey()
}

func NewSnapshotModelFromProto(sn *pb.SnapshotWithType) (*SnapshotModel, error) {
	if sn == nil || sn.Snapshot == nil || sn.Snapshot.Data == nil {
		return nil, fmt.Errorf("snapshot is not valid")
	}
	res := &SnapshotModel{
		SbType:   coresb.SmartBlockType(sn.SbType),
		LogHeads: sn.Snapshot.LogHeads,
		Data:     NewStateSnapshotFromProto(sn.Snapshot.Data),
		FileKeys: sn.Snapshot.FileKeys,
	}
	migrateAddMissingUniqueKey(res)
	return res, nil
}

// Response expected response of each converter, incapsulate blocks snapshots and converting errors
type Response struct {
	Snapshots        []*Snapshot
	RootCollectionID string
}

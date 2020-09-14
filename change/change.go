package change

import (
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

func NewChangeFromRecord(record core.SmartblockRecordWithLogID) (*Change, error) {
	var ch = &pb.Change{}
	if err := record.Unmarshal(ch); err != nil {
		return nil, err
	}
	return &Change{
		Id:     record.ID,
		Change: ch,
	}, nil
}

type Change struct {
	Id          string
	Next        []*Change
	detailsOnly bool
	*pb.Change
}

func (ch *Change) GetLastSnapshotId() string {
	if ch.Snapshot != nil {
		return ch.Id
	}
	return ch.LastSnapshotId
}

func (ch *Change) HasDetails() bool {
	if ch.Snapshot != nil {
		return true
	}
	for _, ct := range ch.Content {
		if ct.GetDetailsSet() != nil {
			return true
		}
		if ct.GetDetailsUnset() != nil {
			return true
		}
	}
	return false
}

func NewSnapshotChange(blocks []*model.Block, details *types.Struct, fileKeys []*core.FileKeys) proto.Marshaler {
	fkeys := make([]*pb.ChangeFileKeys, len(fileKeys))
	for i, k := range fileKeys {
		fkeys[i] = &pb.ChangeFileKeys{
			Hash: k.Hash,
			Keys: k.Keys,
		}
	}
	return &pb.Change{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks:  blocks,
				Details: details,
			},
			FileKeys: fkeys,
		},
		Timestamp: time.Now().Unix(),
	}
}

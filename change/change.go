package change

import (
	"encoding/json"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

func NewChangeFromRecord(record core.SmartblockRecordEnvelope) (*Change, error) {
	var ch = &pb.Change{}
	if err := record.Unmarshal(ch); err != nil {
		return nil, err
	}
	return &Change{
		Id:      record.ID,
		Account: record.AccountID,
		Device:  record.LogID,
		Change:  ch,
	}, nil
}

type Change struct {
	Id          string
	Account     string
	Device      string
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

func (ch *Change) HasMeta() bool {
	if ch.Snapshot != nil {
		return true
	}
	for _, ct := range ch.Content {
		switch ct.Value.(type) {
		case *pb.ChangeContentValueOfDetailsSet:
			return true
		case *pb.ChangeContentValueOfDetailsUnset:
			return true
		case *pb.ChangeContentValueOfRelationAdd:
			return true
		case *pb.ChangeContentValueOfRelationRemove:
			return true
		case *pb.ChangeContentValueOfRelationUpdate:
			return true
		case *pb.ChangeContentValueOfObjectTypeAdd:
			return true
		case *pb.ChangeContentValueOfObjectTypeRemove:
			return true
		case *pb.ChangeContentValueOfBlockUpdate:
			// todo: find a better solution to store dataview relations
			for _, ev := range ct.Value.(*pb.ChangeContentValueOfBlockUpdate).BlockUpdate.Events {
				switch ev.Value.(type) {
				case *pb.EventMessageValueOfBlockDataviewRelationSet:
					return true
				case *pb.EventMessageValueOfBlockDataviewRelationDelete:
					return true
				}
			}
		}
	}
	return false
}

func NewSnapshotChange(blocks []*model.Block, details *types.Struct, relations []*model.Relation, objectTypes []string, fileKeys []*files.FileKeys) proto.Marshaler {
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
				Blocks:         blocks,
				Details:        details,
				ExtraRelations: relations,
				ObjectTypes:    objectTypes,
			},
			FileKeys: fkeys,
		},
		Timestamp: time.Now().Unix(),
	}
}

func (ch *Change) MarshalJSON() ([]byte, error) {
	pbjson := ""
	if ch.Change != nil {
		var err error
		ml := &jsonpb.Marshaler{}
		pbjson, err = ml.MarshalToString(ch.Change)
		if err != nil {
			return nil, err
		}
	}
	var data = map[string]string{
		"Id":      ch.Id,
		"Account": ch.Account,
		"Device":  ch.Device,
		"Change":  pbjson,
	}
	return json.Marshal(data)
}

func (ch *Change) UnmarshalJSON(data []byte) (err error) {
	var dataMap = make(map[string]string)
	if err = json.Unmarshal(data, &dataMap); err != nil {
		return
	}
	if chs, ok := dataMap["Change"]; ok {
		ch.Change = &pb.Change{}
		if err = jsonpb.UnmarshalString(chs, ch.Change); err != nil {
			return
		}
	}
	ch.Id = dataMap["Id"]
	ch.Account = dataMap["Account"]
	ch.Device = dataMap["Device"]
	return
}

package objects

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/gogo/protobuf/types"
)

const (
	CustomObjectTypeURLPrefix  = "https://anytype.io/schemas/object/custom/"
	BundledObjectTypeURLPrefix = "https://anytype.io/schemas/object/bundled/"
)

func New(
	pageStore localstore.ObjectStore,
	objectTypeUrl string,
	setDetails func(req pb.RpcBlockSetDetailsRequest) error,
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct) (pageId string, err error),
) database.Database {
	return &setOfObjects{
		ObjectStore:      pageStore,
		objectTypeUrl:    objectTypeUrl,
		setDetails:       setDetails,
		createSmartBlock: createSmartBlock,
	}
}

type setOfObjects struct {
	localstore.ObjectStore
	objectTypeUrl    string
	setDetails       func(req pb.RpcBlockSetDetailsRequest) error
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct) (pageId string, err error)
}

func (sp setOfObjects) Create(rec database.Record) (database.Record, error) {
	id, err := sp.createSmartBlock(coresb.SmartBlockTypePage, rec.Details)
	if err != nil {
		return rec, err
	}

	if rec.Details == nil || rec.Details.Fields == nil {
		rec.Details = &types.Struct{Fields: make(map[string]*types.Value)}
	}

	// inject created block ID into the record
	rec.Details.Fields[database.RecordIDField] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}
	return rec, nil
}

func (sp *setOfObjects) Update(id string, rec database.Record) error {
	var details []*pb.RpcBlockSetDetailsDetail
	for k, v := range rec.Details.Fields {
		details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
	}

	if len(details) == 0 {
		return nil
	}

	return sp.setDetails(pb.RpcBlockSetDetailsRequest{
		ContextId: id, // not sure?
		Details:   details,
	})
}

func (sp setOfObjects) Delete(id string) error {

	// TODO implement!

	return errors.New("not implemented")
}

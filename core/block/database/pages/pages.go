package pages

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/gogo/protobuf/types"
)

func New(
	pageStore localstore.PageStore,
	setDetails func(req pb.RpcBlockSetDetailsRequest) error,
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct) (pageId string, err error),
) database.Database {
	return &setPages{
		PageStore:        pageStore,
		setDetails:       setDetails,
		createSmartBlock: createSmartBlock,
	}
}

type setPages struct {
	localstore.PageStore
	setDetails       func(req pb.RpcBlockSetDetailsRequest) error
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct) (pageId string, err error)
}

func (sp setPages) Create(rec database.Record) (database.Record, error) {
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

func (sp *setPages) Update(id string, rec database.Record) error {
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

func (sp setPages) Delete(id string) error {

	// TODO implement!

	return errors.New("not implemented")
}

func (sp setPages) Schema() string {
	return sp.Schema()
}

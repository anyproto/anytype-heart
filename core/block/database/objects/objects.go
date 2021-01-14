package objects

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
	getRelations func(objectId string) (relations []*pbrelation.Relation, err error),
	setRelations func(id string, relations []*pbrelation.Relation) (err error),

	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, err error),
) database.Database {
	return &setOfObjects{
		ObjectStore:      pageStore,
		objectTypeUrl:    objectTypeUrl,
		setDetails:       setDetails,
		getRelations:     getRelations,
		setRelations:     setRelations,
		createSmartBlock: createSmartBlock,
	}
}

type setOfObjects struct {
	localstore.ObjectStore
	objectTypeUrl string
	setDetails    func(req pb.RpcBlockSetDetailsRequest) error
	getRelations  func(objectId string) (relations []*pbrelation.Relation, err error)
	setRelations  func(id string, relations []*pbrelation.Relation) (err error)

	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, err error)
}

func (sp setOfObjects) Create(relations []*pbrelation.Relation, rec database.Record, sub database.Subscription) (database.Record, error) {
	if rec.Details == nil || rec.Details.Fields == nil {
		rec.Details = &types.Struct{Fields: make(map[string]*types.Value)}
	}

	rec.Details.Fields["type"] = pbtypes.StringList([]string{sp.objectTypeUrl})
	id, err := sp.createSmartBlock(coresb.SmartBlockTypePage, rec.Details, nil)
	if err != nil {
		return rec, err
	}

	if sub != nil {
		sub.Subscribe([]string{id})
	}
	err = sp.setRelations(id, relations)
	if err != nil {
		return rec, err
	}

	rec.Details.Fields["type"] = pbtypes.StringList([]string{sp.objectTypeUrl})

	var details []*pb.RpcBlockSetDetailsDetail
	for k, v := range rec.Details.Fields {
		details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
	}

	rec.Details.Fields[database.RecordIDField] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}

	if len(details) == 0 {
		return rec, nil
	}

	return rec, sp.setDetails(pb.RpcBlockSetDetailsRequest{
		ContextId: id, // not sure?
		Details:   details,
	})
}

func (sp *setOfObjects) Update(id string, rels []*pbrelation.Relation, rec database.Record) error {
	var details []*pb.RpcBlockSetDetailsDetail
	for k, v := range rec.Details.Fields {
		details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
	}

	if len(details) == 0 {
		return nil
	}

	err := sp.setRelations(id, rels)
	if err != nil {
		return err
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

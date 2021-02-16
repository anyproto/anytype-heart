package objects

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/google/martian/log"
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
	modifyExtraRelations func(id string, modifier func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error)) error,
	addExtraRelationOption func(req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error),
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, newDetails *types.Struct, err error),
) database.Database {
	return &setOfObjects{
		ObjectStore:            pageStore,
		objectTypeUrl:          objectTypeUrl,
		setDetails:             setDetails,
		getRelations:           getRelations,
		setRelations:           setRelations,
		createSmartBlock:       createSmartBlock,
		modifyExtraRelations:   modifyExtraRelations,
		addExtraRelationOption: addExtraRelationOption,
	}
}

type setOfObjects struct {
	localstore.ObjectStore
	objectTypeUrl          string
	setDetails             func(req pb.RpcBlockSetDetailsRequest) error
	getRelations           func(objectId string) (relations []*pbrelation.Relation, err error)
	setRelations           func(id string, relations []*pbrelation.Relation) (err error)
	modifyExtraRelations   func(id string, modifier func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error)) error
	addExtraRelationOption func(req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error)
	createSmartBlock       func(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, newDetails *types.Struct, err error)
}

func (sp setOfObjects) Create(relations []*pbrelation.Relation, rec database.Record, sub database.Subscription) (database.Record, error) {
	if rec.Details == nil || rec.Details.Fields == nil {
		rec.Details = &types.Struct{Fields: make(map[string]*types.Value)}
	}

	rec.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.StringList([]string{sp.objectTypeUrl})
	id, newDetails, err := sp.createSmartBlock(coresb.SmartBlockTypePage, rec.Details, nil)
	if err != nil {
		return rec, err
	}

	if newDetails == nil || newDetails.Fields == nil {
		log.Errorf("createSmartBlock returns an empty details for %s", id)
		newDetails = &types.Struct{Fields: map[string]*types.Value{}}
	}
	rec.Details = newDetails
	rec.Details.Fields[bundle.RelationKeyId.String()] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}

	if sub != nil {
		sub.Subscribe([]string{id})
	}

	var relsToSet []*pbrelation.Relation
	for _, rel := range relations {
		if pbtypes.HasField(rec.Details, rel.Key) {
			relsToSet = append(relsToSet, rel)
		}
	}

	err = sp.setRelations(id, relsToSet)
	if err != nil {
		return rec, err
	}

	return rec, nil
}

func (sp *setOfObjects) Update(id string, rels []*pbrelation.Relation, rec database.Record) error {
	var details []*pb.RpcBlockSetDetailsDetail
	if rec.Details != nil && rec.Details.Fields != nil {
		for k, v := range rec.Details.Fields {
			details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
		}
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

func (sp *setOfObjects) AddRelationOption(id string, relationKey string, option pbrelation.RelationOption) (optionId string, err error) {
	o, err := sp.addExtraRelationOption(pb.RpcObjectRelationOptionAddRequest{
		ContextId:   id,
		RelationKey: relationKey,
		Option:      &option,
	})
	if err != nil {
		return "", err
	}
	return o.Id, nil
}

func (sp setOfObjects) Delete(id string) error {

	// TODO implement!

	return errors.New("not implemented")
}

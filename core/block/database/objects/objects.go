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

	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error),
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

	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error)
}

func (sp setOfObjects) Create(relations []*pbrelation.Relation, rec database.Record, sub database.Subscription) (database.Record, error) {
	id, err := sp.createSmartBlock(coresb.SmartBlockTypePage, rec.Details, []string{sp.objectTypeUrl}, nil)
	if err != nil {
		return rec, err
	}

	if rec.Details == nil || rec.Details.Fields == nil {
		rec.Details = &types.Struct{Fields: make(map[string]*types.Value)}
	}

	if sub != nil {
		sub.Subscribe(id)
	}

	err = sp.UpdateObject(id, rec.Details, &pbrelation.Relations{Relations: relations}, nil, "")
	if err != nil {
		return rec, err
	}

	// inject created block ID into the record
	rec.Details.Fields[database.RecordIDField] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}
	return rec, nil
}

func (sp *setOfObjects) Update(id string, relations []*pbrelation.Relation, rec database.Record) error {
	var details []*pb.RpcBlockSetDetailsDetail
	for k, v := range rec.Details.Fields {
		details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
	}

	if len(details) == 0 {
		return nil
	}

	existingRelations, err := sp.getRelations(id)
	if err != nil {
		return err
	}

	var existingRelationMap = make(map[string]*pbrelation.Relation, len(existingRelations))
	for i, relation := range existingRelations {
		existingRelationMap[relation.Key] = existingRelations[i]
	}

	var relationsToSet []*pbrelation.Relation
	for i, relation := range relations {
		if _, detailSet := rec.Details.Fields[relation.Key]; detailSet {
			if rel, exists := existingRelationMap[relation.Key]; !exists {
				relationsToSet = append(relationsToSet, relations[i])
			} else if !pbtypes.RelationEqual(rel, relation) {
				// todo: check if this relation is extra?
				relationsToSet = append(relationsToSet, relations[i])
			}
		}
	}

	err = sp.setRelations(id, relationsToSet)
	return sp.setDetails(pb.RpcBlockSetDetailsRequest{
		ContextId: id, // not sure?
		Details:   details,
	})
}

func (sp setOfObjects) Delete(id string) error {

	// TODO implement!

	return errors.New("not implemented")
}

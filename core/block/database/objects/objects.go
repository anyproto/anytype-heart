package objects

import (
	"errors"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

const (
	//CustomObjectTypeURLPrefix  = "https://anytype.io/schemas/object/custom/"
	BundledObjectTypeURLPrefix = "_ot"
	defaultObjectType          = bundle.TypeKeyPage
)

var log = logging.Logger("anytype-core-db")

func New(
	pageStore objectstore.ObjectStore,
	objectType *model.ObjectType,
	setDetails func(req pb.RpcBlockSetDetailsRequest) error,
	getRelations func(objectId string) (relations []*model.Relation, err error),
	setRelations func(id string, relations []*model.Relation) (err error),
	modifyExtraRelations func(id string, modifier func(current []*model.Relation) ([]*model.Relation, error)) error,
	updateExtraRelationOption func(req pb.RpcObjectRelationOptionUpdateRequest) (opt *model.RelationOption, err error),
	deleteRelationOption func(id string, relationKey string, optionId string) error,
	createSmartBlock func(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error),
) database.Database {
	return &setOfObjects{
		ObjectStore:               pageStore,
		objectType:                objectType,
		setDetails:                setDetails,
		getRelations:              getRelations,
		setRelations:              setRelations,
		createSmartBlock:          createSmartBlock,
		modifyExtraRelations:      modifyExtraRelations,
		deleteExtraRelationOption: deleteRelationOption,
		updateExtraRelationOption: updateExtraRelationOption,
	}
}

type setOfObjects struct {
	objectstore.ObjectStore
	objectType                *model.ObjectType
	setDetails                func(req pb.RpcBlockSetDetailsRequest) error
	getRelations              func(objectId string) (relations []*model.Relation, err error)
	setRelations              func(id string, relations []*model.Relation) (err error)
	modifyExtraRelations      func(id string, modifier func(current []*model.Relation) ([]*model.Relation, error)) error
	deleteExtraRelationOption func(id string, relationKey string, optionId string) error
	updateExtraRelationOption func(req pb.RpcObjectRelationOptionUpdateRequest) (opt *model.RelationOption, err error)
	createSmartBlock          func(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error)
}

func (sp setOfObjects) Create(relations []*model.Relation, rec database.Record, sub database.Subscription, templateId string) (database.Record, error) {
	if rec.Details == nil || rec.Details.Fields == nil {
		rec.Details = &types.Struct{Fields: make(map[string]*types.Value)}
	}

	var relsToSet []*model.Relation
	for _, rel := range relations {
		if pbtypes.HasField(rec.Details, rel.Key) {
			relsToSet = append(relsToSet, rel)
		} else {
			// we should not explicitly pass relation if set is created by the object type and this object type contains the relation
			if sp.objectType == nil || !pbtypes.HasRelation(sp.objectType.Relations, rel.Key) {
				relsToSet = append(relsToSet, rel)
				// set detail to null so it will return in the subscription and the filter will work
				rec.Details.Fields[rel.Key] = pbtypes.Null()
			}
		}
	}

	// todo: remove this? As we can only create SmartBlockTypePage via sets now
	var sbType = coresb.SmartBlockTypePage
	for sbT, objType := range bundle.DefaultObjectTypePerSmartblockType {
		if sp.objectType != nil && objType.URL() == sp.objectType.Url {
			sbType = sbT
			break
		}
	}

	if targetType := pbtypes.GetString(rec.Details, bundle.RelationKeyTargetObjectType.String()); targetType != "" {
		var types []string
		if sp.objectType != nil {
			types = append(types, sp.objectType.Url)
		} else {
			types = append(types, defaultObjectType.URL())
		}
		types = append(types, targetType)
		rec.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.StringList(types)
	} else if sp.objectType == nil {
		if ot := pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()); ot == "" {
			rec.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(defaultObjectType.URL())
		}
	} else {
		rec.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(sp.objectType.Url)
	}

	id, newDetails, err := sp.createSmartBlock(sbType, rec.Details, relsToSet, templateId)
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

	return rec, nil
}

func (sp *setOfObjects) Update(id string, rels []*model.Relation, rec database.Record) error {
	var details []*pb.RpcBlockSetDetailsDetail
	if rec.Details != nil && rec.Details.Fields != nil {
		for k, v := range rec.Details.Fields {
			if _, ok := v.Kind.(*types.Value_NullValue); ok {
				v = nil
			}

			details = append(details, &pb.RpcBlockSetDetailsDetail{Key: k, Value: v})
		}
	}

	err := sp.setRelations(id, rels)
	if err != nil {
		return err
	}

	if len(details) == 0 {
		return nil
	}

	return sp.setDetails(pb.RpcBlockSetDetailsRequest{
		ContextId: id, // not sure?
		Details:   details,
	})
}

func (sp *setOfObjects) ModifyExtraRelations(id string, modifier func(current []*model.Relation) ([]*model.Relation, error)) error {
	return sp.modifyExtraRelations(id, modifier)
}

func (sp *setOfObjects) DeleteRelationOption(id string, relKey string, optionId string) error {
	return sp.deleteExtraRelationOption(id, relKey, optionId)
}

func (sp *setOfObjects) UpdateRelationOption(id string, relationKey string, option model.RelationOption) (optionId string, err error) {
	o, err := sp.updateExtraRelationOption(pb.RpcObjectRelationOptionUpdateRequest{
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

package core

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

const (
	customObjectTypeURLPrefix  = "https://anytype.io/schemas/object/custom/"
	bundledObjectTypeURLPrefix = "https://anytype.io/schemas/object/bundled/"
)

func (mw *Middleware) ObjectTypeRelationList(req *pb.RpcObjectTypeRelationListRequest) *pb.RpcObjectTypeRelationListResponse {
	response := func(code pb.RpcObjectTypeRelationListResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcObjectTypeRelationListResponse {
		m := &pb.RpcObjectTypeRelationListResponse{Relations: relations, Error: &pb.RpcObjectTypeRelationListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	objType, err := mw.getObjectType(req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
		}
		return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_ERROR, nil, err)
	}

	// todo: AppendRelationsFromOtherTypes case
	return response(pb.RpcObjectTypeRelationListResponseError_NULL, objType.Relations, nil)
}

func (mw *Middleware) ObjectTypeRelationAdd(req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	response := func(code pb.RpcObjectTypeRelationAddResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcObjectTypeRelationAddResponse {
		m := &pb.RpcObjectTypeRelationAddResponse{Relations: relations, Error: &pb.RpcObjectTypeRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	objType, err := mw.getObjectType(req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
		}

		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, nil, err)
	}

	if strings.HasPrefix(objType.Url, objects.BundledObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE, nil, fmt.Errorf("can't modify bundled object type"))
	}

	var relations []*pbrelation.Relation
	id := strings.TrimPrefix(objType.Url, objects.CustomObjectTypeURLPrefix)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		relations, err = bs.AddRelations(id, req.Relations)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcObjectTypeRelationAddResponseError_NULL, relations, nil)
}

func (mw *Middleware) ObjectTypeRelationUpdate(req *pb.RpcObjectTypeRelationUpdateRequest) *pb.RpcObjectTypeRelationUpdateResponse {
	response := func(code pb.RpcObjectTypeRelationUpdateResponseErrorCode, err error) *pb.RpcObjectTypeRelationUpdateResponse {
		m := &pb.RpcObjectTypeRelationUpdateResponse{Error: &pb.RpcObjectTypeRelationUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	objType, err := mw.getObjectType(req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationUpdateResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
		}

		return response(pb.RpcObjectTypeRelationUpdateResponseError_UNKNOWN_ERROR, err)
	}

	if strings.HasPrefix(objType.Url, objects.BundledObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationUpdateResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	id := strings.TrimPrefix(objType.Url, objects.CustomObjectTypeURLPrefix)
	err = mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.UpdateRelations(id, []*pbrelation.Relation{req.Relation})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationUpdateResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationUpdateResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeRelationRemove(req *pb.RpcObjectTypeRelationRemoveRequest) *pb.RpcObjectTypeRelationRemoveResponse {
	response := func(code pb.RpcObjectTypeRelationRemoveResponseErrorCode, err error) *pb.RpcObjectTypeRelationRemoveResponse {
		m := &pb.RpcObjectTypeRelationRemoveResponse{Error: &pb.RpcObjectTypeRelationRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	objType, err := mw.getObjectType(req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
		}

		return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}

	if strings.HasPrefix(objType.Url, objects.BundledObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}
	id := strings.TrimPrefix(objType.Url, objects.CustomObjectTypeURLPrefix)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.RemoveRelations(id, []string{req.RelationKey})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationRemoveResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeCreate(req *pb.RpcObjectTypeCreateRequest) *pb.RpcObjectTypeCreateResponse {
	response := func(code pb.RpcObjectTypeCreateResponseErrorCode, otype *pbrelation.ObjectType, err error) *pb.RpcObjectTypeCreateResponse {
		m := &pb.RpcObjectTypeCreateResponse{ObjectType: otype, Error: &pb.RpcObjectTypeCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var sbId string
	var relations []*pbrelation.Relation
	var requiredRelationByKey = make(map[string]*pbrelation.Relation, len(relation.RequiredInternalRelations))

	for _, rel := range relation.RequiredInternalRelations {
		requiredRelationByKey[rel] = relation.BundledRelations[rel]
	}

	for _, rel := range req.ObjectType.Relations {
		if rel.Key == "" {
			continue
		}
		if v, exists := requiredRelationByKey[rel.Key]; exists {
			if !pbtypes.RelationEqual(v, rel) {
				return response(pb.RpcObjectTypeCreateResponseError_BAD_INPUT, nil, fmt.Errorf("required relation %s not equals the bundled one", rel.Key))
			}
			delete(requiredRelationByKey, rel.Key)
		}
	}

	for _, rel := range requiredRelationByKey {
		req.ObjectType.Relations = append(req.ObjectType.Relations, rel)
	}

	err := mw.doBlockService(func(bs block.Service) (err error) {
		sbId, err = bs.CreateSmartBlock(smartblock.SmartBlockTypeObjectType, &types.Struct{
			Fields: map[string]*types.Value{
				"name":      pbtypes.String(req.ObjectType.Name),
				"iconEmoji": pbtypes.String(req.ObjectType.IconEmoji),
				"layout":    pbtypes.Float64(float64(req.ObjectType.Layout)),
			},
		}, nil, nil)
		if err != nil {
			return err
		}

		relations, err = bs.AddRelations(sbId, req.ObjectType.Relations)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeCreateResponseError_UNKNOWN_ERROR, nil, err)
	}

	otype := req.ObjectType
	otype.Relations = relations
	otype.Url = customObjectTypeURLPrefix + sbId
	return response(pb.RpcObjectTypeCreateResponseError_NULL, otype, nil)
}

func (mw *Middleware) ObjectTypeList(_ *pb.RpcObjectTypeListRequest) *pb.RpcObjectTypeListResponse {
	response := func(code pb.RpcObjectTypeListResponseErrorCode, otypes []*pbrelation.ObjectType, err error) *pb.RpcObjectTypeListResponse {
		m := &pb.RpcObjectTypeListResponse{ObjectTypes: otypes, Error: &pb.RpcObjectTypeListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	otypes, err := relation.ListObjectTypes()
	if err != nil {
		return response(pb.RpcObjectTypeListResponseError_UNKNOWN_ERROR, nil, err)
	}

	threadIds, err := mw.Anytype.ThreadService().ListThreadIdsByType(smartblock.SmartBlockTypeObjectType)
	if err != nil {
		return response(pb.RpcObjectTypeListResponseError_UNKNOWN_ERROR, nil, err)
	}

	for _, id := range threadIds {
		err = mw.doBlockService(func(bs block.Service) (err error) {
			otype, err := bs.GetObjectType(objects.CustomObjectTypeURLPrefix + id.String())
			if err != nil {
				return err
			}
			otypes = append(otypes, otype)
			return nil
		})
	}

	return response(pb.RpcObjectTypeListResponseError_NULL, otypes, nil)
}

func (mw *Middleware) SetCreate(req *pb.RpcSetCreateRequest) *pb.RpcSetCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcSetCreateResponseErrorCode, id string, err error) *pb.RpcSetCreateResponse {
		m := &pb.RpcSetCreateResponse{Error: &pb.RpcSetCreateResponseError{Code: code}, Id: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		_, id, err = bs.CreateSet(ctx, pb.RpcBlockCreateSetRequest{ObjectTypeUrl: req.ObjectTypeUrl, Details: req.Details})
		return err
	})

	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcSetCreateResponseError_UNKNOWN_OBJECT_TYPE_URL, "", err)
		}
		return response(pb.RpcSetCreateResponseError_UNKNOWN_ERROR, "", err)
	}

	return response(pb.RpcSetCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) getObjectType(url string) (*pbrelation.ObjectType, error) {
	var objType = &pbrelation.ObjectType{}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		objType, err = bs.GetObjectType(url)
		if err != nil {
			return err
		}
		return nil
	})

	return objType, err
}

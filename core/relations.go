package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (mw *Middleware) ObjectTypeRelationList(cctx context.Context, req *pb.RpcObjectTypeRelationListRequest) *pb.RpcObjectTypeRelationListResponse {
	response := func(code pb.RpcObjectTypeRelationListResponseErrorCode, relations []*model.Relation, err error) *pb.RpcObjectTypeRelationListResponse {
		m := &pb.RpcObjectTypeRelationListResponse{Relations: relations, Error: &pb.RpcObjectTypeRelationListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationListResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	objType, err := mw.getObjectType(at, req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
		}
		return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_ERROR, nil, err)
	}

	// todo: AppendRelationsFromOtherTypes case
	return response(pb.RpcObjectTypeRelationListResponseError_NULL, objType.Relations, nil)
}

func (mw *Middleware) ObjectTypeRelationAdd(cctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	response := func(code pb.RpcObjectTypeRelationAddResponseErrorCode, err error) *pb.RpcObjectTypeRelationAddResponse {
		m := &pb.RpcObjectTypeRelationAddResponse{Error: &pb.RpcObjectTypeRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_BAD_INPUT, fmt.Errorf("account must be started"))
	}

	objType, err := mw.getObjectType(at, req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
		}

		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, err)
	}

	if strings.HasPrefix(objType.Url, bundle.TypePrefix) {
		return response(pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	err = mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.AddExtraRelations(nil, objType.Url, req.RelationIds)
		if err != nil {
			return err
		}
		// TODO:
		/*err = bs.ModifyDetails(objType.Url, func(current *types.Struct) (*types.Struct, error) {
			list := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())
			for _, rel := range relations {
				var relId string
				if bundle.HasRelation(rel.Key) {
					relId = addr.BundledRelationURLPrefix + rel.Key
				} else {
					relId = addr.CustomRelationURLPrefix + rel.Key
				}

				if slice.FindPos(list, relId) == -1 {
					list = append(list, relId)
				}
			}
			detCopy := pbtypes.CopyStruct(current)
			detCopy.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(list)
			return detCopy, nil
		})
		if err != nil {
			return err
		}
		*/
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeRelationRemove(cctx context.Context, req *pb.RpcObjectTypeRelationRemoveRequest) *pb.RpcObjectTypeRelationRemoveResponse {
	response := func(code pb.RpcObjectTypeRelationRemoveResponseErrorCode, err error) *pb.RpcObjectTypeRelationRemoveResponse {
		m := &pb.RpcObjectTypeRelationRemoveResponse{Error: &pb.RpcObjectTypeRelationRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_BAD_INPUT, fmt.Errorf("account must be started"))
	}

	objType, err := mw.getObjectType(at, req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
		}

		return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}

	if strings.HasPrefix(objType.Url, bundle.TypePrefix) {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	err = mw.doBlockService(func(bs block.Service) (err error) {
		// TODO:
		/*
			err = bs.ModifyDetails(objType.Url, func(current *types.Struct) (*types.Struct, error) {
				list := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())
				var relId string
				if bundle.HasRelation(req.RelationKey) {
					relId = addr.BundledRelationURLPrefix + req.RelationKey
				} else {
					relId = addr.CustomRelationURLPrefix + req.RelationKey
				}

				list = slice.Remove(list, relId)
				detCopy := pbtypes.CopyStruct(current)
				detCopy.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(list)
				return detCopy, nil
			})
			if err != nil {
				return err
			}
			err = bs.RemoveExtraRelations(nil, objType.Url, []string{req.RelationKey})
			if err != nil {
				return err
			}
			return nil

		*/
		return
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationRemoveResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeCreate(cctx context.Context, req *pb.RpcObjectTypeCreateRequest) *pb.RpcObjectTypeCreateResponse {
	response := func(code pb.RpcObjectTypeCreateResponseErrorCode, details *types.Struct, err error) *pb.RpcObjectTypeCreateResponse {
		m := &pb.RpcObjectTypeCreateResponse{NewDetails: details, Error: &pb.RpcObjectTypeCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	_, newDetails, err := mw.objectTypeCreate(req)
	if err != nil {
		return response(pb.RpcObjectTypeCreateResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcObjectTypeCreateResponseError_NULL, newDetails, nil)
}

func (mw *Middleware) objectTypeCreate(req *pb.RpcObjectTypeCreateRequest) (id string, newDetails *types.Struct, err error) {
	if req.Details == nil {
		req.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	req.Details = internalflag.AddToDetails(req.Details, req.InternalFlags)

	var sbId string
	var recommendedRelationKeys []string

	rawLayout := pbtypes.GetFloat64(req.Details, bundle.RelationKeyRecommendedLayout.String())
	layout, err := bundle.GetLayout(model.ObjectTypeLayout(rawLayout))
	if err != nil {
		return "", nil, fmt.Errorf("invalid layout: %w", err)
	}

	for _, rel := range bundle.RequiredInternalRelations {
		recommendedRelationKeys = append(recommendedRelationKeys, addr.BundledRelationURLPrefix+rel.String())
	}

	for _, rel := range layout.RequiredRelations {
		k := addr.BundledRelationURLPrefix + rel.Key
		if slice.FindPos(recommendedRelationKeys, k) != -1 {
			continue
		}
		recommendedRelationKeys = append(recommendedRelationKeys, k)
	}

	details := req.Details
	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyObjectType.URL())
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
	details.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationKeys)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		sbId, _, err = bs.CreateSmartBlock(context.TODO(), smartblock.SmartBlockTypeObjectType, details, nil) // TODO: add relationIds
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return
	}

	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(sbId)

	return sbId, details, nil
}

func (mw *Middleware) ObjectTypeList(cctx context.Context, _ *pb.RpcObjectTypeListRequest) *pb.RpcObjectTypeListResponse {
	response := func(code pb.RpcObjectTypeListResponseErrorCode, otypes []*model.ObjectType, err error) *pb.RpcObjectTypeListResponse {
		m := &pb.RpcObjectTypeListResponse{ObjectTypes: otypes, Error: &pb.RpcObjectTypeListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeListResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	var (
		ids    []string
		otypes []*model.ObjectType
	)
	for _, t := range []smartblock.SmartBlockType{smartblock.SmartBlockTypeObjectType, smartblock.SmartBlockTypeBundledObjectType} {
		st, err := mw.GetApp().MustComponent(source.CName).(source.Service).SourceTypeBySbType(t)
		if err != nil {
			return response(pb.RpcObjectTypeListResponseError_UNKNOWN_ERROR, nil, err)
		}
		idsT, err := st.ListIds()
		if err != nil {
			return response(pb.RpcObjectTypeListResponseError_UNKNOWN_ERROR, nil, err)
		}
		ids = append(ids, idsT...)
	}

	for _, id := range ids {
		otype, err := mw.getObjectType(at, id)
		if err != nil {
			log.Errorf("failed to get objectType %s info: %s", id, err.Error())
			continue
		}
		otypes = append(otypes, otype)
	}

	return response(pb.RpcObjectTypeListResponseError_NULL, otypes, nil)
}

func (mw *Middleware) ObjectCreateSet(cctx context.Context, req *pb.RpcObjectCreateSetRequest) *pb.RpcObjectCreateSetResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCreateSetResponseErrorCode, id string, err error) *pb.RpcObjectCreateSetResponse {
		m := &pb.RpcObjectCreateSetResponse{Error: &pb.RpcObjectCreateSetResponseError{Code: code}, Id: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	id, err := mw.objectCreateSet(req)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_OBJECT_TYPE_URL, "", err)
		}
		return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_ERROR, "", err)
	}

	return response(pb.RpcObjectCreateSetResponseError_NULL, id, nil)
}

func (mw *Middleware) objectCreateSet(req *pb.RpcObjectCreateSetRequest) (string, error) {
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		if req.GetDetails().GetFields() == nil {
			req.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}
		req.Details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList(req.Source)
		id, err = bs.CreateSet(*req)
		return err
	})
	return id, err
}

func (mw *Middleware) getObjectType(at core.Service, url string) (*model.ObjectType, error) {
	return objectstore.GetObjectType(at.ObjectStore(), url)
}

func (mw *Middleware) RelationCreate(cctx context.Context, req *pb.RpcRelationCreateRequest) *pb.RpcRelationCreateResponse {
	response := func(id, key string, err error) *pb.RpcRelationCreateResponse {
		if err != nil {
			return &pb.RpcRelationCreateResponse{
				Error: &pb.RpcRelationCreateResponseError{
					Code:        pb.RpcRelationCreateResponseError_UNKNOWN_ERROR,
					Description: err.Error(),
				},
			}
		}
		return &pb.RpcRelationCreateResponse{
			Error: &pb.RpcRelationCreateResponseError{
				Code: pb.RpcRelationCreateResponseError_NULL,
			},
			Id:  id,
			Key: key,
		}
	}
	rl, err := mw.relationCreate(req)
	if err != nil {
		return response("", "", err)
	}
	return response(rl.Id, rl.Key, err)
}

func (mw *Middleware) relationCreate(req *pb.RpcRelationCreateRequest) (*model.RelationLink, error) {
	var rl *model.RelationLink
	err := mw.doRelationService(func(rs relation.Service) error {
		var err error
		rl, err = rs.Create(req.Relation, req.Details)
		if err != nil {
			return err
		}
		return nil
	})
	return rl, err
}

func (mw *Middleware) RelationCreateOption(cctx context.Context, request *pb.RpcRelationCreateOptionRequest) *pb.RpcRelationCreateOptionResponse {
	response := func(id string, err error) *pb.RpcRelationCreateOptionResponse {
		if err != nil {
			return &pb.RpcRelationCreateOptionResponse{
				Error: &pb.RpcRelationCreateOptionResponseError{
					Code:        pb.RpcRelationCreateOptionResponseError_UNKNOWN_ERROR,
					Description: err.Error(),
				},
			}
		}
		return &pb.RpcRelationCreateOptionResponse{
			Error: &pb.RpcRelationCreateOptionResponseError{
				Code: pb.RpcRelationCreateOptionResponseError_NULL,
			},
			Id: id,
		}
	}

	var id string
	err := mw.doBlockService(func(rs block.Service) error {
		var err error
		id, err = rs.CreateRelationOption(request.RelationKey, (&relation.Option{request.Option}).ToStruct())
		return err
	})
	return response(id, err)
}

func (mw *Middleware) RelationListRemoveOption(cctx context.Context, request *pb.RpcRelationListRemoveOptionRequest) *pb.RpcRelationListRemoveOptionResponse {
	//TODO implement me
	panic("implement me")
}

func (mw *Middleware) RelationOptions(cctx context.Context, request *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse {
	//TODO implement me
	panic("implement me")
}

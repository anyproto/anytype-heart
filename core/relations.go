package core

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

	var relations []*pbrelation.Relation
	if strings.HasPrefix(req.ObjectTypeURL, bundledObjectTypeURLPrefix) {
		objType, err := relation.GetObjectType(req.ObjectTypeURL)
		if err != nil {
			if err == relation.ErrNotFound {
				return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
			}
			return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_ERROR, nil, err)
		}
		relations = objType.Relations
	} else if !strings.HasPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, fmt.Errorf("incorrect object type URL format"))
	}

	sbid := strings.TrimPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix)
	sb, err := mw.Anytype.GetBlock(sbid)
	if err != nil {
		return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
	}

	err = mw.doBlockService(func(bs block.Service) (err error) {
		otype, err := bs.GetObjectType(sb.ID())
		if err != nil {
			return err
		}
		relations = otype.Relations
		return nil
	})

	// todo: AppendRelationsFromOtherTypes case

	return response(pb.RpcObjectTypeRelationListResponseError_NULL, relations, nil)
}

func (mw *Middleware) ObjectTypeRelationAdd(req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	response := func(code pb.RpcObjectTypeRelationAddResponseErrorCode, err error) *pb.RpcObjectTypeRelationAddResponse {
		m := &pb.RpcObjectTypeRelationAddResponse{Error: &pb.RpcObjectTypeRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if strings.HasPrefix(req.ObjectTypeURL, bundledObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	if !strings.HasPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_OBJECT_TYPE_URL, fmt.Errorf("incorrect object type URL format"))
	}

	sbid := strings.TrimPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix)

	sb, err := mw.Anytype.GetBlock(sbid)
	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
	}

	err = mw.doBlockService(func(bs block.Service) (err error) {
		_, err = bs.AddRelations(sb.ID(), req.Relations)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeRelationUpdate(req *pb.RpcObjectTypeRelationUpdateRequest) *pb.RpcObjectTypeRelationUpdateResponse {
	response := func(code pb.RpcObjectTypeRelationUpdateResponseErrorCode, err error) *pb.RpcObjectTypeRelationUpdateResponse {
		m := &pb.RpcObjectTypeRelationUpdateResponse{Error: &pb.RpcObjectTypeRelationUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if strings.HasPrefix(req.ObjectTypeURL, bundledObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationUpdateResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	if !strings.HasPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix) {
		return response(pb.RpcObjectTypeRelationUpdateResponseError_UNKNOWN_OBJECT_TYPE_URL, fmt.Errorf("incorrect object type URL format"))
	}

	sbid := strings.TrimPrefix(req.ObjectTypeURL, customObjectTypeURLPrefix)

	sb, err := mw.Anytype.GetBlock(sbid)
	if err != nil {
		return response(pb.RpcObjectTypeRelationUpdateResponseError_UNKNOWN_OBJECT_TYPE_URL, err)
	}

	err = mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.UpdateRelations(sb.ID(), []*pbrelation.Relation{req.Relation})
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

func (mw *Middleware) ObjectTypeCreate(req *pb.RpcObjectTypeCreateRequest) *pb.RpcObjectTypeCreateResponse {
	response := func(code pb.RpcObjectTypeCreateResponseErrorCode, otype *pbrelation.ObjectType, err error) *pb.RpcObjectTypeCreateResponse {
		m := &pb.RpcObjectTypeCreateResponse{Error: &pb.RpcObjectTypeCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	sb, err := mw.Anytype.CreateBlock(smartblock.SmartBlockTypeObjectType)
	var relations []*pbrelation.Relation
	err = mw.doBlockService(func(bs block.Service) (err error) {
		details := []*pb.RpcBlockSetDetailsDetail{
			{
				Key:   "name",
				Value: pbtypes.String(req.ObjectType.Name),
			},
			{
				Key:   "layout",
				Value: pbtypes.Float64(float64(req.ObjectType.Layout)),
			},
		}

		err = bs.SetDetails(pb.RpcBlockSetDetailsRequest{ContextId: sb.ID(), Details: details})
		if err != nil {
			return err
		}

		relations, err = bs.AddRelations(sb.ID(), req.ObjectType.Relations)
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
	otype.Url = customObjectTypeURLPrefix + sb.ID()
	return response(pb.RpcObjectTypeCreateResponseError_NULL, otype, nil)
}

func (mw *Middleware) ObjectTypeList(request *pb.RpcObjectTypeListRequest) *pb.RpcObjectTypeListResponse {
	response := func(code pb.RpcObjectTypeListResponseErrorCode, otypes []*pbrelation.ObjectType, err error) *pb.RpcObjectTypeListResponse {
		m := &pb.RpcObjectTypeListResponse{Error: &pb.RpcObjectTypeListResponseError{Code: code}}
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
			otype, err := bs.GetObjectType(id.String())
			if err != nil {
				return err
			}
			otypes = append(otypes, otype)
			return nil
		})
	}

	return response(pb.RpcObjectTypeListResponseError_NULL, otypes, nil)
}

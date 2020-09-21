package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
)

func (mw *Middleware) RelationList(request *pb.RpcRelationListRequest) *pb.RpcRelationListResponse {
	response := func(code pb.RpcRelationListResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcRelationListResponse {
		m := &pb.RpcRelationListResponse{Error: &pb.RpcRelationListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	objType, err := relation.GetObjectType(request.ObjectType)
	if err != nil {
		if err == relation.ErrNotFound {
			return response(pb.RpcRelationListResponseError_UNKNOWN_OBJECT_TYPE, nil, err)
		}
		return response(pb.RpcRelationListResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcRelationListResponseError_NULL, objType.Relations, nil)
}

func (mw *Middleware) RelationAdd(request *pb.RpcRelationAddRequest) *pb.RpcRelationAddResponse {
	panic("implement me")
}

func (mw *Middleware) ObjectTypeCreate(request *pb.RpcObjectTypeCreateRequest) *pb.RpcObjectTypeCreateResponse {
	panic("implement me")
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

	return response(pb.RpcObjectTypeListResponseError_NULL, otypes, nil)
}

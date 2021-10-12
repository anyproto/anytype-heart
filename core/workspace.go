package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) WorkspaceCreate(req *pb.RpcWorkspaceCreateRequest) *pb.RpcWorkspaceCreateResponse {
	response := func(workspaceId string, code pb.RpcWorkspaceCreateResponseErrorCode, err error) *pb.RpcWorkspaceCreateResponse {
		m := &pb.RpcWorkspaceCreateResponse{WorkspaceId: workspaceId, Error: &pb.RpcWorkspaceCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		workspaceId, err = bs.CreateWorkspace(req)
		return
	})
	if err != nil {
		return response("", pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceId, pb.RpcWorkspaceCreateResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceSetTitleObject(req *pb.RpcWorkspaceSetTitleObjectRequest) *pb.RpcWorkspaceSetTitleObjectResponse {
	response := func(code pb.RpcWorkspaceSetTitleObjectResponseErrorCode, err error) *pb.RpcWorkspaceSetTitleObjectResponse {
		m := &pb.RpcWorkspaceSetTitleObjectResponse{Error: &pb.RpcWorkspaceSetTitleObjectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.SetWorkspaceTitleObject(req)
		return
	})
	if err != nil {
		return response(pb.RpcWorkspaceSetTitleObjectResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcWorkspaceSetTitleObjectResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceSelect(req *pb.RpcWorkspaceSelectRequest) *pb.RpcWorkspaceSelectResponse {
	response := func(code pb.RpcWorkspaceSelectResponseErrorCode, err error) *pb.RpcWorkspaceSelectResponse {
		m := &pb.RpcWorkspaceSelectResponse{Error: &pb.RpcWorkspaceSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs block.Service) error {
		return bs.SelectWorkspace(req)
	})
	if err != nil {
		return response(pb.RpcWorkspaceSelectResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcWorkspaceSelectResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceGetAll(req *pb.RpcWorkspaceGetAllRequest) *pb.RpcWorkspaceGetAllResponse {
	response := func(workspaceIds []string, code pb.RpcWorkspaceGetAllResponseErrorCode, err error) *pb.RpcWorkspaceGetAllResponse {
		m := &pb.RpcWorkspaceGetAllResponse{WorkspaceIds: workspaceIds, Error: &pb.RpcWorkspaceGetAllResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceIds []string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		workspaceIds, err = bs.GetAllWorkspaces(req)
		return
	})
	if err != nil {
		return response([]string{}, pb.RpcWorkspaceGetAllResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceIds, pb.RpcWorkspaceGetAllResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceGetCurrent(req *pb.RpcWorkspaceGetCurrentRequest) *pb.RpcWorkspaceGetCurrentResponse {
	response := func(workspaceId string, code pb.RpcWorkspaceGetCurrentResponseErrorCode, err error) *pb.RpcWorkspaceGetCurrentResponse {
		m := &pb.RpcWorkspaceGetCurrentResponse{WorkspaceId: workspaceId, Error: &pb.RpcWorkspaceGetCurrentResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		workspaceId, err = bs.GetCurrentWorkspace(req)
		return
	})
	if err != nil {
		return response("", pb.RpcWorkspaceGetCurrentResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceId, pb.RpcWorkspaceGetCurrentResponseError_NULL, nil)
}

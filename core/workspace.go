package core

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) WorkspaceCreate(cctx context.Context, req *pb.RpcWorkspaceCreateRequest) *pb.RpcWorkspaceCreateResponse {
	response := func(workspaceId string, code pb.RpcWorkspaceCreateResponseErrorCode, err error) *pb.RpcWorkspaceCreateResponse {
		m := &pb.RpcWorkspaceCreateResponse{SpaceId: workspaceId, Error: &pb.RpcWorkspaceCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		workspaceId, err = bs.CreateWorkspace(cctx, req)
		return
	})
	if err != nil {
		return response("", pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceId, pb.RpcWorkspaceCreateResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceOpen(cctx context.Context, req *pb.RpcWorkspaceOpenRequest) *pb.RpcWorkspaceOpenResponse {
	response := func(info *model.AccountInfo, code pb.RpcWorkspaceOpenResponseErrorCode, err error) *pb.RpcWorkspaceOpenResponse {
		m := &pb.RpcWorkspaceOpenResponse{
			Info:  info,
			Error: &pb.RpcWorkspaceOpenResponseError{Code: code},
		}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	// TODO: [MR] this should probably be related only to account
	info, err := getService[account.Service](mw).GetInfo(cctx, req.SpaceId)
	if err != nil {
		return response(info, pb.RpcWorkspaceOpenResponseError_UNKNOWN_ERROR, err)
	}

	err = mw.doBlockService(func(bs *block.Service) error {
		return cache.Do[*editor.SpaceView](bs, info.SpaceViewId, func(sv *editor.SpaceView) error {
			return sv.UpdateLastOpenedDate()
		})
	})
	if err != nil {
		return response(info, pb.RpcWorkspaceOpenResponseError_UNKNOWN_ERROR, err)
	}

	return response(info, pb.RpcWorkspaceOpenResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceSetInfo(cctx context.Context, req *pb.RpcWorkspaceSetInfoRequest) *pb.RpcWorkspaceSetInfoResponse {
	response := func(code pb.RpcWorkspaceSetInfoResponseErrorCode, err error) *pb.RpcWorkspaceSetInfoResponse {
		m := &pb.RpcWorkspaceSetInfoResponse{Error: &pb.RpcWorkspaceSetInfoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		err = bs.SetSpaceInfo(req)
		return
	})
	if err != nil {
		return response(pb.RpcWorkspaceSetInfoResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcWorkspaceSetInfoResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceSelect(cctx context.Context, req *pb.RpcWorkspaceSelectRequest) *pb.RpcWorkspaceSelectResponse {
	response := func(code pb.RpcWorkspaceSelectResponseErrorCode, err error) *pb.RpcWorkspaceSelectResponse {
		m := &pb.RpcWorkspaceSelectResponse{Error: &pb.RpcWorkspaceSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.SelectWorkspace(req)
	})
	if err != nil {
		return response(pb.RpcWorkspaceSelectResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcWorkspaceSelectResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceGetAll(cctx context.Context, req *pb.RpcWorkspaceGetAllRequest) *pb.RpcWorkspaceGetAllResponse {
	response := func(workspaceIds []string, code pb.RpcWorkspaceGetAllResponseErrorCode, err error) *pb.RpcWorkspaceGetAllResponse {
		m := &pb.RpcWorkspaceGetAllResponse{WorkspaceIds: workspaceIds, Error: &pb.RpcWorkspaceGetAllResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceIds []string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		workspaceIds, err = bs.GetAllWorkspaces(req)
		return
	})
	if err != nil {
		return response([]string{}, pb.RpcWorkspaceGetAllResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceIds, pb.RpcWorkspaceGetAllResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceGetCurrent(cctx context.Context, req *pb.RpcWorkspaceGetCurrentRequest) *pb.RpcWorkspaceGetCurrentResponse {
	response := func(workspaceId string, code pb.RpcWorkspaceGetCurrentResponseErrorCode, err error) *pb.RpcWorkspaceGetCurrentResponse {
		m := &pb.RpcWorkspaceGetCurrentResponse{WorkspaceId: workspaceId, Error: &pb.RpcWorkspaceGetCurrentResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var workspaceId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		workspaceId, err = bs.GetCurrentWorkspace(req)
		return
	})
	if err != nil {
		return response("", pb.RpcWorkspaceGetCurrentResponseError_UNKNOWN_ERROR, err)
	}

	return response(workspaceId, pb.RpcWorkspaceGetCurrentResponseError_NULL, nil)
}

// WorkspaceObjectListAdd is unused
func (mw *Middleware) WorkspaceObjectListAdd(cctx context.Context, req *pb.RpcWorkspaceObjectListAddRequest) *pb.RpcWorkspaceObjectListAddResponse {
	response := func(ids []string, code pb.RpcWorkspaceObjectListAddResponseErrorCode, err error) *pb.RpcWorkspaceObjectListAddResponse {
		m := &pb.RpcWorkspaceObjectListAddResponse{ObjectIds: ids, Error: &pb.RpcWorkspaceObjectListAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var (
		ids []string
	)

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		ids, _, err = bs.SpaceInstallBundledObjects(cctx, req.SpaceId, req.ObjectIds)
		return
	})

	if err != nil {
		return response(ids, pb.RpcWorkspaceObjectListAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(ids, pb.RpcWorkspaceObjectListAddResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceObjectAdd(cctx context.Context, req *pb.RpcWorkspaceObjectAddRequest) *pb.RpcWorkspaceObjectAddResponse {
	response := func(id string, details *types.Struct, code pb.RpcWorkspaceObjectAddResponseErrorCode, err error) *pb.RpcWorkspaceObjectAddResponse {
		m := &pb.RpcWorkspaceObjectAddResponse{ObjectId: id, Details: details, Error: &pb.RpcWorkspaceObjectAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var (
		id      string
		details *types.Struct
	)

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		id, details, err = bs.SpaceInstallBundledObject(cctx, req.SpaceId, req.ObjectId)
		return
	})

	if err != nil {
		return response(id, details, pb.RpcWorkspaceObjectAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(id, details, pb.RpcWorkspaceObjectAddResponseError_NULL, nil)
}

func (mw *Middleware) WorkspaceObjectListRemove(cctx context.Context, req *pb.RpcWorkspaceObjectListRemoveRequest) *pb.RpcWorkspaceObjectListRemoveResponse {
	response := func(ids []string, code pb.RpcWorkspaceObjectListRemoveResponseErrorCode, err error) *pb.RpcWorkspaceObjectListRemoveResponse {
		m := &pb.RpcWorkspaceObjectListRemoveResponse{Ids: ids, Error: &pb.RpcWorkspaceObjectListRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.DeleteArchivedObjects(req.ObjectIds)
	})
	if err != nil {
		return response([]string{}, pb.RpcWorkspaceObjectListRemoveResponseError_UNKNOWN_ERROR, err)
	}
	return response(req.ObjectIds, 0, nil)
}

// WorkspaceExport is unused now, it must be fixed if someone wants to use it
func (mw *Middleware) WorkspaceExport(cctx context.Context, req *pb.RpcWorkspaceExportRequest) *pb.RpcWorkspaceExportResponse {
	return &pb.RpcWorkspaceExportResponse{
		Error: &pb.RpcWorkspaceExportResponseError{
			Code:        pb.RpcWorkspaceExportResponseError_NULL,
			Description: "Not implemented",
		},
	}
}

func (mw *Middleware) WorkspaceReinit(cctx context.Context, req *pb.RpcWorkspaceReinitRequest) *pb.RpcWorkspaceReinitResponse {
	response := func(path string, err error) (res *pb.RpcWorkspaceReinitResponse) {
		res = &pb.RpcWorkspaceReinitResponse{
			Error: &pb.RpcWorkspaceReinitResponseError{
				Code: pb.RpcWorkspaceReinitResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcWorkspaceReinitResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Path = path
		}
		return res
	}
	var (
		path string
		err  error
	)
	err = mw.doBlockService(func(bs *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		// create temp dir
		tempDirProvider := mw.applicationService.GetApp().MustComponent(core.CName).(core.TempDirProvider)
		path, _, err = es.Export(cctx, pb.RpcObjectListExportRequest{
			SpaceId: req.WorkspaceId,
			Path:    tempDirProvider.TempDir(),
			Format:  model.Export_Protobuf,
		})
		os := mw.applicationService.GetApp().MustComponent(objectstore.CName).(objectstore.ObjectStore)
		ids, _, err := os.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.ObjectType_basic), int(model.ObjectType_profile), int(model.ObjectType_todo), int(model.ObjectType_set), int(model.ObjectType_note), int(model.ObjectType_bookmark)),
				},
				{
					RelationKey: bundle.RelationKeySpaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(req.WorkspaceId),
				},
			},
		})

		for _, id := range ids {
			err := bs.DeleteObjectByFullID(domain.FullID{SpaceID: req.WorkspaceId, ObjectID: id})
			if err != nil {
				log.With("error", err, "objectId", id).Error("space reinit: failed to perform delete operation on object")
			}
		}

		resp := mw.ObjectImport(cctx, &pb.RpcObjectImportRequest{
			SpaceId:               req.WorkspaceId,
			UpdateExistingObjects: true,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			Type:                  model.Import_Pb,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
				Path:            []string{path},
				NoCollection:    true,
				CollectionTitle: "",
				ImportType:      0,
			},
			}})
		if resp.Error.Code != pb.RpcObjectImportResponseError_NULL {
			return errors.New(resp.Error.Description)
		}
		return err
	})
	return response(path, err)
}

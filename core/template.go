package core

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) TemplateCreateFromObject(cctx context.Context, req *pb.RpcTemplateCreateFromObjectRequest) *pb.RpcTemplateCreateFromObjectResponse {
	response := func(templateId string, err error) *pb.RpcTemplateCreateFromObjectResponse {
		m := &pb.RpcTemplateCreateFromObjectResponse{
			Error: &pb.RpcTemplateCreateFromObjectResponseError{Code: pb.RpcTemplateCreateFromObjectResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcTemplateCreateFromObjectResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		templateId, err = bs.TemplateCreateFromObject(cctx, req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) TemplateClone(cctx context.Context, req *pb.RpcTemplateCloneRequest) *pb.RpcTemplateCloneResponse {
	response := func(templateId string, err error) *pb.RpcTemplateCloneResponse {
		m := &pb.RpcTemplateCloneResponse{
			Error: &pb.RpcTemplateCloneResponseError{Code: pb.RpcTemplateCloneResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcTemplateCloneResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		templateId, err = bs.TemplateClone(req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) ObjectApplyTemplate(cctx context.Context, req *pb.RpcObjectApplyTemplateRequest) *pb.RpcObjectApplyTemplateResponse {
	response := func(err error) *pb.RpcObjectApplyTemplateResponse {
		m := &pb.RpcObjectApplyTemplateResponse{
			Error: &pb.RpcObjectApplyTemplateResponseError{Code: pb.RpcObjectApplyTemplateResponseError_NULL},
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectApplyTemplateResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.ObjectApplyTemplate(req.ContextId, req.TemplateId)
	})
	return response(err)
}

func (mw *Middleware) TemplateCreateFromObjectType(cctx context.Context, req *pb.RpcTemplateCreateFromObjectTypeRequest) *pb.RpcTemplateCreateFromObjectTypeResponse {
	response := func(templateId string, err error) *pb.RpcTemplateCreateFromObjectTypeResponse {
		m := &pb.RpcTemplateCreateFromObjectTypeResponse{
			Error: &pb.RpcTemplateCreateFromObjectTypeResponseError{Code: pb.RpcTemplateCreateFromObjectTypeResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcTemplateCreateFromObjectTypeResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		templateId, err = bs.TemplateCreateFromObjectByObjectType(cctx, req.ObjectType)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) TemplateExportAll(cctx context.Context, req *pb.RpcTemplateExportAllRequest) *pb.RpcTemplateExportAllResponse {
	response := func(path string, err error) (res *pb.RpcTemplateExportAllResponse) {
		res = &pb.RpcTemplateExportAllResponse{
			Error: &pb.RpcTemplateExportAllResponseError{
				Code: pb.RpcTemplateExportAllResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcTemplateExportAllResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Path = path
		}
		return res
	}
	var (
		path string
	)
	err := mw.doBlockService(func(_ *block.Service) error {
		es := mw.app.MustComponent(export.CName).(export.Export)
		ds := mw.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
		docIds, _, err := ds.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIsArchived.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(false),
				},
			},
		}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeTemplate})
		if err != nil {
			return err
		}
		if len(docIds) == 0 {
			return fmt.Errorf("no templates")
		}
		path, _, err = es.Export(cctx, pb.RpcObjectListExportRequest{
			Path:      req.Path,
			ObjectIds: docIds,
			Format:    pb.RpcObjectListExport_Protobuf,
			Zip:       true,
		})
		return err
	})
	return response(path, err)
}

// WorkspaceExport is unused now, it must be fixed if someone wants to use it
func (mw *Middleware) WorkspaceExport(cctx context.Context, req *pb.RpcWorkspaceExportRequest) *pb.RpcWorkspaceExportResponse {
	response := func(path string, err error) (res *pb.RpcWorkspaceExportResponse) {
		res = &pb.RpcWorkspaceExportResponse{
			Error: &pb.RpcWorkspaceExportResponseError{
				Code: pb.RpcWorkspaceExportResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcWorkspaceExportResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Path = path
		}
		return res
	}
	var (
		path string
	)
	err := mw.doBlockService(func(_ *block.Service) error {
		es := mw.app.MustComponent(export.CName).(export.Export)
		ds := mw.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
		docIds, _, err := ds.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIsArchived.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(false),
				},
				{
					RelationKey: bundle.RelationKeyWorkspaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(req.WorkspaceId),
				},
			},
		}, []smartblock.SmartBlockType{})
		if err != nil {
			return err
		}
		if len(docIds) == 0 {
			return fmt.Errorf("no objects in workspace")
		}
		path, _, err = es.Export(cctx, pb.RpcObjectListExportRequest{
			Path:          req.Path,
			ObjectIds:     docIds,
			Format:        pb.RpcObjectListExport_Protobuf,
			Zip:           true,
			IncludeNested: false,
		})
		return err
	})
	return response(path, err)
}

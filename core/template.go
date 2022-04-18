package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (mw *Middleware) TemplateCreateFromObject(req *pb.RpcTemplateCreateFromObjectRequest) *pb.RpcTemplateCreateFromObjectResponse {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		templateId, err = bs.TemplateCreateFromObject(req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) TemplateClone(req *pb.RpcTemplateCloneRequest) *pb.RpcTemplateCloneResponse {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		templateId, err = bs.TemplateClone(req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) ObjectApplyTemplate(req *pb.RpcObjectApplyTemplateRequest) *pb.RpcObjectApplyTemplateResponse {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.ObjectApplyTemplate(req.ContextId, req.TemplateId)
	})
	return response(err)
}

func (mw *Middleware) TemplateCreateFromObjectType(req *pb.RpcTemplateCreateFromObjectTypeRequest) *pb.RpcTemplateCreateFromObjectTypeResponse {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		templateId, err = bs.TemplateCreateFromObjectByObjectType(req.ObjectType)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) TemplateExportAll(req *pb.RpcTemplateExportAllRequest) *pb.RpcTemplateExportAllResponse {
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
		err  error
	)
	err = mw.doBlockService(func(_ block.Service) error {
		es := mw.app.MustComponent(export.CName).(export.Export)
		ds := mw.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
		res, _, err := ds.QueryObjectInfo(database.Query{
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
		var docIds []string
		for _, r := range res {
			docIds = append(docIds, r.Id)
		}
		if len(docIds) == 0 {
			return fmt.Errorf("no templates")
		}
		path, _, err = es.Export(pb.RpcObjectListExportRequest{
			Path:      req.Path,
			ObjectIds: docIds,
			Format:    pb.RpcObjectExport_Protobuf,
			Zip:       true,
		})
		return err
	})
	return response(path, err)
}

func (mw *Middleware) WorkspaceExport(req *pb.RpcWorkspaceExportRequest) *pb.RpcWorkspaceExportResponse {
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
		err  error
	)
	err = mw.doBlockService(func(_ block.Service) error {
		es := mw.app.MustComponent(export.CName).(export.Export)
		ds := mw.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
		res, _, err := ds.QueryObjectInfo(database.Query{
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
		var docIds []string
		for _, r := range res {
			docIds = append(docIds, r.Id)
		}
		if len(docIds) == 0 {
			return fmt.Errorf("no objects in workspace")
		}
		path, _, err = es.Export(pb.RpcObjectListExportRequest{
			Path:          req.Path,
			ObjectIds:     docIds,
			Format:        pb.RpcObjectExport_Protobuf,
			Zip:           true,
			IncludeNested: false,
		})
		return err
	})
	return response(path, err)
}

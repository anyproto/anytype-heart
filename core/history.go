package core

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/history"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) HistoryShowVersion(cctx context.Context, req *pb.RpcHistoryShowVersionRequest) *pb.RpcHistoryShowVersionResponse {
	response := func(obj *model.ObjectView, ver *pb.RpcHistoryVersion, err error) (res *pb.RpcHistoryShowVersionResponse) {
		res = &pb.RpcHistoryShowVersionResponse{
			Error: &pb.RpcHistoryShowVersionResponseError{
				Code: pb.RpcHistoryShowVersionResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryShowVersionResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.ObjectView = obj
			res.Version = ver
			res.TraceId = req.TraceId
		}
		return res
	}
	var (
		obj *model.ObjectView
		ver *pb.RpcHistoryVersion
		err error
	)
	if err = mw.doBlockService(func(bs *block.Service) (err error) {
		hs := mw.applicationService.GetApp().MustComponent(history.CName).(history.History)
		res := mw.applicationService.GetApp().MustComponent(idresolver.CName).(idresolver.Resolver)
		spaceID, err := res.ResolveSpaceID(req.ObjectId)
		if err != nil {
			return fmt.Errorf("resolve spaceID: %w", err)
		}
		obj, ver, err = hs.Show(domain.FullID{
			SpaceID:  spaceID,
			ObjectID: req.ObjectId,
		}, req.VersionId)
		return
	}); err != nil {
		return response(nil, nil, err)
	}

	return response(obj, ver, nil)
}

func (mw *Middleware) HistoryGetVersions(cctx context.Context, req *pb.RpcHistoryGetVersionsRequest) *pb.RpcHistoryGetVersionsResponse {
	response := func(vers []*pb.RpcHistoryVersion, err error) (res *pb.RpcHistoryGetVersionsResponse) {
		res = &pb.RpcHistoryGetVersionsResponse{
			Error: &pb.RpcHistoryGetVersionsResponseError{
				Code: pb.RpcHistoryGetVersionsResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryGetVersionsResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Versions = vers
		}
		return res
	}
	var (
		vers []*pb.RpcHistoryVersion
		err  error
	)
	if err = mw.doBlockService(func(bs *block.Service) (err error) {
		hs := mw.applicationService.GetApp().MustComponent(history.CName).(history.History)
		res := mw.applicationService.GetApp().MustComponent(idresolver.CName).(idresolver.Resolver)
		spaceID, err := res.ResolveSpaceID(req.ObjectId)
		if err != nil {
			return fmt.Errorf("resolve spaceID: %w", err)
		}
		vers, err = hs.Versions(domain.FullID{
			SpaceID:  spaceID,
			ObjectID: req.ObjectId,
		}, req.LastVersionId, int(req.Limit))
		return
	}); err != nil {
		return response(nil, err)
	}
	return response(vers, nil)
}

func (mw *Middleware) HistorySetVersion(cctx context.Context, req *pb.RpcHistorySetVersionRequest) *pb.RpcHistorySetVersionResponse {
	response := func(err error) (res *pb.RpcHistorySetVersionResponse) {
		res = &pb.RpcHistorySetVersionResponse{
			Error: &pb.RpcHistorySetVersionResponseError{
				Code: pb.RpcHistorySetVersionResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistorySetVersionResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		}
		return
	}
	return response(mw.doBlockService(func(bs *block.Service) (err error) {
		hs := mw.applicationService.GetApp().MustComponent(history.CName).(history.History)
		res := mw.applicationService.GetApp().MustComponent(idresolver.CName).(idresolver.Resolver)
		spaceID, err := res.ResolveSpaceID(req.ObjectId)
		if err != nil {
			return fmt.Errorf("resolve spaceID: %w", err)
		}
		return hs.SetVersion(domain.FullID{
			SpaceID:  spaceID,
			ObjectID: req.ObjectId,
		}, req.VersionId)
	}))
}

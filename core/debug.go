package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/debug"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) DebugTree(cctx context.Context, req *pb.RpcDebugTreeRequest) *pb.RpcDebugTreeResponse {
	response := func(err error, filename string) *pb.RpcDebugTreeResponse {
		rpcErr := &pb.RpcDebugTreeResponseError{
			Code: pb.RpcDebugTreeResponseError_NULL,
		}
		if err != nil {
			rpcErr.Code = pb.RpcDebugTreeResponseError_UNKNOWN_ERROR
			rpcErr.Description = err.Error()
		}
		return &pb.RpcDebugTreeResponse{
			Error:    rpcErr,
			Filename: filename,
		}
	}

	app := mw.GetApp()
	if app == nil {
		return response(ErrNotLoggedIn, "")
	}

	dbg := app.MustComponent(debug.CName).(debug.Debug)
	filename, err := dbg.DumpTree(cctx, req.TreeId, req.Path, !req.Unanonymized, req.GenerateSvg)
	return response(err, filename)
}

func (mw *Middleware) DebugTreeHeads(cctx context.Context, req *pb.RpcDebugTreeHeadsRequest) *pb.RpcDebugTreeHeadsResponse {
	response := func(err error, treeInfo debug.TreeInfo) *pb.RpcDebugTreeHeadsResponse {
		rpcErr := &pb.RpcDebugTreeHeadsResponseError{
			Code: pb.RpcDebugTreeHeadsResponseError_NULL,
		}
		if err != nil {
			rpcErr.Code = pb.RpcDebugTreeHeadsResponseError_UNKNOWN_ERROR
			rpcErr.Description = err.Error()
		}
		return &pb.RpcDebugTreeHeadsResponse{
			Error:   rpcErr,
			SpaceId: treeInfo.SpaceId,
			Info: &pb.RpcDebugTreeInfo{
				TreeId:  treeInfo.Id,
				HeadIds: treeInfo.Heads,
			},
		}
	}

	app := mw.GetApp()
	if app == nil {
		return response(ErrNotLoggedIn, debug.TreeInfo{})
	}

	dbg := app.MustComponent(debug.CName).(debug.Debug)
	treeInfo, err := dbg.TreeHeads(cctx, req.TreeId)
	if err != nil {
		return response(err, debug.TreeInfo{})
	}
	return response(nil, treeInfo)
}

func (mw *Middleware) DebugSpaceSummary(cctx context.Context, req *pb.RpcDebugSpaceSummaryRequest) *pb.RpcDebugSpaceSummaryResponse {
	response := func(err error, spaceSummary debug.SpaceSummary) *pb.RpcDebugSpaceSummaryResponse {
		rpcErr := &pb.RpcDebugSpaceSummaryResponseError{
			Code: pb.RpcDebugSpaceSummaryResponseError_NULL,
		}
		if err != nil {
			rpcErr.Code = pb.RpcDebugSpaceSummaryResponseError_UNKNOWN_ERROR
			rpcErr.Description = err.Error()
		}
		infos := make([]*pb.RpcDebugTreeInfo, 0, len(spaceSummary.TreeInfos))
		for _, i := range spaceSummary.TreeInfos {
			infos = append(infos, &pb.RpcDebugTreeInfo{
				TreeId:  i.Id,
				HeadIds: i.Heads,
			})
		}
		return &pb.RpcDebugSpaceSummaryResponse{
			Error:   rpcErr,
			SpaceId: spaceSummary.SpaceId,
			Infos:   infos,
		}
	}

	app := mw.GetApp()
	if app == nil {
		return response(ErrNotLoggedIn, debug.SpaceSummary{})
	}
	dbg := app.MustComponent(debug.CName).(debug.Debug)
	spaceSummary, err := dbg.SpaceSummary(cctx, req.SpaceId)
	if err != nil {
		return response(err, debug.SpaceSummary{})
	}
	return response(nil, spaceSummary)
}

func (mw *Middleware) DebugStackGoroutines(_ context.Context, req *pb.RpcDebugStackGoroutinesRequest) *pb.RpcDebugStackGoroutinesResponse {
	response := func(err error) (res *pb.RpcDebugStackGoroutinesResponse) {
		res = &pb.RpcDebugStackGoroutinesResponse{
			Error: &pb.RpcDebugStackGoroutinesResponseError{
				Code: pb.RpcDebugStackGoroutinesResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcDebugStackGoroutinesResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
		}
		return res
	}

	err := mw.SaveGoroutinesStack(req.Path)
	return response(err)
}

func (mw *Middleware) DebugExportLocalstore(cctx context.Context, req *pb.RpcDebugExportLocalstoreRequest) *pb.RpcDebugExportLocalstoreResponse {
	response := func(path string, err error) (res *pb.RpcDebugExportLocalstoreResponse) {
		res = &pb.RpcDebugExportLocalstoreResponse{
			Error: &pb.RpcDebugExportLocalstoreResponseError{
				Code: pb.RpcDebugExportLocalstoreResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcDebugExportLocalstoreResponseError_UNKNOWN_ERROR
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
	err = mw.doBlockService(func(s *block.Service) error {
		dbg := mw.applicationService.GetApp().MustComponent(debug.CName).(debug.Debug)
		path, err = dbg.DumpLocalstore(cctx, req.SpaceId, req.DocIds, req.Path)
		return err
	})
	return response(path, err)
}

func (mw *Middleware) DebugSubscriptions(_ context.Context, _ *pb.RpcDebugSubscriptionsRequest) *pb.RpcDebugSubscriptionsResponse {
	response := func(subscriptions []string, err error) (res *pb.RpcDebugSubscriptionsResponse) {
		res = &pb.RpcDebugSubscriptionsResponse{
			Error: &pb.RpcDebugSubscriptionsResponseError{
				Code: pb.RpcDebugSubscriptionsResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcDebugSubscriptionsResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		}
		res.Subscriptions = subscriptions
		return res
	}
	var subscriptions []string
	err := mw.doBlockService(func(s *block.Service) error {
		subscriptions = getService[subscription.Service](mw).SubscriptionIDs()
		return nil
	})
	return response(subscriptions, err)
}

func (mw *Middleware) DebugOpenedObjects(_ context.Context, _ *pb.RpcDebugOpenedObjectsRequest) *pb.RpcDebugOpenedObjectsResponse {
	response := func(objectIDs []string, err error) (res *pb.RpcDebugOpenedObjectsResponse) {
		res = &pb.RpcDebugOpenedObjectsResponse{
			Error: &pb.RpcDebugOpenedObjectsResponseError{
				Code: pb.RpcDebugOpenedObjectsResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcDebugOpenedObjectsResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		}
		res.ObjectIDs = objectIDs
		return res
	}
	var objectIDs []string
	err := mw.doBlockService(func(s *block.Service) error {
		objectIDs = s.GetOpenedObjects()
		return nil
	})
	return response(objectIDs, err)
}

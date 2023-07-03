package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/debug"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) DebugTree(cctx context.Context, req *pb.RpcDebugTreeRequest) *pb.RpcDebugTreeResponse {
	ctx := mw.newContext(cctx)
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
	filename, err := dbg.DumpTree(ctx, req.TreeId, req.Path, !req.Unanonymized, req.GenerateSvg)
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
	treeInfo, err := dbg.TreeHeads(req.TreeId)
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
	spaceSummary, err := dbg.SpaceSummary()
	if err != nil {
		return response(err, debug.SpaceSummary{})
	}
	return response(nil, spaceSummary)
}

func (mw *Middleware) DebugExportLocalstore(cctx context.Context, req *pb.RpcDebugExportLocalstoreRequest) *pb.RpcDebugExportLocalstoreResponse {
	ctx := mw.newContext(cctx)
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
		dbg := mw.app.MustComponent(debug.CName).(debug.Debug)
		path, err = dbg.DumpLocalstore(ctx, req.DocIds, req.Path)
		return err
	})
	return response(path, err)
}

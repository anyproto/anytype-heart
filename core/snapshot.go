package core

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) SnapshotOpen(cctx context.Context, req *pb.RpcSnapshotOpenRequest) *pb.RpcSnapshotOpenResponse {
	response := func(code pb.RpcSnapshotOpenResponseErrorCode, records []*types.Struct, err error) *pb.RpcSnapshotOpenResponse {
		m := &pb.RpcSnapshotOpenResponse{Error: &pb.RpcSnapshotOpenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Records = records
		}
		return m
	}
	var records []*types.Struct
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		records, err = bs.SnapshotOpen(cctx, req.ZipPath, req.SpaceId)
		return err
	})
	if err != nil {
		return response(pb.RpcSnapshotOpenResponseError_UNKNOWN_ERROR, records, err)
	}
	return response(pb.RpcSnapshotOpenResponseError_NULL, records, nil)
}

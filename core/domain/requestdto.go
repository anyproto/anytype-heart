package domain

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type BlockUploadRequest struct {
	pb.RpcBlockUploadRequest
	Origin model.ObjectOrigin
}

type FileUploadRequest struct {
	pb.RpcFileUploadRequest
	Origin model.ObjectOrigin
}

type BookmarkFetchRequest struct {
	pb.RpcBlockBookmarkFetchRequest
	Origin model.ObjectOrigin
}

type BookmarkCreateAndFetchRequest struct {
	pb.RpcBlockBookmarkCreateAndFetchRequest
	Origin model.ObjectOrigin
}

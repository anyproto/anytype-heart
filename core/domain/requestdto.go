package domain

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type BlockUploadRequestDTO struct {
	pb.RpcBlockUploadRequest
	Origin model.ObjectOrigin
}

type FileUploadRequestDTO struct {
	pb.RpcFileUploadRequest
	Origin model.ObjectOrigin
}

type BookmarkFetchRequestDTO struct {
	pb.RpcBlockBookmarkFetchRequest
	Origin model.ObjectOrigin
}

package syncer

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
)

type BlockService interface {
	GetObject(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
	GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error)
	UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (objectId string, details *types.Struct, err error)
	UploadBlockFile(ctx session.Context, req block.UploadRequest, groupID string, isSync bool) (fileObjectId string, err error)
}

type Syncer interface {
	Sync(id domain.FullID, newIdsSet map[string]struct{}, b simple.Block, origin objectorigin.ObjectOrigin) error
}

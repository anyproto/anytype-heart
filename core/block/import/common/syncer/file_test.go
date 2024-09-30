package syncer

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	block2 "github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer/mock_syncer"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestFileSyncer_Sync(t *testing.T) {
	spaceId := "spaceId"
	objectID := "objectId"
	t.Run("file missing", func(t *testing.T) {
		// given
		syncer := NewFileSyncer(nil, nil)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}},
		}
		simpleBlock := simple.New(block)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "", simpleBlock.Model().GetFile().GetTargetObjectId())
	})
	t.Run("file already loaded", func(t *testing.T) {
		// given
		syncer := NewFileSyncer(nil, nil)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "hash"}},
		}
		simpleBlock := simple.New(block)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "hash", simpleBlock.Model().GetFile().GetTargetObjectId())
	})
	t.Run("file not exist", func(t *testing.T) {
		// given
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Name: "not exist"}},
		}
		simpleBlock := simple.New(block)
		service := mock_syncer.NewMockBlockService(t)
		params := pb.RpcBlockUploadRequest{
			FilePath: "not exist",
			BlockId:  simpleBlock.Model().GetId(),
		}
		dto := block2.UploadRequest{
			RpcBlockUploadRequest: params,
			ObjectOrigin:          objectorigin.Import(model.Import_Pb),
		}
		service.EXPECT().UploadFileBlock(id.ObjectID, dto).Return("", os.ErrNotExist)
		syncer := NewFileSyncer(service, nil)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.NotNil(t, err)
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("file not loaded", func(t *testing.T) {
		// given
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Name: "not exist"}},
		}
		simpleBlock := simple.New(block)
		service := mock_syncer.NewMockBlockService(t)
		params := pb.RpcBlockUploadRequest{
			FilePath: "not exist",
			BlockId:  simpleBlock.Model().GetId(),
		}
		dto := block2.UploadRequest{
			RpcBlockUploadRequest: params,
			ObjectOrigin:          objectorigin.Import(model.Import_Pb),
		}
		service.EXPECT().UploadFileBlock(id.ObjectID, dto).Return("", fmt.Errorf("new error"))
		syncer := NewFileSyncer(service, nil)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.NotNil(t, err)
		assert.True(t, errors.Is(err, common.ErrFileLoad))
	})
	t.Run("file success loaded", func(t *testing.T) {
		// given
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Name: "exist"}},
		}
		simpleBlock := simple.New(block)
		service := mock_syncer.NewMockBlockService(t)
		params := pb.RpcBlockUploadRequest{
			FilePath: "exist",
			BlockId:  simpleBlock.Model().GetId(),
		}
		dto := block2.UploadRequest{
			RpcBlockUploadRequest: params,
			ObjectOrigin:          objectorigin.Import(model.Import_Pb),
		}
		service.EXPECT().UploadFileBlock(id.ObjectID, dto).Return("fileId", nil)
		syncer := NewFileSyncer(service, nil)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
	})
	t.Run("file success loaded from url", func(t *testing.T) {
		// given
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Name: "http://example.com"}},
		}
		simpleBlock := simple.New(block)
		service := mock_syncer.NewMockBlockService(t)
		params := pb.RpcBlockUploadRequest{
			Url:     "http://example.com",
			BlockId: simpleBlock.Model().GetId(),
		}
		dto := block2.UploadRequest{
			RpcBlockUploadRequest: params,
			ObjectOrigin:          objectorigin.Import(model.Import_Pb),
		}
		service.EXPECT().UploadFileBlock(id.ObjectID, dto).Return("fileId", nil)
		syncer := NewFileSyncer(service, nil)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
	})
	t.Run("file with error state", func(t *testing.T) {
		// given
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{State: model.BlockContentFile_Error}},
		}
		syncer := NewFileSyncer(nil, nil)
		simpleBlock := simple.New(block)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "", simpleBlock.Model().GetFile().GetTargetObjectId())
	})
}

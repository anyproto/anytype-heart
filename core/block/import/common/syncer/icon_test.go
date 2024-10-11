package syncer

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/stretchr/testify/assert"

	block2 "github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer/mock_syncer"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestIconSyncer_Sync(t *testing.T) {
	spaceId := "spaceId"
	objectID := "objectId"
	t.Run("icon image missing", func(t *testing.T) {
		// given
		syncer := NewIconSyncer(nil, nil)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: addr.MissingObject,
			}},
		}
		simpleBlock := simple.New(block)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, addr.MissingObject, simpleBlock.Model().GetText().GetIconImage())
	})
	t.Run("icon image is existing file object", func(t *testing.T) {
		// given
		rawCid, err := cidutil.NewCidFromBytes([]byte("test"))
		assert.Nil(t, err)
		syncer := NewIconSyncer(nil, nil)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: rawCid,
			}},
		}
		simpleBlock := simple.New(block)
		newIdsSet := map[string]struct{}{rawCid: {}}
		// when
		err = syncer.Sync(id, newIdsSet, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, rawCid, simpleBlock.Model().GetText().GetIconImage())
	})
	t.Run("icon image is not presented in import archive", func(t *testing.T) {
		// given
		rawCid, err := cidutil.NewCidFromBytes([]byte("test"))
		assert.Nil(t, err)
		newFileObjectId, err := cidutil.NewCidFromBytes([]byte("test1"))
		assert.Nil(t, err)

		service := mock_fileobject.NewMockService(t)
		fileId := domain.FullFileId{FileId: domain.FileId(rawCid), SpaceId: spaceId}
		service.EXPECT().CreateFromImport(fileId, objectorigin.Import(model.Import_Pb)).Return(newFileObjectId, nil)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: rawCid,
			}},
		}
		rootBlock := &model.Block{
			Id:          "test",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}

		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		doc := state.NewDoc("root", map[string]simple.Block{"root": rootSimpleBlock, "test": simpleBlock})
		smartTest := smarttest.New("root")
		smartTest.Doc = doc

		objectGetter := mock_syncer.NewMockBlockService(t)
		objectGetter.EXPECT().GetObject(context.Background(), objectID).Return(smartTest, nil)
		syncer := NewIconSyncer(objectGetter, service)

		// when
		err = syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		testBlock := smartTest.NewState().Get("test")
		assert.Equal(t, newFileObjectId, testBlock.Model().GetText().GetIconImage())
	})
	t.Run("icon image is url", func(t *testing.T) {
		// given
		service := mock_fileobject.NewMockService(t)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: "http://url.com",
			}},
		}
		rootBlock := &model.Block{
			Id:          "test",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}

		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		doc := state.NewDoc("root", map[string]simple.Block{"root": rootSimpleBlock, "test": simpleBlock})
		smartTest := smarttest.New("root")
		smartTest.Doc = doc

		fileUploader := mock_syncer.NewMockBlockService(t)
		fileUploader.EXPECT().GetObject(context.Background(), objectID).Return(smartTest, nil)
		fileUploader.EXPECT().UploadFile(context.Background(), spaceId, block2.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				Url: "http://url.com",
			},
			ObjectOrigin: objectorigin.Import(model.Import_Pb),
		}).Return("newFileObjectId", nil, nil)

		syncer := NewIconSyncer(fileUploader, service)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		testBlock := smartTest.NewState().Get("test")
		assert.Equal(t, "newFileObjectId", testBlock.Model().GetText().GetIconImage())
	})

	t.Run("icon image is url, failed to upload", func(t *testing.T) {
		// given
		service := mock_fileobject.NewMockService(t)
		id := domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceId,
		}
		block := &model.Block{
			Id: "test",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				IconImage: "http://url.com",
			}},
		}
		rootBlock := &model.Block{
			Id:          "test",
			ChildrenIds: []string{"test"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}

		simpleBlock := simple.New(block)
		rootSimpleBlock := simple.New(rootBlock)
		doc := state.NewDoc("root", map[string]simple.Block{"root": rootSimpleBlock, "test": simpleBlock})
		smartTest := smarttest.New("root")
		smartTest.Doc = doc

		fileUploader := mock_syncer.NewMockBlockService(t)
		fileUploader.EXPECT().GetObject(context.Background(), objectID).Return(smartTest, nil)
		fileUploader.EXPECT().UploadFile(context.Background(), spaceId, block2.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				Url: "http://url.com",
			},
			ObjectOrigin: objectorigin.Import(model.Import_Pb),
		}).Return("", nil, fmt.Errorf("failed to upload"))

		syncer := NewIconSyncer(fileUploader, service)

		// when
		err := syncer.Sync(id, nil, simpleBlock, objectorigin.Import(model.Import_Pb))

		// then
		assert.NotNil(t, err)
		testBlock := smartTest.NewState().Get("test")
		assert.Equal(t, "", testBlock.Model().GetText().GetIconImage())
	})
}

package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fileFixture struct {
	sfile
	pickerFx   *mock_cache.MockObjectGetter
	sb         *smarttest.SmartTest
	mockSender *mock_event.MockSender
}

func newFixture(t *testing.T) *fileFixture {
	picker := mock_cache.NewMockObjectGetter(t)
	sb := smarttest.New("root")
	mockSender := mock_event.NewMockSender(t)
	fx := &fileFixture{
		pickerFx:   picker,
		sb:         sb,
		mockSender: mockSender,
	}

	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
	service := process.New()
	err := service.Init(a)
	assert.Nil(t, err)

	fx.sfile = sfile{
		SmartBlock:     sb,
		picker:         picker,
		processService: service,
	}
	return fx
}

func TestFile(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		typeLayout model.ObjectTypeLayout
		fileType   model.BlockContentFileType
	}{
		{
			name:       "Image",
			typeLayout: model.ObjectType_image,
			fileType:   model.BlockContentFile_Image,
		},
		{
			name:       "Audio",
			typeLayout: model.ObjectType_audio,
			fileType:   model.BlockContentFile_Audio,
		},
		{
			name:       "Video",
			typeLayout: model.ObjectType_video,
			fileType:   model.BlockContentFile_Video,
		},
		{
			name:       "PDF",
			typeLayout: model.ObjectType_pdf,
			fileType:   model.BlockContentFile_PDF,
		},
		{
			name:       "File",
			typeLayout: model.ObjectType_file,
			fileType:   model.BlockContentFile_File,
		},
	} {
		t.Run("SetFileTargetObjectId - when "+testCase.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			fileSb := smarttest.New("root")
			fileSb.SetDetails(nil, []domain.Detail{{
				Key:   bundle.RelationKeyResolvedLayout,
				Value: domain.Int64(int64(testCase.typeLayout)),
			}}, false)

			fx.pickerFx.EXPECT().GetObject(mock.Anything, "testObjId").Return(fileSb, nil)

			fx.sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.File("",
						blockbuilder.ID("blockId1"),
					),
				)))

			// when
			err := fx.sfile.SetFileTargetObjectId(nil, "blockId1", "testObjId")

			// then
			require.NoError(t, err)
			file := fx.sfile.Pick("blockId1").Model().GetFile()

			require.Equal(t, "testObjId", file.TargetObjectId)
			require.Equal(t, testCase.fileType, file.Type)
			require.Equal(t, model.BlockContentFile_Embed, file.Style)
			require.Equal(t, model.BlockContentFile_Done, file.State)
		})
	}
}

func TestDropFiles(t *testing.T) {
	t.Run("do not drop files to object with Blocks restriction", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.sb.TestRestrictions = restriction.Restrictions{Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}}}

		// when
		err := fx.sfile.DropFiles(pb.RpcFileDropRequest{})

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, restriction.ErrRestricted))
	})
	t.Run("drop files in collection - no restriction error", func(t *testing.T) {
		// given
		dir := t.TempDir()
		file, err := os.Create(filepath.Join(dir, "test"))
		assert.Nil(t, err)

		fx := newFixture(t)
		st := fx.sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		fx.sb.Doc = st
		fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		mockService := mock_fileobject.NewMockService(t)
		mockService.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return("fileObjectId", domain.NewDetails(), nil).Maybe()
		fx.fileUploaderFactory = prepareFileService(t, fx.mockSender, mockService)

		// when
		err = fx.sfile.DropFiles(pb.RpcFileDropRequest{
			ContextId:      "root",
			LocalFilePaths: []string{file.Name()},
		})

		// then
		assert.Nil(t, err)
	})
	t.Run("drop dir in collection - no restriction error", func(t *testing.T) {
		// given
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test"))
		assert.Nil(t, err)

		fx := newFixture(t)
		st := fx.sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		fx.sb.Doc = st
		fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		mockService := mock_fileobject.NewMockService(t)
		mockService.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return("fileObjectId", domain.NewDetails(), nil).Maybe()
		fx.fileUploaderFactory = prepareFileService(t, fx.mockSender, mockService)

		// when
		err = fx.sfile.DropFiles(pb.RpcFileDropRequest{
			ContextId:      "root",
			LocalFilePaths: []string{dir},
		})

		// then
		assert.Nil(t, err)
	})
	t.Run("drop files in collection - success", func(t *testing.T) {
		// given
		dir := t.TempDir()
		file, err := os.Create(filepath.Join(dir, "test"))
		assert.Nil(t, err)

		fx := newFixture(t)
		st := fx.sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		fx.sb.Doc = st
		fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()
		mockService := mock_fileobject.NewMockService(t)
		mockService.EXPECT().Create(context.Background(), "", mock.Anything).Return("fileObjectId", domain.NewDetails(), nil).Maybe()
		fx.fileUploaderFactory = prepareFileService(t, fx.mockSender, mockService)

		// when
		proc := &dropFilesProcess{
			spaceID:             fx.SpaceID(),
			processService:      fx.processService,
			picker:              fx.picker,
			fileUploaderFactory: fx.fileUploaderFactory,
		}
		err = proc.Init([]string{file.Name()})
		assert.Nil(t, err)
		var ch = make(chan error)
		proc.Start(fx, "", model.Block_Bottom, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		storeSlice := fx.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, storeSlice, 1)
		assert.Equal(t, "fileObjectId", storeSlice[0])
	})
	t.Run("drop dir with file in collection - success", func(t *testing.T) {
		// given
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test"))
		assert.Nil(t, err)

		fx := newFixture(t)
		st := fx.sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		fx.sb.Doc = st
		fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()
		mockService := mock_fileobject.NewMockService(t)
		mockService.EXPECT().Create(context.Background(), "", mock.Anything).Return("fileObjectId", domain.NewDetails(), nil).Maybe()
		fx.fileUploaderFactory = prepareFileService(t, fx.mockSender, mockService)

		// when
		proc := &dropFilesProcess{
			spaceID:             fx.SpaceID(),
			processService:      fx.processService,
			picker:              fx.picker,
			fileUploaderFactory: fx.fileUploaderFactory,
		}
		err = proc.Init([]string{dir})
		assert.Nil(t, err)
		var ch = make(chan error)
		proc.Start(fx, "", model.Block_Bottom, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		storeSlice := fx.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, storeSlice, 1)
		assert.Equal(t, "fileObjectId", storeSlice[0])
	})
}

func prepareFileService(t *testing.T, sender *mock_event.MockSender, fileObjectService *mock_fileobject.MockService) fileuploader.Service {
	dataStoreProvider, err := datastore.NewInMemory()
	assert.Nil(t, err)

	blockStorage := filestorage.NewInMemory()

	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	objectStore := objectstore.NewStoreFixture(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())

	a := new(app.App)
	a.Register(anystoreprovider.New())
	a.Register(dataStoreProvider)
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, sender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(rpcStoreService)
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(testutil.PrepareMock(ctx, a, fileObjectService))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})
	a.Register(core.NewTempDirService())
	a.Register(testutil.PrepareMock(ctx, a, mock_cache.NewMockObjectGetterComponent(t)))
	a.Register(files.New())

	err = a.Start(ctx)
	assert.Nil(t, err)

	service := fileuploader.New()
	err = service.Init(a)
	assert.Nil(t, err)
	return service
}

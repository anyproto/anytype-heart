package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/files/fileuploader/mock_fileuploader"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fileFixture struct {
	sfile
	pickerFx     *mock_cache.MockObjectGetter
	sb           *smarttest.SmartTest
	mockSender   *mock_event.MockSender
	fileUploader *mock_fileuploader.MockService
}

func newFixture(t *testing.T) *fileFixture {
	picker := mock_cache.NewMockObjectGetter(t)
	sb := smarttest.New("root")
	mockSender := mock_event.NewMockSender(t)
	mockSender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Maybe()

	fileUploader := mock_fileuploader.NewMockService(t)

	ctx := context.Background()
	a := &app.App{}
	a.Register(testutil.PrepareMock(ctx, a, mockSender))
	a.Register(testutil.PrepareMock(ctx, a, fileUploader))
	service := process.New()
	err := service.Init(a)
	assert.Nil(t, err)

	fx := &fileFixture{
		pickerFx:     picker,
		sb:           sb,
		mockSender:   mockSender,
		fileUploader: fileUploader,
	}
	fx.sfile = sfile{
		SmartBlock:          sb,
		picker:              picker,
		processService:      service,
		fileUploaderFactory: fileUploader,
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
		synctest.Run(func() {
			// given
			dir := t.TempDir()
			path := filepath.Join(dir, "test")
			file, err := os.Create(path)
			assert.Nil(t, err)

			fx := newFixture(t)
			st := fx.sb.Doc.NewState()
			st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
			fx.sb.Doc = st
			fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil).Maybe()
			fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
			mockService := mock_fileobject.NewMockService(t)
			mockService.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return("fileObjectId", domain.NewDetails(), nil).Maybe()

			fx.assertUploaded(t, 1)

			// when
			err = fx.sfile.DropFiles(pb.RpcFileDropRequest{
				ContextId:      "root",
				LocalFilePaths: []string{file.Name()},
			})

			// then
			assert.Nil(t, err)

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})
	t.Run("drop dir in collection - no restriction error", func(t *testing.T) {
		synctest.Run(func() {
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

			fx.assertUploaded(t, 1)

			// when
			err = fx.sfile.DropFiles(pb.RpcFileDropRequest{
				ContextId:      "root",
				LocalFilePaths: []string{dir},
			})

			// then
			assert.Nil(t, err)

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})
	t.Run("drop files in collection - success", func(t *testing.T) {
		synctest.Run(func() {
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

			fx.assertUploaded(t, 1)

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
			proc.Start(fx, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
			err = <-ch

			// then
			assert.Nil(t, err)
			storeSlice := fx.NewState().GetStoreSlice(template.CollectionStoreKey)
			assert.Len(t, storeSlice, 1)
		})
	})
	t.Run("drop dir with file in collection - success", func(t *testing.T) {
		synctest.Run(func() {
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

			fx.assertUploaded(t, 1)

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
			proc.Start(fx, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
			err = <-ch

			// then
			assert.Nil(t, err)
			storeSlice := fx.NewState().GetStoreSlice(template.CollectionStoreKey)
			assert.Len(t, storeSlice, 1)

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})

	t.Run("style propagates to file block", func(t *testing.T) {
		synctest.Run(func() {
			// given
			dir := t.TempDir()
			file, err := os.Create(filepath.Join(dir, "test.jpg"))
			assert.Nil(t, err)

			fx := newFixture(t)
			fx.sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text("", blockbuilder.ID("targetBlock")),
				)))

			fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil)
			fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()

			fx.assertUploaded(t, 1)

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
			go proc.Start(fx, pb.RpcFileDropRequest{
				DropTargetId: "targetBlock",
				Position:     model.Block_Bottom,
				Style:        model.BlockContentFile_Embed,
			}, ch)
			err = <-ch

			// then
			assert.Nil(t, err)

			// Find the created file block
			var fileBlock *model.Block
			for _, block := range fx.sb.Blocks() {
				if block.GetFile() != nil && block.GetFile().Name == "test.jpg" {
					fileBlock = block
					break
				}
			}

			require.NotNil(t, fileBlock, "file block should be created")
			assert.Equal(t, model.BlockContentFile_Embed, fileBlock.GetFile().Style, "style should be propagated to file block")

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})

	t.Run("style propagates to multiple file blocks", func(t *testing.T) {
		synctest.Run(func() {
			// given
			dir := t.TempDir()
			file1, err := os.Create(filepath.Join(dir, "test1.jpg"))
			assert.Nil(t, err)
			file2, err := os.Create(filepath.Join(dir, "test2.png"))
			assert.Nil(t, err)

			fx := newFixture(t)
			fx.sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text("", blockbuilder.ID("targetBlock")),
				)))

			fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil)
			fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()

			fx.assertUploaded(t, 2)

			// when
			proc := &dropFilesProcess{
				spaceID:             fx.SpaceID(),
				processService:      fx.processService,
				picker:              fx.picker,
				fileUploaderFactory: fx.fileUploaderFactory,
			}
			err = proc.Init([]string{file1.Name(), file2.Name()})
			assert.Nil(t, err)
			var ch = make(chan error)
			go proc.Start(fx, pb.RpcFileDropRequest{
				DropTargetId: "targetBlock",
				Position:     model.Block_Bottom,
				Style:        model.BlockContentFile_Link,
			}, ch)
			err = <-ch

			// then
			assert.Nil(t, err)

			// Find all created file blocks
			var fileBlocks []*model.Block
			for _, block := range fx.sb.Blocks() {
				if block.GetFile() != nil && (block.GetFile().Name == "test1.jpg" || block.GetFile().Name == "test2.png") {
					fileBlocks = append(fileBlocks, block)
				}
			}

			require.Len(t, fileBlocks, 2, "two file blocks should be created")
			for _, fileBlock := range fileBlocks {
				assert.Equal(t, model.BlockContentFile_Link, fileBlock.GetFile().Style, "style should be propagated to all file blocks")
			}

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})

	t.Run("default style when not specified", func(t *testing.T) {
		synctest.Run(func() {
			// given
			dir := t.TempDir()
			file, err := os.Create(filepath.Join(dir, "test.pdf"))
			assert.Nil(t, err)

			fx := newFixture(t)
			fx.sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text("", blockbuilder.ID("targetBlock")),
				)))

			fx.pickerFx.EXPECT().GetObject(context.Background(), "root").Return(fx, nil)
			fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()

			fx.assertUploaded(t, 1)

			// when - no style specified, should default to Auto
			proc := &dropFilesProcess{
				spaceID:             fx.SpaceID(),
				processService:      fx.processService,
				picker:              fx.picker,
				fileUploaderFactory: fx.fileUploaderFactory,
			}
			err = proc.Init([]string{file.Name()})
			assert.Nil(t, err)
			var ch = make(chan error)
			go proc.Start(fx, pb.RpcFileDropRequest{
				DropTargetId: "targetBlock",
				Position:     model.Block_Bottom,
				Style:        model.BlockContentFile_Auto,
			}, ch)
			err = <-ch

			// then
			assert.Nil(t, err)

			// Find the created file block
			var fileBlock *model.Block
			for _, block := range fx.sb.Blocks() {
				if block.GetFile() != nil && block.GetFile().Name == "test.pdf" {
					fileBlock = block
					break
				}
			}

			require.NotNil(t, fileBlock, "file block should be created")
			assert.Equal(t, model.BlockContentFile_Auto, fileBlock.GetFile().Style, "default style should be Auto")

			// wait for background processes to finish
			time.Sleep(1 * time.Second)
		})
	})
}

func (fx *fileFixture) assertUploaded(t *testing.T, times int) {
	uploader := mock_fileuploader.NewMockUploader(t)
	uploader.EXPECT().SetName(mock.Anything).Return(uploader).Times(times)
	uploader.EXPECT().SetFile(mock.Anything).Return(uploader).Times(times)
	uploader.EXPECT().SetCreatedInContext(mock.Anything).Return(uploader).Times(times)
	uploader.EXPECT().SetCreatedInContextRef(mock.Anything).Return(uploader).Times(times)
	uploader.EXPECT().Upload(mock.Anything).Return(fileuploader.UploadResult{}).Times(times)

	fx.fileUploader.EXPECT().NewUploader(mock.Anything, mock.Anything).Return(uploader).Times(times)
}

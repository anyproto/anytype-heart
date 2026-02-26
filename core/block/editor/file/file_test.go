package file

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
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
	fileUploader *fileuploader.MockService
}

func newFixture(t *testing.T) *fileFixture {
	picker := mock_cache.NewMockObjectGetter(t)
	sb := smarttest.New("root")
	mockSender := mock_event.NewMockSender(t)
	mockSender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Maybe()

	fileUploader := fileuploader.NewMockService(t)

	fx := &fileFixture{
		pickerFx:     picker,
		sb:           sb,
		mockSender:   mockSender,
		fileUploader: fileUploader,
	}
	fx.sfile = sfile{
		SmartBlock:          sb,
		picker:              picker,
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

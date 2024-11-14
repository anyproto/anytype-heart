package files

import (
	"context"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestGetImageForWidth(t *testing.T) {
	fx := newFixture(t)
	res := testAddImage(t, fx)

	fullId := domain.FullFileId{
		SpaceId: spaceId,
		FileId:  res.FileId,
	}
	ctx := context.Background()

	img, err := fx.ImageByHash(ctx, fullId)
	require.NoError(t, err)

	for _, testCase := range []struct {
		name           string
		requestedWidth int
		expectedWidth  int64
	}{
		{
			name:           "original",
			requestedWidth: 1920,
			expectedWidth:  1024,
		},
		{
			name:           "large",
			requestedWidth: 1920,
			expectedWidth:  1024,
		},
		{
			name:           "small",
			requestedWidth: 320,
			expectedWidth:  320,
		},
		{
			name:           "thumbnail",
			requestedWidth: 100,
			expectedWidth:  100,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			file, err := img.GetFileForWidth(testCase.requestedWidth)
			require.NoError(t, err)

			assertWidth(t, file, testCase.expectedWidth)
		})
	}
}

func assertWidth(t *testing.T, file File, width int64) {
	require.NotNil(t, file)
	require.NotNil(t, file.Meta())

	meta := file.Info().GetMeta()

	assert.Equal(t, width, pbtypes.GetInt64(meta, "width"))
}

func TestImageDetails(t *testing.T) {
	fx := newFixture(t)

	got := testAddImageWithRichExifData(t, fx)

	ctx := context.Background()
	image, err := fx.ImageByHash(ctx, domain.FullFileId{SpaceId: spaceId, FileId: got.FileId})
	require.NoError(t, err)

	details, err := image.Details(ctx)
	require.NoError(t, err)

	// From exif data
	createdDate, err := time.ParseInLocation("2006:01:02 15:04:05", "2008:05:30 15:56:01", time.Local)
	require.NoError(t, err)

	for _, testCase := range []struct {
		key   domain.RelationKey
		value *types.Value
	}{
		{key: bundle.RelationKeyWidthInPixels, value: pbtypes.Int64(100)},
		{key: bundle.RelationKeyHeightInPixels, value: pbtypes.Int64(68)},
		{key: bundle.RelationKeyIsHidden, value: nil},
		{key: bundle.RelationKeyName, value: pbtypes.String("myFile")},
		{key: bundle.RelationKeyFileExt, value: pbtypes.String("jpg")},
		{key: bundle.RelationKeyFileMimeType, value: pbtypes.String("image/jpeg")},
		{key: bundle.RelationKeySizeInBytes, value: pbtypes.Int64(5480)},
		{key: bundle.RelationKeyCreatedDate, value: pbtypes.Int64(createdDate.Unix())},
		{key: bundle.RelationKeyCamera, value: pbtypes.String("Canon EOS 40D")},
		{key: bundle.RelationKeyExposure, value: pbtypes.String("1/160")},
		{key: bundle.RelationKeyFocalRatio, value: pbtypes.Float64(7.1)},
		{key: bundle.RelationKeyCameraIso, value: pbtypes.Float64(100)},
	} {
		t.Run(testCase.key.String(), func(t *testing.T) {
			assert.Equal(t, testCase.value, details.Fields[testCase.key.String()])
		})
	}
}

func TestImageGetOriginalFile(t *testing.T) {
	fx := newFixture(t)

	got := testAddImageWithRichExifData(t, fx)

	ctx := context.Background()
	image, err := fx.ImageByHash(ctx, domain.FullFileId{SpaceId: spaceId, FileId: got.FileId})
	require.NoError(t, err)

	file, err := image.GetOriginalFile()
	require.NoError(t, err)

	reader, err := file.Reader(ctx)
	require.NoError(t, err)

	imageData, err := jpeg.Decode(reader)
	require.NoError(t, err)

	wantFile, err := os.Open("testdata/image_with_rich_exif_data.jpg")
	require.NoError(t, err)
	defer wantFile.Close()

	wantImageData, err := jpeg.Decode(wantFile)
	require.NoError(t, err)

	assert.Equal(t, wantImageData, imageData)
}

func testAddImageWithRichExifData(t *testing.T, fx *fixture) *AddResult {
	f, err := os.Open("testdata/image_with_rich_exif_data.jpg")
	require.NoError(t, err)
	defer f.Close()

	fileName := "myFile.jpg"
	lastModifiedDate := time.Now()
	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(f),
	}
	got, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()
	return got
}

func TestSelectAndSortResizeVariants(t *testing.T) {
	t.Run("with resize variants", func(t *testing.T) {
		got := selectAndSortResizeVariants([]*storage.FileInfo{
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(200),
					},
				},
			},
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(100),
					},
				},
			},
			{
				Mill: mill.ImageExifId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(300),
					},
				},
			},
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(300),
					},
				},
			},
		})

		want := []*storage.FileInfo{
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(100),
					},
				},
			},
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(200),
					},
				},
			},
			{
				Mill: mill.ImageResizeId,
				Meta: &types.Struct{
					Fields: map[string]*types.Value{
						"width": pbtypes.Int64(300),
					},
				},
			},
		}

		assert.Equal(t, want, got)
	})
	t.Run("with blob variant", func(t *testing.T) {
		got := selectAndSortResizeVariants([]*storage.FileInfo{
			{
				Mill: mill.BlobId,
			},
		})

		want := []*storage.FileInfo{
			{
				Mill: mill.BlobId,
			},
		}

		assert.Equal(t, want, got)
	})
}

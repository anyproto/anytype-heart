package files

import (
	"context"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func TestGetImageForWidth(t *testing.T) {
	fx := newFixture(t)
	res := testAddImage(t, fx)

	fullId := domain.FullFileId{
		SpaceId: spaceId,
		FileId:  res.FileId,
	}
	ctx := context.Background()

	variants, err := fx.GetFileVariants(ctx, fullId, res.EncryptionKeys.EncryptionKeys)
	require.NoError(t, err)

	img := NewImage(fx, fullId, variants)

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

	meta := file.Meta()

	assert.Equal(t, width, meta.Width)
}

func TestImageDetails(t *testing.T) {
	fx := newFixture(t)

	got := testAddImageWithRichExifData(t, fx)

	fullId := domain.FullFileId{SpaceId: spaceId, FileId: got.FileId}
	ctx := context.Background()
	variants, err := fx.GetFileVariants(ctx, fullId, got.EncryptionKeys.EncryptionKeys)
	require.NoError(t, err)

	image := NewImage(fx, fullId, variants)

	details, err := image.Details(ctx)
	require.NoError(t, err)

	// From exif data
	createdDate, err := time.ParseInLocation("2006:01:02 15:04:05", "2008:05:30 15:56:01", time.Local)
	require.NoError(t, err)

	for _, testCase := range []struct {
		key   domain.RelationKey
		value domain.Value
	}{
		{key: bundle.RelationKeyWidthInPixels, value: domain.Int64(100)},
		{key: bundle.RelationKeyHeightInPixels, value: domain.Int64(68)},
		{key: bundle.RelationKeyIsHidden, value: domain.Invalid()},
		{key: bundle.RelationKeyName, value: domain.String("myFile")},
		{key: bundle.RelationKeyFileExt, value: domain.String("jpg")},
		{key: bundle.RelationKeyFileMimeType, value: domain.String("image/jpeg")},
		{key: bundle.RelationKeySizeInBytes, value: domain.Int64(7958)},
		{key: bundle.RelationKeyCreatedDate, value: domain.Int64(createdDate.Unix())},
		{key: bundle.RelationKeyCamera, value: domain.String("Canon EOS 40D")},
		{key: bundle.RelationKeyExposure, value: domain.String("1/160")},
		{key: bundle.RelationKeyFocalRatio, value: domain.Float64(7.1)},
		{key: bundle.RelationKeyCameraIso, value: domain.Float64(100)},
	} {
		t.Run(testCase.key.String(), func(t *testing.T) {
			assert.Equal(t, testCase.value, details.Get(testCase.key))
		})
	}
}

func TestImageGetOriginalFile(t *testing.T) {
	fx := newFixture(t)

	got := testAddImageWithRichExifData(t, fx)

	fullId := domain.FullFileId{SpaceId: spaceId, FileId: got.FileId}
	ctx := context.Background()
	variants, err := fx.GetFileVariants(ctx, fullId, got.EncryptionKeys.EncryptionKeys)
	require.NoError(t, err)

	image := NewImage(fx, fullId, variants)

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
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(200),
				}).ToProto(),
			},
			{
				Mill: mill.ImageResizeId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(100),
				}).ToProto(),
			},
			{
				Mill: mill.ImageExifId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(300),
				}).ToProto(),
			},
			{
				Mill: mill.ImageResizeId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(300),
				}).ToProto(),
			},
		})

		want := []*storage.FileInfo{
			{
				Mill: mill.ImageResizeId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(100),
				}).ToProto(),
			},
			{
				Mill: mill.ImageResizeId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(200),
				}).ToProto(),
			},
			{
				Mill: mill.ImageResizeId,
				Meta: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"width": domain.Int64(300),
				}).ToProto(),
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

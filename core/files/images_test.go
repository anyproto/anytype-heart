package files

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
)

func TestImageAdd(t *testing.T) {
	t.Run("add image", func(t *testing.T) {
		fx := newFixture(t)
		got := testAddImage(t, fx)

		assert.NotEmpty(t, got.MIME)
		assert.True(t, got.Size > 0)
		assert.False(t, got.IsExisting)

		t.Run("add same image again", func(t *testing.T) {
			got2 := testAddImage(t, fx)

			assert.NotEmpty(t, got2.MIME)
			assert.True(t, got2.Size > 0)

			assert.Equal(t, got.FileId, got2.FileId)
			assert.Equal(t, got.EncryptionKeys, got2.EncryptionKeys)
			assert.Equal(t, got.MIME, got2.MIME)
			assert.Equal(t, got.Size, got2.Size)
			assert.True(t, got2.IsExisting)
		})
	})

	// We had a problem with concurrent adding of the same image, so test it
	t.Run("concurrent adding of the same image", func(t *testing.T) {
		testAddConcurrently(t, func(t *testing.T, fx *fixture) *AddResult {
			return testAddImage(t, fx)
		})
	})
}

func TestIndexImage(t *testing.T) {
	t.Run("with encryption keys available", func(t *testing.T) {
		fx := newFixture(t)
		got := testAddImage(t, fx)

		err := fx.fileStore.DeleteFile(got.FileId)
		require.NoError(t, err)

		err = fx.fileStore.AddFileKeys(*got.EncryptionKeys)
		require.NoError(t, err)

		image, err := fx.ImageByHash(context.Background(), domain.FullFileId{SpaceId: spaceId, FileId: got.FileId})
		require.NoError(t, err)

		assert.Equal(t, got.FileId, image.FileId())
	})

	t.Run("with encryption keys not available", func(t *testing.T) {
		fx := newFixture(t)
		got := testAddImage(t, fx)

		err := fx.fileStore.DeleteFile(got.FileId)
		require.NoError(t, err)

		_, err = fx.ImageByHash(context.Background(), domain.FullFileId{SpaceId: spaceId, FileId: got.FileId})
		require.Error(t, err)
	})
}

func TestImageAddWithCustomEncryptionKeys(t *testing.T) {
	fx := newFixture(t)

	customKeys := map[string]string{
		encryptionKeyPath(schema.LinkImageOriginal):  "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		encryptionKeyPath(schema.LinkImageLarge):     "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		encryptionKeyPath(schema.LinkImageSmall):     "bear36qgxpvnsqis2omwqi33zcrjo6arxhokpqr3bnh2oqphxkiba",
		encryptionKeyPath(schema.LinkImageThumbnail): "bcewq7zoa6cbbev6nxkykrrclvidriuglgags67zbdda53wfnn6eq",
		encryptionKeyPath(schema.LinkImageExif):      "bdoiogvdd5bayrezafzf2lvgh3xxjk7ru4yq2frpxhjgmx26ih6sq",
	}
	f, err := os.Open("../../pkg/lib/mill/testdata/image.jpeg")
	require.NoError(t, err)
	defer f.Close()

	fileName := "myFile"
	lastModifiedDate := time.Now()
	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(f),
		WithCustomEncryptionKeys(customKeys),
	}
	got, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()

	assertCustomEncryptionKeys(t, fx, got, customKeys)
}

func TestImageAddReuse(t *testing.T) {
	fx := newFixture(t)

	f, err := os.Open("../../pkg/lib/mill/testdata/image.jpeg")
	require.NoError(t, err)
	defer f.Close()

	fileName := "myFile"
	lastModifiedDate := time.Now()
	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(f),
	}
	got1, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got1.Commit()

	f.Seek(0, 0)
	got2, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got2.Commit()
	require.True(t, got2.IsExisting)
	require.Equal(t, got1.FileId.String(), got2.FileId.String())
	require.Equal(t, got1.EncryptionKeys.EncryptionKeys, got2.EncryptionKeys.EncryptionKeys)

	b, err := io.ReadAll(f)
	require.NoError(t, err)
	b[10000] = 0x00
	// patch the original image so it will have the different source hash, but the same(empty) exif
	patchedReader := bytes.NewReader(b)
	opts = []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(patchedReader),
	}
	// exif will be the same but images are different
	got3, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got3.Commit()
	fileId3 := got3.FileId.String()

	require.NotEqual(t, got1.FileId.String(), fileId3)
	require.False(t, got3.IsExisting)
}

func TestReuseWithCorruptedFileInfo(t *testing.T) {
	fx := newFixture(t)

	addResult := testAddImage(t, fx)

	variants, err := fx.fileStore.ListFileVariants(addResult.FileId)
	require.NoError(t, err)
	for _, variant := range variants {
		variant.Targets = nil
	}
	err = fx.fileStore.AddFileVariants(true, variants...)
	require.NoError(t, err)

	addResult = testAddImage(t, fx)
	require.False(t, addResult.IsExisting)
}

func assertCustomEncryptionKeys(t *testing.T, fx *fixture, got *AddResult, customKeys map[string]string) {
	encKeys, err := fx.fileStore.GetFileKeys(got.FileId)
	require.NoError(t, err)
	assert.Equal(t, customKeys, encKeys)

	variants, err := fx.fileStore.ListFileVariants(got.FileId)
	require.NoError(t, err)

	for _, v := range variants {
		var found bool
		for _, key := range customKeys {
			if v.Key == key {
				found = true
				break
			}
		}
		require.True(t, found)
	}
}

func testAddImage(t *testing.T, fx *fixture) *AddResult {
	f, err := os.Open("../../pkg/lib/mill/testdata/image.jpeg")
	require.NoError(t, err)
	defer f.Close()

	fileName := "myFile"
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

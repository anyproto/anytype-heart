package files

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func TestFile_Details(t *testing.T) {
	fx := newFixture(t)

	t.Run("svg details", func(t *testing.T) {
		// given
		f, err := NewFile(fx, domain.FullFileId{SpaceId: spaceId, FileId: "id"}, []*storage.FileInfo{
			{
				Media: "svg+xml",
				Name:  "image.svg",
			},
		})
		require.NoError(t, err)

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyImage, typeKey)
		assert.Equal(t, int64(model.ObjectType_image), details.GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, "svg", details.GetString(bundle.RelationKeyFileExt))
		assert.Equal(t, "image", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id", details.GetString(bundle.RelationKeyFileId))
	})
	t.Run("general file", func(t *testing.T) {
		// given
		f, err := NewFile(fx, domain.FullFileId{SpaceId: spaceId, FileId: "id"}, []*storage.FileInfo{
			{
				Name: "file.txt",
			},
		})
		require.NoError(t, err)

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyFile, typeKey)
		assert.Equal(t, int64(model.ObjectType_file), details.GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, "txt", details.GetString(bundle.RelationKeyFileExt))
		assert.Equal(t, "file", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id", details.GetString(bundle.RelationKeyFileId))
	})
	t.Run("audio file", func(t *testing.T) {
		// given
		f, err := NewFile(fx, domain.FullFileId{SpaceId: spaceId, FileId: "id"}, []*storage.FileInfo{
			{
				Name:  "file.mp3",
				Media: "audio",
			},
		})
		require.NoError(t, err)

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyAudio, typeKey)
		assert.Equal(t, int64(model.ObjectType_audio), details.GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, "mp3", details.GetString(bundle.RelationKeyFileExt))
		assert.Equal(t, "file", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id", details.GetString(bundle.RelationKeyFileId))
	})
	t.Run("video file", func(t *testing.T) {
		// given
		f, err := NewFile(fx, domain.FullFileId{SpaceId: spaceId, FileId: "id"}, []*storage.FileInfo{
			{
				Name:  "file.mp4",
				Media: "video",
			},
		})
		require.NoError(t, err)

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyVideo, typeKey)
		assert.Equal(t, int64(model.ObjectType_video), details.GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, "mp4", details.GetString(bundle.RelationKeyFileExt))
		assert.Equal(t, "file", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id", details.GetString(bundle.RelationKeyFileId))
	})
	t.Run("pdf file", func(t *testing.T) {
		// given
		f, err := NewFile(fx, domain.FullFileId{SpaceId: spaceId, FileId: "id"}, []*storage.FileInfo{
			{
				Name:  "file.pdf",
				Media: "application/pdf",
			},
		})
		require.NoError(t, err)

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyFile, typeKey)
		assert.Equal(t, int64(model.ObjectType_pdf), details.GetInt64(bundle.RelationKeyLayout))
		assert.Equal(t, "pdf", details.GetString(bundle.RelationKeyFileExt))
		assert.Equal(t, "file", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id", details.GetString(bundle.RelationKeyFileId))
	})
}

package files

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFile_Details(t *testing.T) {
	t.Run("svg details", func(t *testing.T) {
		// given
		f := &file{
			info: &storage.FileInfo{
				Media: "svg+xml",
				Name:  "image.svg",
			},
			fileId: domain.FileId("id"),
		}

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyImage, typeKey)
		assert.Equal(t, int64(model.ObjectType_image), pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
		assert.Equal(t, "svg", pbtypes.GetString(details, bundle.RelationKeyFileExt.String()))
		assert.Equal(t, "image", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, "id", pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	})
	t.Run("general file", func(t *testing.T) {
		// given
		f := &file{
			info: &storage.FileInfo{
				Name: "file.txt",
			},
			fileId: domain.FileId("id"),
		}

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyFile, typeKey)
		assert.Equal(t, int64(model.ObjectType_file), pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
		assert.Equal(t, "txt", pbtypes.GetString(details, bundle.RelationKeyFileExt.String()))
		assert.Equal(t, "file", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, "id", pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	})
	t.Run("audio file", func(t *testing.T) {
		// given
		f := &file{
			info: &storage.FileInfo{
				Name:  "file.mp3",
				Media: "audio",
			},
			fileId: domain.FileId("id"),
		}

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyAudio, typeKey)
		assert.Equal(t, int64(model.ObjectType_audio), pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
		assert.Equal(t, "mp3", pbtypes.GetString(details, bundle.RelationKeyFileExt.String()))
		assert.Equal(t, "file", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, "id", pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	})
	t.Run("video file", func(t *testing.T) {
		// given
		f := &file{
			info: &storage.FileInfo{
				Name:  "file.mp4",
				Media: "video",
			},
			fileId: domain.FileId("id"),
		}

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyVideo, typeKey)
		assert.Equal(t, int64(model.ObjectType_video), pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
		assert.Equal(t, "mp4", pbtypes.GetString(details, bundle.RelationKeyFileExt.String()))
		assert.Equal(t, "file", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, "id", pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	})
	t.Run("pdf file", func(t *testing.T) {
		// given
		f := &file{
			info: &storage.FileInfo{
				Name:  "file.pdf",
				Media: "application/pdf",
			},
			fileId: domain.FileId("id"),
		}

		// when
		details, typeKey, err := f.Details(context.Background())

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundle.TypeKeyFile, typeKey)
		assert.Equal(t, int64(model.ObjectType_pdf), pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
		assert.Equal(t, "pdf", pbtypes.GetString(details, bundle.RelationKeyFileExt.String()))
		assert.Equal(t, "file", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, "id", pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	})
}

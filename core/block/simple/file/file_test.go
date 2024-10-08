package file

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/test"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestFile_Diff(t *testing.T) {
	testBlock := func() *File {
		return NewFile(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfFile{File: &model.BlockContentFile{}},
		}).(*File)
	}
	testBlockPdf := func() *File {
		return NewFile(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfFile{File: &model.BlockContentFile{Type: model.BlockContentFile_PDF}},
		}).(*File)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff(b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlockPdf()
		b2 := testBlockPdf()
		tm := time.Now()
		b1.SetHash("1").SetMIME("2").SetName("3").SetSize(4).SetTime(tm)
		b2.SetHash("1").SetMIME("2").SetName("3").SetSize(4).SetTime(tm)
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlockPdf()
		b2 := testBlockPdf()
		b2.Restrictions.Read = true
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()

		b2.SetState(model.BlockContentFile_Done)
		b2.SetHash("hash").SetMIME("video/mpeg").SetName("image.mpg").SetSize(3).SetTime(time.Now()).SetType(model.BlockContentFile_Video)

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetFile{
			BlockSetFile: &pb.EventBlockSetFile{
				Id:    b1.Id,
				Type:  &pb.EventBlockSetFileType{Value: model.BlockContentFile_Video},
				State: &pb.EventBlockSetFileState{Value: model.BlockContentFile_Done},
				Mime:  &pb.EventBlockSetFileMime{Value: "video/mpeg"},
				Hash:  &pb.EventBlockSetFileHash{Value: "hash"},
				Name:  &pb.EventBlockSetFileName{Value: "image.mpg"},
				Size_: &pb.EventBlockSetFileSize{Value: 3},
			},
		}), diff)
	})
}

func TestDetectTypeByMIME(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		mime     string
		expected model.BlockContentFileType
	}{
		{
			name:     "SVG file",
			fileName: "example.svg",
			mime:     "image/svg+xml",
			expected: model.BlockContentFile_Image,
		},
		{
			name:     "JPEG image",
			fileName: "photo.jpg",
			mime:     "image/jpeg",
			expected: model.BlockContentFile_Image,
		},
		{
			name:     "Video file",
			fileName: "video.mp4",
			mime:     "video/mp4",
			expected: model.BlockContentFile_Video,
		},
		{
			name:     "Audio file",
			fileName: "song.mp3",
			mime:     "audio/mpeg",
			expected: model.BlockContentFile_Audio,
		},
		{
			name:     "PDF document",
			fileName: "document.pdf",
			mime:     "application/pdf",
			expected: model.BlockContentFile_PDF,
		},
		{
			name:     "Generic file",
			fileName: "archive.zip",
			mime:     "application/zip",
			expected: model.BlockContentFile_File,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := DetectTypeByMIME(tc.fileName, tc.mime)
			assert.Equal(t, tc.expected, result)
		})
	}
}

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

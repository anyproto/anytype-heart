package file

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetFile).BlockSetFile
		assert.NotNil(t, change.Hash)
		assert.NotNil(t, change.Size_)
		assert.NotNil(t, change.State)
		assert.NotNil(t, change.Name)
		assert.NotNil(t, change.Type)
		assert.NotNil(t, change.Mime)
	})
}

func TestFile_Validate(t *testing.T) {
	t.Run("not validated", func(t *testing.T) {
		b := NewFile(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfFile{File: &model.BlockContentFile{State: model.BlockContentFile_Done, Size_: 0}},
		}).(*File)
		err := b.Validate()
		assert.Error(t, err)
	})
}

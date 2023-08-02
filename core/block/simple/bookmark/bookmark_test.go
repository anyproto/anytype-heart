package bookmark

import (
	"github.com/anyproto/anytype-heart/core/block/simple/test"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setLinkPreview(b Block, data model.LinkPreview) {
	b.UpdateContent(func(c *model.BlockContentBookmark) {
		c.Url = data.Url
		c.Title = data.Title
		c.Description = data.Description
		c.Type = data.Type
	})
}

func TestBookmark_Diff(t *testing.T) {
	testBlock := func() *Bookmark {
		return NewBookmark(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}},
		}).(*Bookmark)
	}
	lp := model.LinkPreview{
		Url:         "1",
		Title:       "2",
		Description: "3",
		ImageUrl:    "4",
		FaviconUrl:  "5",
		Type:        6,
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff(b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		setLinkPreview(b1, lp)
		setLinkPreview(b2, lp)
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = true
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()

		setLinkPreview(b2, lp)
		b2.UpdateContent(func(c *model.BlockContentBookmark) {
			c.FaviconHash = "fh"
			c.ImageHash = "ih"
			c.TargetObjectId = "newobject"
		})

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetBookmark{
			BlockSetBookmark: &pb.EventBlockSetBookmark{
				Id:             b1.Id,
				Url:            &pb.EventBlockSetBookmarkUrl{Value: "1"},
				Title:          &pb.EventBlockSetBookmarkTitle{Value: "2"},
				Description:    &pb.EventBlockSetBookmarkDescription{Value: "3"},
				ImageHash:      &pb.EventBlockSetBookmarkImageHash{Value: "ih"},
				FaviconHash:    &pb.EventBlockSetBookmarkFaviconHash{Value: "fh"},
				Type:           &pb.EventBlockSetBookmarkType{Value: 6},
				TargetObjectId: &pb.EventBlockSetBookmarkTargetObjectId{Value: "newobject"},
			},
		}), diff)
	})
}

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func Test_GetBookmarkBlock(t *testing.T) {
	bo := &BookmarkObject{
		URL:     "",
		Caption: []*api.RichText{},
	}
	bb, _ := bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "")

	bo = &BookmarkObject{
		URL:     "http://example.com",
		Caption: []*api.RichText{},
	}
	bb, _ = bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "")

	bo = &BookmarkObject{
		URL: "",
		Caption: []*api.RichText{{
			Type:      api.Text,
			PlainText: "Text",
		}},
	}
	bb, _ = bo.GetBookmarkBlock()
	assert.NotNil(t, bb)
	assert.Equal(t, bb.GetBookmark().GetUrl(), bo.URL)
	assert.Equal(t, bb.GetBookmark().GetTitle(), "Text")
}

func Test_GetLinkToObjectBlockSuccess(t *testing.T) {
	c := &ChildPG{Title: "title"}
	nameToID := map[string]string{"id": "title"}
	notionIdsToAnytype := map[string]string{"id": "anytypeId"}
	bl := c.GetLinkToObjectBlock(notionIdsToAnytype, nameToID)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockFail(t *testing.T) {
	c := &ChildPG{Title: "title"}
	bl := c.GetLinkToObjectBlock(nil, nil)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)

	nameToID := map[string]string{"id": "title"}
	bl = c.GetLinkToObjectBlock(nameToID, nil)
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)
}

func Test_GetLinkToObjectBlockInlineCollection(t *testing.T) {
	c := &ChildDB{Title: "title"}
	nameToID := map[string]string{"id": "title"}
	notionIdsToAnytype := map[string]string{"id": "anytypeId"}
	bl := c.GetDataviewBlock(notionIdsToAnytype, nameToID)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "anytypeId")
}

func Test_GetLinkToObjectBlockInlineCollectionEmpty(t *testing.T) {
	c := &ChildDB{Title: "title"}
	bl := c.GetDataviewBlock(nil, nil)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfDataview)
	assert.True(t, ok)
	assert.Equal(t, content.Dataview.TargetObjectId, "")
}

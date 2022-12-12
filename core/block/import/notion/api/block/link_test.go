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
	c := &Child{Title: "title"}
	nameToID := map[string]string{"id": "title"}
	notionIdsToAnytype := map[string]string{"id": "anytypeId"}
	bl, _ := c.GetLinkToObjectBlock(notionIdsToAnytype, nameToID)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfLink)
	assert.True(t, ok)
	assert.Equal(t, content.Link.TargetBlockId, "anytypeId")
}

func Test_GetLinkToObjectBlockFail(t *testing.T) {
	c := &Child{Title: "title"}
	bl, _ := c.GetLinkToObjectBlock(nil, nil)
	assert.NotNil(t, bl)
	content, ok := bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)

	nameToID := map[string]string{"id": "title"}
	bl, _ = c.GetLinkToObjectBlock(nameToID, nil)
	assert.NotNil(t, bl)
	content, ok = bl.Content.(*model.BlockContentOfText)
	assert.True(t, ok)
	assert.Equal(t, content.Text.Text, notFoundPageMessage)
}